package cache

import (
	"context"
	"fmt"
)

// BatchStoreMetrics stores multiple metrics efficiently
// For SQLite: uses a transaction to reduce commit overhead
// For Memory: simply calls StoreMetrics multiple times
// Returns the number of metrics successfully stored and any error
func BatchStoreMetrics(ctx context.Context, cache Cache, metrics []*Metrics) (int, error) {
	if len(metrics) == 0 {
		return 0, nil
	}

	// Check if this is a SQLite cache - if so, use transaction
	if sqliteCache, ok := cache.(*SQLiteCache); ok {
		return batchStoreSQLite(ctx, sqliteCache, metrics)
	}

	// For other implementations, store one at a time
	return batchStoreSequential(ctx, cache, metrics)
}

// batchStoreSQLite uses a transaction for efficient batch insertion
func batchStoreSQLite(ctx context.Context, cache *SQLiteCache, metrics []*Metrics) (int, error) {
	// Begin transaction
	tx, err := cache.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Ensure transaction is rolled back on error
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Prepare statement within transaction
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO metrics (
			client_id, timestamp, cpu_usage_percent,
			memory_total, memory_used, memory_available, memory_usage_percent,
			disk_total, disk_used, disk_free, disk_usage_percent,
			network_bytes_received, network_bytes_sent, synced
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return 0, fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	// Insert all metrics
	count := 0
	for _, m := range metrics {
		if m == nil {
			continue
		}

		_, err = stmt.ExecContext(ctx,
			m.ClientID,
			m.Timestamp,
			m.CPUUsagePercent,
			m.MemoryTotal,
			m.MemoryUsed,
			m.MemoryAvailable,
			m.MemoryUsagePercent,
			m.DiskTotalBytes,
			m.DiskUsedBytes,
			m.DiskFreeBytes,
			m.DiskUsagePercent,
			m.NetworkBytesReceived,
			m.NetworkBytesSent,
			boolToInt(m.Synced),
		)
		if err != nil {
			return count, fmt.Errorf("failed to insert metric: %w", err)
		}
		count++
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return count, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return count, nil
}

// batchStoreSequential stores metrics one at a time
func batchStoreSequential(ctx context.Context, cache Cache, metrics []*Metrics) (int, error) {
	count := 0
	for _, m := range metrics {
		if m == nil {
			continue
		}

		if err := cache.StoreMetrics(ctx, m); err != nil {
			return count, fmt.Errorf("failed to store metric %d: %w", count, err)
		}
		count++
	}

	return count, nil
}

// BatchStoreCommands stores multiple commands efficiently
// For SQLite: uses a transaction to reduce commit overhead
// For Memory: simply calls StoreCommand multiple times
// Returns the number of commands successfully stored and any error
func BatchStoreCommands(ctx context.Context, cache Cache, commands []*Command) (int, error) {
	if len(commands) == 0 {
		return 0, nil
	}

	// Check if this is a SQLite cache - if so, use transaction
	if sqliteCache, ok := cache.(*SQLiteCache); ok {
		return batchStoreCommandsSQLite(ctx, sqliteCache, commands)
	}

	// For other implementations, store one at a time
	return batchStoreCommandsSequential(ctx, cache, commands)
}

// batchStoreCommandsSQLite uses a transaction for efficient batch insertion
func batchStoreCommandsSQLite(ctx context.Context, cache *SQLiteCache, commands []*Command) (int, error) {
	// Begin transaction
	tx, err := cache.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Ensure transaction is rolled back on error
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Prepare statement within transaction
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO commands (command_id, type, payload, status, created_at)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return 0, fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	// Insert all commands
	count := 0
	for _, cmd := range commands {
		if cmd == nil {
			continue
		}

		status := cmd.Status
		if status == "" {
			status = StatusPending
		}

		_, err = stmt.ExecContext(ctx,
			cmd.CommandID,
			cmd.Type,
			cmd.Payload,
			status,
			cmd.CreatedAt,
		)
		if err != nil {
			return count, fmt.Errorf("failed to insert command: %w", err)
		}
		count++
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return count, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return count, nil
}

// batchStoreCommandsSequential stores commands one at a time
func batchStoreCommandsSequential(ctx context.Context, cache Cache, commands []*Command) (int, error) {
	count := 0
	for _, cmd := range commands {
		if cmd == nil {
			continue
		}

		if err := cache.StoreCommand(ctx, cmd); err != nil {
			return count, fmt.Errorf("failed to store command %d: %w", count, err)
		}
		count++
	}

	return count, nil
}
