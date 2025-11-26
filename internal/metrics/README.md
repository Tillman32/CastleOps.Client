# Metrics Collection System

High-performance, cross-platform metrics collection system for CastleOps.Client.

## Features

- **Low Overhead**: Target <1% CPU during collection, <10MB memory footprint
- **Cross-Platform**: Full support for macOS and Windows via gopsutil
- **Non-Blocking**: All collectors run concurrently with context-based cancellation
- **Smoothed Data**: Moving average calculations reduce noise in metrics
- **Zero-Allocation Design**: Object pooling and efficient buffer reuse minimize GC pressure
- **Thread-Safe**: Safe for concurrent access from multiple goroutines
- **Modular**: Enable/disable individual collectors as needed

## Performance Characteristics

Based on benchmark results (Apple M1 Pro):

| Operation | Time/op | Allocations | Notes |
|-----------|---------|-------------|-------|
| CPU Collection | ~101ms | 83 allocs | Includes 100ms sampling interval |
| Memory Collection | ~33μs | 49 allocs | Very fast, no sampling needed |
| Disk Collection | ~5μs | 7 allocs | Fastest collector |
| Network Collection | ~6ms | 257 allocs | Interface enumeration overhead |
| Full System Collection | ~101ms | 417 allocs | All collectors concurrent |
| Moving Average Add | ~46ns | 0 allocs | Zero-allocation design |
| Metrics Pooling | ~22ns | 0 allocs | 8x faster than allocation |

**CPU Usage**: <1% during 60-second collection intervals
**Memory Usage**: ~5-10MB steady state with pooling
**Collection Duration**: ~100ms (dominated by CPU sampling interval)

## Architecture

```
┌─────────────────────────────────────────────────┐
│              System (Orchestrator)              │
│  - Manages lifecycle (start/stop)              │
│  - Coordinates collectors                       │
│  - Handles callbacks                            │
└────────────┬────────────────────────────────────┘
             │
             ├─────────────────┬─────────────────┬─────────────────┐
             ▼                 ▼                 ▼                 ▼
     ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐
     │CPU Collector │  │Mem Collector │  │Disk Collector│  │Net Collector │
     │- Sampling    │  │- Virtual Mem │  │- Usage Stats │  │- Byte Counts │
     │- Moving Avg  │  │- Moving Avg  │  │- Moving Avg  │  │- Cumulative  │
     └──────┬───────┘  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘
            │                 │                 │                 │
            └─────────────────┴─────────────────┴─────────────────┘
                                      │
                              ┌───────▼────────┐
                              │ Metrics Buffer │
                              │  (Thread-Safe) │
                              └───────┬────────┘
                                      │
                              ┌───────▼────────┐
                              │ cache.Metrics  │
                              │   (Pooled)     │
                              └────────────────┘
```

## Quick Start

### Basic Usage

```go
import (
    "github.com/castleops/client/internal/metrics"
    "github.com/rs/zerolog"
)

// Create system with defaults
logger := zerolog.New(os.Stdout)
config := metrics.DefaultConfig()
system, err := metrics.NewSystem(config, logger, "client-123")
if err != nil {
    panic(err)
}

// Collect once
ctx := context.Background()
m, err := system.CollectNow(ctx)
if err != nil {
    panic(err)
}

fmt.Printf("CPU: %.2f%%\n", m.CPUUsagePercent)
```

### Periodic Collection

```go
// Start background collection
system.SetMetricsCallback(func(ctx context.Context, m *cache.Metrics) error {
    // Store in cache or send to server
    return yourCache.StoreMetrics(ctx, m)
})

ctx := context.Background()
system.Start(ctx)

// Later...
system.Stop()
```

### Custom Configuration

```go
config := &metrics.Config{
    CollectionInterval:  30 * time.Second,  // How often to collect
    MovingAverageWindow: 10,                 // Smoothing window size
    EnableCPU:           true,
    EnableMemory:        true,
    EnableDisk:          true,
    EnableNetwork:       false,              // Disable if not needed
}

system, err := metrics.NewSystem(config, logger, "client-123")
```

## Configuration Options

### Config Structure

```go
type Config struct {
    // CollectionInterval specifies how often to collect metrics
    // Default: 60 seconds
    // Minimum: 1 second (not recommended, increases CPU usage)
    CollectionInterval time.Duration

    // MovingAverageWindow specifies how many samples to average
    // Default: 5 samples
    // Set to 1 to disable smoothing
    // Higher values = smoother but less responsive
    MovingAverageWindow int

    // Individual collector toggles
    EnableCPU     bool
    EnableMemory  bool
    EnableDisk    bool
    EnableNetwork bool
}
```

### Default Configuration

```go
&Config{
    CollectionInterval:  60 * time.Second,
    MovingAverageWindow: 5,
    EnableCPU:           true,
    EnableMemory:        true,
    EnableDisk:          true,
    EnableNetwork:       true,
}
```

## Collectors

### CPU Collector

**What it measures:**
- Overall system CPU usage percentage
- Averaged across all cores/threads

**Performance:**
- Uses 100ms sampling interval for accuracy
- Applies moving average to smooth spikes
- ~101ms per collection (sampling interval)
- 83 allocations per collection

**Notes:**
- First collection may return 0% (no historical data)
- On context timeout, returns last known value
- Cross-platform via gopsutil

**Example:**
```go
info, _ := metrics.GetCPUInfo(ctx)
fmt.Printf("CPU: %s (%d cores)\n", info.ModelName, info.Cores)
```

### Memory Collector

**What it measures:**
- Total physical memory
- Used memory
- Available memory (may include cache)
- Usage percentage

**Performance:**
- Very fast: ~33μs per collection
- 49 allocations per collection
- No sampling delay

**Notes:**
- "Available" includes cached memory on Unix systems
- Windows and Unix report differently
- Moving average smooths cache flushes

**Example:**
```go
info, _ := metrics.GetMemoryInfo(ctx)
fmt.Printf("Memory: %d GB total\n", info.Total/1024/1024/1024)
```

### Disk Collector

**What it measures:**
- Disk space usage for primary mount point
- Total, used, free bytes
- Usage percentage

**Performance:**
- Fastest collector: ~5μs per collection
- Only 7 allocations per collection
- Minimal overhead

**Configuration:**
- Default: "/" on Unix, "C:" on Windows
- Can monitor any mount point
- Use `GetAllPartitions()` to discover mounts

**Example:**
```go
partitions, _ := metrics.GetAllPartitions(ctx)
for _, p := range partitions {
    fmt.Printf("%s: %.2f%% used\n", p.Mountpoint, p.UsedPercent)
}
```

### Network Collector

**What it measures:**
- Cumulative bytes received (since boot)
- Cumulative bytes sent (since boot)
- Aggregated across all interfaces

**Performance:**
- ~6ms per collection
- 257 allocations (interface enumeration)
- Moderate overhead

**Notes:**
- First collection initializes counters (returns 0)
- Stores cumulative values, not deltas
- Consumer calculates rates between samples
- Handles counter wraps gracefully

**Example:**
```go
interfaces, _ := metrics.GetNetworkInterfaces(ctx)
for _, iface := range interfaces {
    if iface.IsUp() && !iface.IsLoopback() {
        fmt.Printf("%s: %d bytes\n", iface.Name, iface.BytesReceived)
    }
}
```

## Advanced Usage

### Context Cancellation

All collectors respect context cancellation:

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

system.Start(ctx)
<-ctx.Done()
system.Stop()
```

### Error Handling

Individual collector failures don't stop collection:

```go
system.SetMetricsCallback(func(ctx context.Context, m *cache.Metrics) error {
    if err := store(m); err != nil {
        logger.Error().Err(err).Msg("Storage failed")
        return err // Logged but doesn't stop collection
    }
    return nil
})
```

### Dynamic Configuration

Update config without restart:

```go
newConfig := system.GetConfig()
newConfig.CollectionInterval = 30 * time.Second
system.UpdateConfig(&newConfig)
```

**Note:** Changes to Enable* flags require restart to take effect.

### Manual Reset

Clear moving average history:

```go
system.ResetCollectors() // Start fresh measurements
```

### Statistics

Get runtime stats:

```go
stats := system.Stats()
fmt.Printf("Running: %v, Collectors: %d\n",
    stats.Running,
    stats.CollectorCount)
```

## Integration with Cache Layer

The metrics system outputs `cache.Metrics` structures:

```go
type Metrics struct {
    // Network
    NetworkBytesReceived uint64
    NetworkBytesSent     uint64

    // Disk
    DiskTotalBytes   uint64
    DiskUsedBytes    uint64
    DiskFreeBytes    uint64

    // Memory
    MemoryTotal     uint64
    MemoryUsed      uint64
    MemoryAvailable uint64

    // Metadata
    Timestamp time.Time
    ClientID  string

    // Percentages
    CPUUsagePercent    float64
    DiskUsagePercent   float64
    MemoryUsagePercent float64

    // Database fields
    ID     int64
    Synced bool
}
```

### Storage Integration

```go
system.SetMetricsCallback(func(ctx context.Context, m *cache.Metrics) error {
    return cache.StoreMetrics(ctx, m)
})
```

## Performance Optimization

### Object Pooling

Metrics structures are pooled to reduce allocations:

```go
// Internal implementation (automatic)
m := getMetrics()  // From pool
defer putMetrics(m) // Return to pool
```

**Result:** 8x faster than allocation, 0 heap allocations

### Moving Averages

Circular buffer design for O(1) operations:

```go
// Zero-allocation moving average
ma := newMovingAverage(10)
smoothed := ma.Add(cpuPercent) // 46ns, 0 allocs
```

### Concurrent Collection

All collectors run in parallel:

```go
// Happens automatically
// Duration = max(collector_times), not sum(collector_times)
// ~101ms total instead of ~107ms sequential
```

### Minimal Lock Contention

Fine-grained locking in buffer and moving averages:

- Read operations use RLock (concurrent)
- Write operations use Lock (exclusive)
- Lock-free atomic for state flags

## Cross-Platform Support

### macOS (darwin/arm64, darwin/amd64)

✅ CPU metrics (percent, cores, frequency)
✅ Memory metrics (physical, available, cached)
✅ Disk metrics (all mount points)
✅ Network metrics (all interfaces)
ℹ️ Uses sysctl and /proc-like interfaces

### Windows (windows/amd64)

✅ CPU metrics (percent, cores, frequency)
✅ Memory metrics (physical, available)
✅ Disk metrics (drive letters)
✅ Network metrics (all adapters)
ℹ️ Uses WMI and Windows Performance Counters

### Platform Differences

| Metric | macOS | Windows | Notes |
|--------|-------|---------|-------|
| Memory.Available | Includes cache | Physical only | Unix includes freeable cache |
| Memory.Wired | ✅ | ❌ | macOS-specific (non-pageable) |
| Disk.Mountpoint | "/" | "C:" | Use getDefaultMountPoint() |
| Network.Interfaces | en0, en1, etc | Ethernet, WiFi | Use GetNetworkInterfaces() |

## Testing

### Unit Tests

```bash
go test -v ./internal/metrics/
```

### Benchmarks

```bash
go test -bench=. -benchmem ./internal/metrics/
```

### Race Detection

```bash
go test -race ./internal/metrics/
```

### Coverage

```bash
go test -cover ./internal/metrics/
```

## Troubleshooting

### High CPU Usage

**Symptom:** Metrics collection using >1% CPU

**Solutions:**
- Increase `CollectionInterval` (default: 60s)
- Disable unused collectors
- Reduce `MovingAverageWindow` size
- Check for CPU collector sampling interval

### High Memory Usage

**Symptom:** Memory usage growing over time

**Solutions:**
- Verify metrics callback is consuming data
- Check for goroutine leaks (use pprof)
- Ensure `Stop()` is called on shutdown
- Monitor with `runtime.ReadMemStats()`

### Collection Timeouts

**Symptom:** Context deadline exceeded errors

**Solutions:**
- Increase context timeout (default: 10s)
- Check network connectivity (network collector)
- Verify disk mount points are accessible
- Use shorter CPU sampling interval

### Inaccurate Metrics

**Symptom:** CPU/memory values seem wrong

**Solutions:**
- Check `MovingAverageWindow` (may be over-smoothing)
- Compare with system monitor (Activity Monitor, Task Manager)
- Verify first collection (may need warmup)
- Reset collectors after config changes

## Best Practices

1. **Collection Interval**: 60s for production, 10s for development
2. **Moving Average**: 5-10 samples for stable metrics
3. **Error Handling**: Always check errors, log but continue
4. **Context Usage**: Use context.Background() for long-running collection
5. **Graceful Shutdown**: Always call Stop() before exit
6. **Callback Performance**: Keep callback fast (<100ms), offload to queue if needed
7. **Platform Testing**: Test on both macOS and Windows
8. **Resource Limits**: Monitor with pprof in production

## Future Enhancements

Potential additions (not yet implemented):

- [ ] Per-core CPU metrics
- [ ] Process-level metrics (PID, CPU, memory per process)
- [ ] GPU metrics (if available)
- [ ] Temperature sensors
- [ ] Battery status (laptops)
- [ ] Custom collector plugin system
- [ ] Prometheus exporter
- [ ] Metrics aggregation service

## Dependencies

- `github.com/shirou/gopsutil/v4` - Cross-platform system metrics
- `github.com/rs/zerolog` - Structured logging
- Standard library only otherwise

## License

Part of CastleOps.Client project.
