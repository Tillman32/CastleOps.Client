# Cache Layer Quick Start Guide

## Installation

The cache package is already part of the CastleOps.Client module. No additional installation required.

## 5-Minute Quickstart

### 1. Create a Cache Instance

```go
import (
    "context"
    "github.com/castleops/client/internal/cache"
    "github.com/castleops/client/internal/config"
    "github.com/rs/zerolog"
)

// Load config and create cache
cfg := config.Get()
logger := zerolog.New(os.Stdout)
cache, err := cache.New(cfg, logger)
if err != nil {
    log.Fatal(err)
}
defer cache.Close()
```

### 2. Store Metrics

```go
ctx := context.Background()

metrics := &cache.Metrics{
    ClientID:             "client-123",
    Timestamp:            time.Now(),
    CPUUsagePercent:      45.5,
    MemoryTotal:          16 * 1024 * 1024 * 1024,
    MemoryUsed:           8 * 1024 * 1024 * 1024,
    MemoryAvailable:      8 * 1024 * 1024 * 1024,
    MemoryUsagePercent:   50.0,
    DiskTotalBytes:       500 * 1024 * 1024 * 1024,
    DiskUsedBytes:        250 * 1024 * 1024 * 1024,
    DiskFreeBytes:        250 * 1024 * 1024 * 1024,
    DiskUsagePercent:     50.0,
    NetworkBytesReceived: 1024 * 1024 * 100,
    NetworkBytesSent:     1024 * 1024 * 50,
}

err = cache.StoreMetrics(ctx, metrics)
```

### 3. Query Metrics

```go
// Get last hour of metrics
start := time.Now().Add(-1 * time.Hour)
end := time.Now()

metrics, err := cache.GetMetrics(ctx, start, end)
for _, m := range metrics {
    fmt.Printf("CPU: %.1f%%, Memory: %.1f%%\n",
        m.CPUUsagePercent, m.MemoryUsagePercent)
}
```

### 4. Work with Commands

```go
// Store a command
cmd := &cache.Command{
    CommandID: "cmd-456",
    Type:      "install_package",
    Payload:   `{"package": "nginx", "version": "latest"}`,
    Status:    cache.StatusPending,
    CreatedAt: time.Now(),
}
cache.StoreCommand(ctx, cmd)

// Get pending commands
pending, err := cache.GetPendingCommands(ctx)
for _, cmd := range pending {
    fmt.Printf("Command: %s (%s)\n", cmd.CommandID, cmd.Type)

    // Process command...

    // Mark complete
    cache.MarkCommandComplete(ctx, cmd.CommandID)
}
```

## Common Patterns

### Batch Store Metrics (3x Faster)

```go
metrics := make([]*cache.Metrics, 100)
for i := 0; i < 100; i++ {
    metrics[i] = collectMetrics()
}

count, err := cache.BatchStoreMetrics(ctx, cache, metrics)
fmt.Printf("Stored %d metrics\n", count)
```

### Testing with In-Memory Cache

```go
func TestMyFunction(t *testing.T) {
    cache := cache.NewMemoryCache(cache.MemoryConfig{
        RetentionDays: 1,
        Logger:        testLogger(),
    })
    defer cache.Close()

    // Use cache in tests...
}
```

### Force SQLite Backend

```go
cache, err := cache.NewSQLiteCache(cache.SQLiteConfig{
    Path:          "/custom/path/cache.db",
    RetentionDays: 30,
    Logger:        logger,
})
```

## Configuration

### config.yaml

```yaml
cache:
  type: "sqlite"                         # or "memory"
  path: "/var/lib/castleops/cache.db"

metrics:
  retention_days: 7
```

### Environment Variables

```bash
export CASTLEOPS_CACHE_TYPE=sqlite
export CASTLEOPS_CACHE_PATH=/var/lib/castleops/cache.db
export CASTLEOPS_METRICS_RETENTION_DAYS=7
```

## Performance Tips

1. **Use batch operations** for storing multiple metrics
2. **Query specific time ranges** instead of retrieving all data
3. **Close cache properly** to release resources
4. **Use context** for cancellation support
5. **Monitor retention period** to balance storage and data availability

## Troubleshooting

### SQLite fails to initialize

**Error**: "failed to initialize SQLite cache"

**Solution**: Check directory permissions, ensure path is writable. System will automatically fall back to in-memory cache.

### High memory usage

**Issue**: In-memory cache growing too large

**Solution**: Reduce retention_days or switch to SQLite backend

### Lock timeout errors

**Error**: "database is locked"

**Solution**: Increase `_busy_timeout` in connection string or reduce concurrent write load

## API Reference

### Cache Interface

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

### Helper Functions

```go
// Factory function (automatic backend selection)
func New(cfg *config.Config, logger zerolog.Logger) (Cache, error)

// Batch operations
func BatchStoreMetrics(ctx context.Context, cache Cache, metrics []*Metrics) (int, error)
func BatchStoreCommands(ctx context.Context, cache Cache, commands []*Command) (int, error)
```

## Next Steps

- Read the [full README](README.md) for detailed documentation
- Check [example_test.go](example_test.go) for more usage examples
- Review [CACHE_IMPLEMENTATION_SUMMARY.md](../../CACHE_IMPLEMENTATION_SUMMARY.md) for architecture details
- Run benchmarks: `go test -bench=. -benchmem`
