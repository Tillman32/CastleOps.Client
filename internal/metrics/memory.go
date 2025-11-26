package metrics

import (
	"context"
	"fmt"
	"sync"

	"github.com/shirou/gopsutil/v4/mem"
)

// memoryCollector gathers memory usage metrics
// Memory readings are typically stable and don't require heavy smoothing
type memoryCollector struct {
	buffer    *metricsBuffer
	movingAvg *movingAverage
	mu        sync.Mutex
}

// newMemoryCollector creates a new memory metrics collector
// While memory metrics are more stable than CPU, we still apply light smoothing
// to handle temporary spikes from cache flushes or memory-intensive operations
func newMemoryCollector(buffer *metricsBuffer, windowSize int) *memoryCollector {
	return &memoryCollector{
		buffer:    buffer,
		movingAvg: newMovingAverage(windowSize),
	}
}

// Name returns the collector name for logging and debugging
func (c *memoryCollector) Name() string {
	return "memory"
}

// Collect gathers memory usage metrics
// Memory collection is fast (typically <1ms) as it reads from /proc or system APIs
// No sampling interval is needed unlike CPU metrics
func (c *memoryCollector) Collect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Create a channel for the result to allow context cancellation
	type result struct {
		vmem *mem.VirtualMemoryStat
		err  error
	}
	resultChan := make(chan result, 1)

	// Perform memory collection in a goroutine to allow context cancellation
	go func() {
		vmem, err := mem.VirtualMemoryWithContext(ctx)
		resultChan <- result{vmem: vmem, err: err}
	}()

	// Wait for result or context cancellation
	select {
	case res := <-resultChan:
		if res.err != nil {
			return fmt.Errorf("memory collection failed: %w", res.err)
		}

		vmem := res.vmem

		// Apply moving average to usage percentage
		// This smooths out temporary spikes from cache operations
		smoothedPercent := c.movingAvg.Add(vmem.UsedPercent)

		// Update metrics buffer
		// Note: Available may include cached memory that can be freed
		// Different OSes report this differently (especially Windows vs Unix)
		c.buffer.updateMemory(
			vmem.Total,
			vmem.Used,
			vmem.Available,
			smoothedPercent,
		)

		return nil

	case <-ctx.Done():
		// Context cancelled, return error
		// We don't have a fallback like CPU because memory reads are fast
		return ctx.Err()
	}
}

// Reset clears the moving average history
func (c *memoryCollector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.movingAvg.Reset()
}

// GetSwapInfo returns swap/page file information
// This is separate from virtual memory and may not be needed for all use cases
// Call this explicitly when swap information is required
func GetSwapInfo(ctx context.Context) (*SwapInfo, error) {
	swap, err := mem.SwapMemoryWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get swap info: %w", err)
	}

	return &SwapInfo{
		Total:       swap.Total,
		Used:        swap.Used,
		Free:        swap.Free,
		UsedPercent: swap.UsedPercent,
		Sin:         swap.Sin,  // bytes swapped in from disk
		Sout:        swap.Sout, // bytes swapped out to disk
	}, nil
}

// SwapInfo holds swap/page file information
// Swap activity can indicate memory pressure
type SwapInfo struct {
	Total       uint64
	Used        uint64
	Free        uint64
	UsedPercent float64
	Sin         uint64 // Swap in (from disk to memory)
	Sout        uint64 // Swap out (from memory to disk)
}

// GetMemoryInfo returns detailed memory statistics
// This provides a richer view than the basic metrics stored in cache.Metrics
// Use this for diagnostic purposes or detailed reporting
func GetMemoryInfo(ctx context.Context) (*MemoryInfo, error) {
	vmem, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get virtual memory: %w", err)
	}

	info := &MemoryInfo{
		Total:       vmem.Total,
		Available:   vmem.Available,
		Used:        vmem.Used,
		UsedPercent: vmem.UsedPercent,
		Free:        vmem.Free,
	}

	// Platform-specific fields
	// These may not be available on all platforms
	// gopsutil handles this internally and returns 0 for unavailable metrics

	// Buffers: memory used for buffering I/O (Unix-like systems)
	info.Buffers = vmem.Buffers

	// Cached: memory used for disk caching (Unix-like systems)
	info.Cached = vmem.Cached

	// Active: recently used memory (Unix-like systems)
	info.Active = vmem.Active

	// Inactive: memory not recently used (Unix-like systems)
	info.Inactive = vmem.Inactive

	// Wired: memory that cannot be paged out (macOS, BSD)
	info.Wired = vmem.Wired

	return info, nil
}

// MemoryInfo provides detailed memory statistics
// Not all fields are available on all platforms
// Platform-specific fields will be 0 when not available
type MemoryInfo struct {
	// Universal fields (available on all platforms)
	Total       uint64  // Total physical memory
	Available   uint64  // Available memory (may include cached memory)
	Used        uint64  // Memory in use
	UsedPercent float64 // Percentage of memory in use
	Free        uint64  // Completely free memory

	// Unix-like systems (Linux, macOS, BSD)
	Buffers  uint64 // Memory used for I/O buffers
	Cached   uint64 // Memory used for disk cache
	Active   uint64 // Recently used memory
	Inactive uint64 // Memory not recently used

	// macOS/BSD specific
	Wired uint64 // Memory that cannot be paged out
}
