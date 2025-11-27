package metrics

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
)

// cpuCollector gathers CPU usage metrics
// Uses moving averages to smooth out spikes and provide stable readings
type cpuCollector struct {
	buffer         *metricsBuffer
	movingAvg      *movingAverage
	lastSampleTime time.Time
	mu             sync.Mutex
}

// newCPUCollector creates a new CPU metrics collector
// The collector uses a moving average window to reduce noise in CPU readings
func newCPUCollector(buffer *metricsBuffer, windowSize int) *cpuCollector {
	return &cpuCollector{
		buffer:         buffer,
		movingAvg:      newMovingAverage(windowSize),
		lastSampleTime: time.Now(),
	}
}

// Name returns the collector name for logging and debugging
func (c *cpuCollector) Name() string {
	return "cpu"
}

// Collect gathers CPU usage metrics
// This implementation uses gopsutil's cpu.Percent which samples CPU over an interval
// For the first call or when context allows, we use a 1-second interval for accuracy
// Subsequent calls with tight deadlines use the last known values to avoid blocking
func (c *cpuCollector) Collect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Determine sampling interval
	// For first-time collection or when we have time, do a proper 1-second sample
	// Otherwise use instant reading to avoid blocking
	interval := 100 * time.Millisecond
	timeSinceLastSample := time.Since(c.lastSampleTime)

	// If it's been a while since last sample, we can afford a longer interval
	if timeSinceLastSample > 5*time.Second {
		interval = 1 * time.Second
	}

	// Check if context allows the sampling interval
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining < interval {
			// Not enough time for full sample, use instant reading
			interval = 0
		}
	}

	// Create a channel for the result to allow context cancellation
	type result struct {
		percent float64
		err     error
	}
	resultChan := make(chan result, 1)

	// Perform CPU sampling in a goroutine to allow context cancellation
	go func() {
		// cpu.Percent samples CPU over the specified interval
		// interval=0 returns instant reading
		// percpu=false returns overall system CPU usage
		percentages, err := cpu.PercentWithContext(ctx, interval, false)
		if err != nil {
			resultChan <- result{err: err}
			return
		}

		if len(percentages) == 0 {
			resultChan <- result{err: fmt.Errorf("no CPU percentage data returned")}
			return
		}

		resultChan <- result{percent: percentages[0], err: nil}
	}()

	// Wait for result or context cancellation
	select {
	case res := <-resultChan:
		if res.err != nil {
			return fmt.Errorf("cpu collection failed: %w", res.err)
		}

		// Apply moving average to smooth out spikes
		smoothed := c.movingAvg.Add(res.percent)

		// Update metrics buffer
		c.buffer.updateCPU(smoothed)
		c.lastSampleTime = time.Now()

		return nil

	case <-ctx.Done():
		// Context cancelled, return the last known value from moving average
		// This ensures we always have a value even if collection is interrupted
		lastKnown := c.movingAvg.Get()
		c.buffer.updateCPU(lastKnown)
		return ctx.Err()
	}
}

// Reset clears the moving average history
// Useful when you want to start fresh measurements
func (c *cpuCollector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.movingAvg.Reset()
	c.lastSampleTime = time.Now()
}

// GetInfo returns static CPU information
// This is separate from metrics collection as it rarely changes
// Call this once during initialization rather than on every collection cycle
func GetCPUInfo(ctx context.Context) (*CPUInfo, error) {
	info, err := cpu.InfoWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get CPU info: %w", err)
	}

	if len(info) == 0 {
		return nil, fmt.Errorf("no CPU info available")
	}

	// Get logical CPU count (includes hyperthreading)
	logicalCount, err := cpu.CountsWithContext(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("failed to get logical CPU count: %w", err)
	}

	// Get physical CPU count (actual cores)
	physicalCount, err := cpu.CountsWithContext(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get physical CPU count: %w", err)
	}

	return &CPUInfo{
		ModelName:    info[0].ModelName,
		Family:       info[0].Family,
		Model:        info[0].Model,
		Stepping:     info[0].Stepping,
		Cores:        int32(physicalCount),
		LogicalCores: int32(logicalCount),
		MHz:          info[0].Mhz,
		CacheSize:    info[0].CacheSize,
		VendorID:     info[0].VendorID,
	}, nil
}

// CPUInfo holds static CPU information
// This data is collected once during initialization
type CPUInfo struct {
	ModelName    string
	Family       string
	VendorID     string
	Model        string
	Stepping     int32
	Cores        int32
	LogicalCores int32
	CacheSize    int32
	MHz          float64
}
