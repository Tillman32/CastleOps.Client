package metrics

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"github.com/shirou/gopsutil/v4/disk"
)

// diskCollector gathers disk usage metrics
// Focuses on the primary system disk to avoid overhead of scanning all partitions
type diskCollector struct {
	buffer     *metricsBuffer
	movingAvg  *movingAverage
	mountPoint string // Primary disk mount point to monitor
	mu         sync.Mutex
}

// newDiskCollector creates a new disk metrics collector
// The mountPoint specifies which disk to monitor
// Use "/" for Unix-like systems and "C:" for Windows
func newDiskCollector(buffer *metricsBuffer, windowSize int, mountPoint string) *diskCollector {
	// Auto-detect mount point if not specified
	if mountPoint == "" {
		mountPoint = getDefaultMountPoint()
	}

	return &diskCollector{
		buffer:     buffer,
		movingAvg:  newMovingAverage(windowSize),
		mountPoint: mountPoint,
	}
}

// getDefaultMountPoint returns the platform-appropriate default mount point
func getDefaultMountPoint() string {
	if runtime.GOOS == "windows" {
		return "C:"
	}
	return "/"
}

// Name returns the collector name for logging and debugging
func (c *diskCollector) Name() string {
	return "disk"
}

// Collect gathers disk usage metrics for the configured mount point
// Disk usage typically changes slowly, so readings are very stable
func (c *diskCollector) Collect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Create a channel for the result to allow context cancellation
	type result struct {
		usage *disk.UsageStat
		err   error
	}
	resultChan := make(chan result, 1)

	// Perform disk collection in a goroutine to allow context cancellation
	go func() {
		usage, err := disk.UsageWithContext(ctx, c.mountPoint)
		resultChan <- result{usage: usage, err: err}
	}()

	// Wait for result or context cancellation
	select {
	case res := <-resultChan:
		if res.err != nil {
			return fmt.Errorf("disk collection failed for %s: %w", c.mountPoint, res.err)
		}

		usage := res.usage

		// Apply moving average to usage percentage
		// Disk usage is very stable, but we smooth anyway for consistency
		smoothedPercent := c.movingAvg.Add(usage.UsedPercent)

		// Update metrics buffer
		c.buffer.updateDisk(
			usage.Total,
			usage.Used,
			usage.Free,
			smoothedPercent,
		)

		return nil

	case <-ctx.Done():
		// Context cancelled, return error
		return ctx.Err()
	}
}

// Reset clears the moving average history
func (c *diskCollector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.movingAvg.Reset()
}

// SetMountPoint changes the monitored mount point
// Use this to switch between different disks without recreating the collector
func (c *diskCollector) SetMountPoint(mountPoint string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.mountPoint = mountPoint
	c.movingAvg.Reset() // Reset averages when changing disks
}

// GetMountPoint returns the currently monitored mount point
func (c *diskCollector) GetMountPoint() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.mountPoint
}

// GetAllPartitions returns information about all disk partitions
// This is useful for discovering available mount points
// Note: This can be expensive on systems with many partitions/mounts
func GetAllPartitions(ctx context.Context) ([]PartitionInfo, error) {
	partitions, err := disk.PartitionsWithContext(ctx, false) // false = don't include pseudo-filesystems
	if err != nil {
		return nil, fmt.Errorf("failed to get partitions: %w", err)
	}

	result := make([]PartitionInfo, 0, len(partitions))
	for _, p := range partitions {
		// Get usage for this partition
		usage, err := disk.UsageWithContext(ctx, p.Mountpoint)
		if err != nil {
			// Skip partitions we can't read (may be unmounted or inaccessible)
			continue
		}

		result = append(result, PartitionInfo{
			Device:      p.Device,
			Mountpoint:  p.Mountpoint,
			FSType:      p.Fstype,
			Opts:        p.Opts,
			Total:       usage.Total,
			Used:        usage.Used,
			Free:        usage.Free,
			UsedPercent: usage.UsedPercent,
		})
	}

	return result, nil
}

// PartitionInfo holds information about a disk partition
type PartitionInfo struct {
	Device      string   // Device name (e.g., "/dev/sda1", "C:")
	Mountpoint  string   // Where it's mounted (e.g., "/", "C:")
	FSType      string   // Filesystem type (e.g., "ext4", "NTFS")
	Opts        []string // Mount options
	Total       uint64   // Total bytes
	Used        uint64   // Used bytes
	Free        uint64   // Free bytes
	UsedPercent float64  // Usage percentage
}

// GetDiskIO returns I/O statistics for all disks
// This provides read/write throughput information
// Note: Counter values are cumulative since boot and must be diffed over time
func GetDiskIO(ctx context.Context) (map[string]*DiskIOStats, error) {
	ioCounters, err := disk.IOCountersWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get disk I/O stats: %w", err)
	}

	result := make(map[string]*DiskIOStats, len(ioCounters))
	for name, counter := range ioCounters {
		result[name] = &DiskIOStats{
			Name:         name,
			ReadCount:    counter.ReadCount,
			WriteCount:   counter.WriteCount,
			ReadBytes:    counter.ReadBytes,
			WriteBytes:   counter.WriteBytes,
			ReadTime:     counter.ReadTime,
			WriteTime:    counter.WriteTime,
			IoTime:       counter.IoTime,
			MergedReads:  counter.MergedReadCount,
			MergedWrites: counter.MergedWriteCount,
		}
	}

	return result, nil
}

// DiskIOStats holds I/O statistics for a disk
// All counters are cumulative since boot
// To get rates, you must diff between two samples
type DiskIOStats struct {
	Name         string // Disk name
	ReadCount    uint64 // Number of read operations
	WriteCount   uint64 // Number of write operations
	ReadBytes    uint64 // Bytes read
	WriteBytes   uint64 // Bytes written
	ReadTime     uint64 // Time spent reading (ms)
	WriteTime    uint64 // Time spent writing (ms)
	IoTime       uint64 // Time spent doing I/O (ms)
	MergedReads  uint64 // Number of merged read operations
	MergedWrites uint64 // Number of merged write operations
}

// DiskIOMonitor tracks disk I/O rates by diffing cumulative counters
// Use this to calculate bytes/sec and operations/sec
type DiskIOMonitor struct {
	lastStats map[string]*DiskIOStats
	mu        sync.RWMutex
}

// NewDiskIOMonitor creates a new disk I/O rate monitor
func NewDiskIOMonitor() *DiskIOMonitor {
	return &DiskIOMonitor{
		lastStats: make(map[string]*DiskIOStats),
	}
}

// Update captures current I/O stats and returns rates since last update
// Returns nil on first call (no previous data to diff against)
func (m *DiskIOMonitor) Update(ctx context.Context) (map[string]*DiskIORates, error) {
	currentStats, err := GetDiskIO(ctx)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// First call - just store stats and return nil
	if len(m.lastStats) == 0 {
		m.lastStats = currentStats
		return nil, nil
	}

	// Calculate rates
	rates := make(map[string]*DiskIORates)
	for name, current := range currentStats {
		last, exists := m.lastStats[name]
		if !exists {
			// New disk appeared, skip for now
			continue
		}

		rates[name] = &DiskIORates{
			Name:       name,
			ReadOps:    current.ReadCount - last.ReadCount,
			WriteOps:   current.WriteCount - last.WriteCount,
			ReadBytes:  current.ReadBytes - last.ReadBytes,
			WriteBytes: current.WriteBytes - last.WriteBytes,
		}
	}

	// Store current stats for next diff
	m.lastStats = currentStats

	return rates, nil
}

// DiskIORates holds disk I/O rates between two samples
type DiskIORates struct {
	Name       string // Disk name
	ReadOps    uint64 // Read operations since last sample
	WriteOps   uint64 // Write operations since last sample
	ReadBytes  uint64 // Bytes read since last sample
	WriteBytes uint64 // Bytes written since last sample
}
