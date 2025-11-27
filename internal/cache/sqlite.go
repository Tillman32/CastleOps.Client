package cache

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog"
)

// SQLiteCache implements the Cache interface using SQLite as the backend
// Optimized for high-performance with WAL mode, prepared statements, and connection pooling
type SQLiteCache struct {
	db     *sql.DB
	dbPath string
	logger zerolog.Logger

	// Prepared statements for hot paths (reused to minimize allocations)
	stmtStoreMetrics       *sql.Stmt
	stmtGetMetrics         *sql.Stmt
	stmtStoreCommand       *sql.Stmt
	stmtGetPendingCommands *sql.Stmt
	stmtMarkComplete       *sql.Stmt

	// Mutex for statement initialization
	stmtMu sync.RWMutex

	// Retention settings
	retentionDays int
}

// SQLiteConfig holds configuration for SQLite cache
type SQLiteConfig struct {
	Path          string
	RetentionDays int
	Logger        zerolog.Logger
}

// NewSQLiteCache creates a new SQLite-backed cache with optimized settings
func NewSQLiteCache(cfg SQLiteConfig) (*SQLiteCache, error) {
	if cfg.Path == "" {
		return nil, fmt.Errorf("cache path cannot be empty")
	}

	if cfg.RetentionDays <= 0 {
		cfg.RetentionDays = 7 // Default to 7 days
	}

	// Ensure directory exists
	dir := filepath.Dir(cfg.Path)
	if err := ensureDir(dir); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Open database with performance optimizations
	// Connection string includes:
	// - _journal_mode=WAL: Write-Ahead Logging for better concurrency
	// - _synchronous=NORMAL: Balance between safety and performance
	// - _cache_size=-64000: 64MB cache (negative means KB)
	// - _busy_timeout=5000: Wait up to 5s for locks
	// - _foreign_keys=ON: Enforce referential integrity
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_synchronous=NORMAL&_cache_size=-64000&_busy_timeout=5000&_foreign_keys=ON",
		cfg.Path)

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool for optimal performance
	// Max open connections = 4 (SQLite doesn't benefit from many connections)
	// Max idle connections = 2 (keep warm connections ready)
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(time.Hour)
	db.SetConnMaxIdleTime(10 * time.Minute)

	cache := &SQLiteCache{
		db:            db,
		dbPath:        cfg.Path,
		logger:        cfg.Logger,
		retentionDays: cfg.RetentionDays,
	}

	// Initialize schema
	if err := cache.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	// Prepare statements for hot paths
	if err := cache.prepareStatements(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to prepare statements: %w", err)
	}

	// Start background cleanup goroutine
	go cache.periodicCleanup()

	return cache, nil
}

// initSchema creates the database schema if it doesn't exist
func (c *SQLiteCache) initSchema() error {
	schema := `
	-- Metrics table stores system metrics snapshots
	CREATE TABLE IF NOT EXISTS metrics (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		client_id TEXT NOT NULL,
		timestamp DATETIME NOT NULL,
		cpu_usage_percent REAL NOT NULL,
		memory_total INTEGER NOT NULL,
		memory_used INTEGER NOT NULL,
		memory_available INTEGER NOT NULL,
		memory_usage_percent REAL NOT NULL,
		disk_total INTEGER NOT NULL,
		disk_used INTEGER NOT NULL,
		disk_free INTEGER NOT NULL,
		disk_usage_percent REAL NOT NULL,
		network_bytes_received INTEGER NOT NULL,
		network_bytes_sent INTEGER NOT NULL,
		synced INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	-- Index for time-range queries (most common query pattern)
	CREATE INDEX IF NOT EXISTS idx_metrics_timestamp ON metrics(timestamp);

	-- Index for finding unsynced metrics
	CREATE INDEX IF NOT EXISTS idx_metrics_synced ON metrics(synced, timestamp);

	-- Index for client queries
	CREATE INDEX IF NOT EXISTS idx_metrics_client ON metrics(client_id, timestamp);

	-- Commands table stores pending and completed commands
	CREATE TABLE IF NOT EXISTS commands (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		command_id TEXT NOT NULL UNIQUE,
		type TEXT NOT NULL,
		payload TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending',
		result TEXT,
		created_at DATETIME NOT NULL,
		completed_at DATETIME,
		created_local DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	-- Index for finding pending commands (hot query)
	CREATE INDEX IF NOT EXISTS idx_commands_status ON commands(status, created_at);

	-- Index for command lookups by ID
	CREATE INDEX IF NOT EXISTS idx_commands_command_id ON commands(command_id);

	-- Sync status table tracks synchronization state
	CREATE TABLE IF NOT EXISTS sync_status (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		last_sync_time DATETIME NOT NULL,
		metrics_synced INTEGER NOT NULL DEFAULT 0,
		commands_processed INTEGER NOT NULL DEFAULT 0,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	-- Initialize sync_status if empty
	INSERT OR IGNORE INTO sync_status (id, last_sync_time, metrics_synced, commands_processed)
	VALUES (1, datetime('now'), 0, 0);
	`

	_, err := c.db.Exec(schema)
	return err
}

// prepareStatements creates prepared statements for frequently used queries
// This eliminates repeated query parsing and optimization, improving performance
func (c *SQLiteCache) prepareStatements() error {
	var err error

	// Prepare StoreMetrics statement
	c.stmtStoreMetrics, err = c.db.Prepare(`
		INSERT INTO metrics (
			client_id, timestamp, cpu_usage_percent,
			memory_total, memory_used, memory_available, memory_usage_percent,
			disk_total, disk_used, disk_free, disk_usage_percent,
			network_bytes_received, network_bytes_sent, synced
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare StoreMetrics: %w", err)
	}

	// Prepare GetMetrics statement
	c.stmtGetMetrics, err = c.db.Prepare(`
		SELECT id, client_id, timestamp, cpu_usage_percent,
			memory_total, memory_used, memory_available, memory_usage_percent,
			disk_total, disk_used, disk_free, disk_usage_percent,
			network_bytes_received, network_bytes_sent, synced
		FROM metrics
		WHERE timestamp >= ? AND timestamp <= ?
		ORDER BY timestamp ASC
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare GetMetrics: %w", err)
	}

	// Prepare StoreCommand statement
	c.stmtStoreCommand, err = c.db.Prepare(`
		INSERT INTO commands (command_id, type, payload, status, created_at)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare StoreCommand: %w", err)
	}

	// Prepare GetPendingCommands statement
	c.stmtGetPendingCommands, err = c.db.Prepare(`
		SELECT id, command_id, type, payload, status, result, created_at, completed_at
		FROM commands
		WHERE status IN ('pending', 'in_progress')
		ORDER BY created_at ASC
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare GetPendingCommands: %w", err)
	}

	// Prepare MarkCommandComplete statement
	c.stmtMarkComplete, err = c.db.Prepare(`
		UPDATE commands
		SET status = ?, completed_at = ?
		WHERE command_id = ?
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare MarkCommandComplete: %w", err)
	}

	return nil
}

// StoreMetrics persists metric data to the database
func (c *SQLiteCache) StoreMetrics(ctx context.Context, metrics *Metrics) error {
	if metrics == nil {
		return fmt.Errorf("metrics cannot be nil")
	}

	c.stmtMu.RLock()
	stmt := c.stmtStoreMetrics
	c.stmtMu.RUnlock()

	result, err := stmt.ExecContext(ctx,
		metrics.ClientID,
		metrics.Timestamp,
		metrics.CPUUsagePercent,
		metrics.MemoryTotal,
		metrics.MemoryUsed,
		metrics.MemoryAvailable,
		metrics.MemoryUsagePercent,
		metrics.DiskTotalBytes,
		metrics.DiskUsedBytes,
		metrics.DiskFreeBytes,
		metrics.DiskUsagePercent,
		metrics.NetworkBytesReceived,
		metrics.NetworkBytesSent,
		boolToInt(metrics.Synced),
	)
	if err != nil {
		return fmt.Errorf("failed to insert metrics: %w", err)
	}

	// Update the metrics ID with the auto-generated value
	id, err := result.LastInsertId()
	if err == nil {
		metrics.ID = id
	}

	return nil
}

// GetMetrics retrieves metrics within the specified time range
func (c *SQLiteCache) GetMetrics(ctx context.Context, start, end time.Time) ([]*Metrics, error) {
	c.stmtMu.RLock()
	stmt := c.stmtGetMetrics
	c.stmtMu.RUnlock()

	rows, err := stmt.QueryContext(ctx, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to query metrics: %w", err)
	}
	defer rows.Close()

	// Pre-allocate slice with reasonable capacity to minimize allocations
	metrics := make([]*Metrics, 0, 64)

	for rows.Next() {
		m := &Metrics{}
		var synced int

		err := rows.Scan(
			&m.ID,
			&m.ClientID,
			&m.Timestamp,
			&m.CPUUsagePercent,
			&m.MemoryTotal,
			&m.MemoryUsed,
			&m.MemoryAvailable,
			&m.MemoryUsagePercent,
			&m.DiskTotalBytes,
			&m.DiskUsedBytes,
			&m.DiskFreeBytes,
			&m.DiskUsagePercent,
			&m.NetworkBytesReceived,
			&m.NetworkBytesSent,
			&synced,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan metrics row: %w", err)
		}

		m.Synced = intToBool(synced)
		metrics = append(metrics, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating metrics rows: %w", err)
	}

	return metrics, nil
}

// StoreCommand persists a command for execution
func (c *SQLiteCache) StoreCommand(ctx context.Context, cmd *Command) error {
	if cmd == nil {
		return fmt.Errorf("command cannot be nil")
	}

	if cmd.CommandID == "" {
		return fmt.Errorf("command ID cannot be empty")
	}

	// Default status to pending if not set
	if cmd.Status == "" {
		cmd.Status = StatusPending
	}

	c.stmtMu.RLock()
	stmt := c.stmtStoreCommand
	c.stmtMu.RUnlock()

	result, err := stmt.ExecContext(ctx,
		cmd.CommandID,
		cmd.Type,
		cmd.Payload,
		cmd.Status,
		cmd.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert command: %w", err)
	}

	// Update the command ID with the auto-generated value
	id, err := result.LastInsertId()
	if err == nil {
		cmd.ID = id
	}

	return nil
}

// GetPendingCommands retrieves all commands that haven't been completed
func (c *SQLiteCache) GetPendingCommands(ctx context.Context) ([]*Command, error) {
	c.stmtMu.RLock()
	stmt := c.stmtGetPendingCommands
	c.stmtMu.RUnlock()

	rows, err := stmt.QueryContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending commands: %w", err)
	}
	defer rows.Close()

	// Pre-allocate with reasonable capacity
	commands := make([]*Command, 0, 16)

	for rows.Next() {
		cmd := &Command{}
		var completedAt sql.NullTime
		var result sql.NullString

		err := rows.Scan(
			&cmd.ID,
			&cmd.CommandID,
			&cmd.Type,
			&cmd.Payload,
			&cmd.Status,
			&result,
			&cmd.CreatedAt,
			&completedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan command row: %w", err)
		}

		if completedAt.Valid {
			cmd.CompletedAt = completedAt.Time
		}
		if result.Valid {
			cmd.Result = result.String
		}

		commands = append(commands, cmd)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating command rows: %w", err)
	}

	return commands, nil
}

// MarkCommandComplete marks a command as executed
func (c *SQLiteCache) MarkCommandComplete(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("command ID cannot be empty")
	}

	c.stmtMu.RLock()
	stmt := c.stmtMarkComplete
	c.stmtMu.RUnlock()

	result, err := stmt.ExecContext(ctx, StatusCompleted, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to mark command complete: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("command not found: %s", id)
	}

	return nil
}

// Close releases all resources held by the cache
func (c *SQLiteCache) Close() error {
	// Close prepared statements
	c.stmtMu.Lock()
	if c.stmtStoreMetrics != nil {
		c.stmtStoreMetrics.Close()
	}
	if c.stmtGetMetrics != nil {
		c.stmtGetMetrics.Close()
	}
	if c.stmtStoreCommand != nil {
		c.stmtStoreCommand.Close()
	}
	if c.stmtGetPendingCommands != nil {
		c.stmtGetPendingCommands.Close()
	}
	if c.stmtMarkComplete != nil {
		c.stmtMarkComplete.Close()
	}
	c.stmtMu.Unlock()

	// Close database connection
	if c.db != nil {
		return c.db.Close()
	}

	return nil
}

// periodicCleanup runs a background task to remove old data
func (c *SQLiteCache) periodicCleanup() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		c.cleanup()
	}
}

// cleanup removes old metrics and completed commands based on retention policy
func (c *SQLiteCache) cleanup() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cutoff := time.Now().AddDate(0, 0, -c.retentionDays)

	// Delete old synced metrics
	_, err := c.db.ExecContext(ctx,
		"DELETE FROM metrics WHERE synced = 1 AND timestamp < ?",
		cutoff)
	if err != nil {
		c.logger.Error().Err(err).Msg("failed to cleanup old metrics")
	}

	// Delete old completed commands
	_, err = c.db.ExecContext(ctx,
		"DELETE FROM commands WHERE status = 'completed' AND completed_at < ?",
		cutoff)
	if err != nil {
		c.logger.Error().Err(err).Msg("failed to cleanup old commands")
	}

	// Run VACUUM periodically to reclaim space (once per cleanup cycle)
	// Note: VACUUM can be expensive, so we only do it occasionally
	_, err = c.db.ExecContext(ctx, "PRAGMA optimize")
	if err != nil {
		c.logger.Error().Err(err).Msg("failed to optimize database")
	}
}

// Helper functions for boolean to integer conversion
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func intToBool(i int) bool {
	return i != 0
}

// ensureDir creates a directory if it doesn't exist
func ensureDir(dir string) error {
	return ensureDirImpl(dir)
}
