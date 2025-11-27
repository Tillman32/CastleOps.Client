# Cache Package

The cache package provides high-performance persistent storage for the CastleOps client, supporting both SQLite and in-memory backends with automatic fallback.

## Features

- **Dual Backend Support**: SQLite for production, in-memory for testing/fallback
- **High Performance**: Optimized with WAL mode, prepared statements, and connection pooling
- **Thread-Safe**: All operations safe for concurrent access
- **Automatic Cleanup**: Background retention policy enforcement
- **Zero-Downtime Fallback**: Automatically falls back to in-memory if SQLite fails

## Architecture

### Interface

The `Cache` interface defines six core operations:

```go
type Cache interface {
    StoreMetrics(ctx context.Context, metrics *Metrics) error
    GetMetrics(ctx context.Context, start, end time.Time) ([]*Metrics, error)
    StoreCommand(ctx context.Context, cmd *Command) error
    GetPendingCommands(ctx context.Context) ([]*Command, error)
    MarkCommandComplete(ctx context.Context, id string) error
    Close() error
}
```

### Data Models

**Metrics**: System metrics snapshots
- CPU, memory, disk, network statistics
- Timestamp and client ID
- Sync status tracking

**Command**: Commands to execute
- Command ID, type, and payload
- Status tracking (pending, in_progress, completed, failed)
- Creation and completion timestamps

## SQLite Implementation

### Schema

```sql
-- Metrics table
CREATE TABLE metrics (
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

-- Commands table
CREATE TABLE commands (
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

-- Sync status table
CREATE TABLE sync_status (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    last_sync_time DATETIME NOT NULL,
    metrics_synced INTEGER NOT NULL DEFAULT 0,
    commands_processed INTEGER NOT NULL DEFAULT 0,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### Indexes

- `idx_metrics_timestamp`: Time-range queries (most common)
- `idx_metrics_synced`: Finding unsynced metrics
- `idx_metrics_client`: Client-specific queries
- `idx_commands_status`: Pending command queries (hot path)
- `idx_commands_command_id`: Command ID lookups

### Performance Optimizations

**Connection String**:
```
?_journal_mode=WAL              # Write-Ahead Logging for concurrency
&_synchronous=NORMAL            # Balance safety and performance
&_cache_size=-64000             # 64MB cache
&_busy_timeout=5000             # 5s lock wait timeout
&_foreign_keys=ON               # Referential integrity
```

**Connection Pool**:
- Max open connections: 4 (SQLite doesn't benefit from many connections)
- Max idle connections: 2 (keep warm connections ready)
- Connection lifetime: 1 hour
- Idle timeout: 10 minutes

**Prepared Statements**:
All hot-path queries use prepared statements to eliminate parsing overhead:
- StoreMetrics
- GetMetrics
- StoreCommand
- GetPendingCommands
- MarkCommandComplete

**Automatic Cleanup**:
- Runs every hour
- Removes synced metrics older than retention period
- Removes completed commands older than retention period
- Runs PRAGMA optimize for query performance

## In-Memory Implementation

### Features

- **Pure Go**: No external dependencies
- **Thread-Safe**: Single RWMutex for all operations
- **Efficient**: Pre-allocated maps reduce GC pressure
- **Sorted Results**: Automatic ordering by timestamp/creation time

### Storage

```go
metrics  map[int64]*Metrics      // Pre-allocated for ~1024 entries
commands map[string]*Command      // Pre-allocated for ~64 entries
```

### Cleanup

Background goroutine runs hourly to enforce retention policy on both metrics and commands.

## Usage

### Basic Usage

```go
import (
    "github.com/castleops/client/internal/cache"
    "github.com/castleops/client/internal/config"
    "github.com/rs/zerolog"
)

// Create cache from config (automatic backend selection)
cfg := config.Get()
logger := zerolog.New(os.Stdout)
cache, err := cache.New(cfg, logger)
if err != nil {
    log.Fatal(err)
}
defer cache.Close()

// Store metrics
metrics := &cache.Metrics{
    ClientID:             "client-123",
    Timestamp:            time.Now(),
    CPUUsagePercent:      45.5,
    MemoryUsagePercent:   50.0,
    DiskUsagePercent:     60.0,
    // ... other fields
}
cache.StoreMetrics(ctx, metrics)

// Retrieve metrics
start := time.Now().Add(-1 * time.Hour)
end := time.Now()
metrics, err := cache.GetMetrics(ctx, start, end)

// Store command
cmd := &cache.Command{
    CommandID: "cmd-456",
    Type:      "install_package",
    Payload:   `{"package": "nginx"}`,
    Status:    cache.StatusPending,
    CreatedAt: time.Now(),
}
cache.StoreCommand(ctx, cmd)

// Get pending commands
pending, err := cache.GetPendingCommands(ctx)

// Mark command complete
cache.MarkCommandComplete(ctx, "cmd-456")
```

### Direct SQLite Creation

```go
cache, err := cache.NewSQLiteCache(cache.SQLiteConfig{
    Path:          "/var/lib/castleops/cache.db",
    RetentionDays: 7,
    Logger:        logger,
})
```

### Direct In-Memory Creation

```go
cache := cache.NewMemoryCache(cache.MemoryConfig{
    RetentionDays: 7,
    Logger:        logger,
})
```

## Performance Benchmarks

Benchmarks on Apple M1 Pro (results may vary by platform):

```
BenchmarkMemoryCacheStoreMetrics    1,724,912 ops/sec    809 ns/op    188 B/op    1 allocs/op
BenchmarkSQLiteCacheStoreMetrics       12,210 ops/sec  88,652 ns/op  1,280 B/op   30 allocs/op

BenchmarkMemoryCacheGetMetrics          2,496 ops/sec  403,857 ns/op  167 KB/op  1,006 allocs/op
BenchmarkSQLiteCacheGetMetrics            158 ops/sec  6,720,239 ns/op  473 KB/op  25,813 allocs/op
```

**Key Insights**:
- In-memory is ~100x faster for writes, ~17x faster for reads
- SQLite write: ~89μs per metric (11,000+ writes/sec)
- SQLite read: ~6.7ms for 1000 metrics
- Memory cache: <1μs per write, minimal allocations

## Thread Safety

All cache implementations are fully thread-safe:

**SQLiteCache**:
- Prepared statements protected by RWMutex
- SQLite connection pool handles concurrent access
- WAL mode enables concurrent readers with single writer

**MemoryCache**:
- Single RWMutex protects all data structures
- Copies returned from reads prevent mutation races
- Lock-free for most read operations (RLock)

## Error Handling

All operations return meaningful errors:
- Database connection failures
- Constraint violations (duplicate command IDs)
- Context cancellation
- Invalid parameters (nil pointers, empty IDs)

## Testing

Run tests:
```bash
go test ./internal/cache/...
```

Run benchmarks:
```bash
go test ./internal/cache/... -bench=. -benchmem
```

## Configuration

### SQLite Backend

```yaml
cache:
  type: "sqlite"
  path: "/var/lib/castleops/cache.db"

metrics:
  retention_days: 7
```

### In-Memory Backend

```yaml
cache:
  type: "memory"

metrics:
  retention_days: 7
```

## Best Practices

1. **Always use context**: All operations support context cancellation
2. **Close on shutdown**: Call `Close()` to release resources properly
3. **Handle errors**: Don't ignore errors from cache operations
4. **Reasonable retention**: Balance storage vs. data availability (7 days default)
5. **Monitor performance**: Use benchmarks to validate cache performance in your environment

## Future Enhancements

Potential optimizations for future releases:

1. **Batch Operations**: Bulk insert for metrics (reduce transaction overhead)
2. **Compression**: Compress old metrics before deletion
3. **Read-Through Caching**: Add in-memory cache layer on top of SQLite
4. **Partitioning**: Separate databases by time period
5. **Async Writes**: Queue writes for background processing
