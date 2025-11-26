package metrics

import (
	"context"
	"sync"
	"time"

	"github.com/castleops/client/internal/cache"
)

// Collector defines the interface for individual metric collectors
// Each collector is responsible for gathering specific metric types
// Implementations must be safe for concurrent access
type Collector interface {
	// Collect gathers metrics and returns them or an error
	// The context can be used for cancellation during long operations
	// Implementations should return partial data if possible on context cancellation
	Collect(ctx context.Context) error

	// Name returns a human-readable name for this collector
	// Used for logging and debugging purposes
	Name() string
}

// Config holds configuration for the metrics collection system
// Field ordering optimized for memory layout
type Config struct {
	// CollectionInterval specifies how often to collect metrics
	CollectionInterval time.Duration

	// MovingAverageWindow specifies how many samples to use for moving averages
	// Set to 0 to disable moving averages (use raw values)
	MovingAverageWindow int

	// EnableCPU controls whether CPU metrics are collected
	EnableCPU bool

	// EnableMemory controls whether memory metrics are collected
	EnableMemory bool

	// EnableDisk controls whether disk metrics are collected
	EnableDisk bool

	// EnableNetwork controls whether network metrics are collected
	EnableNetwork bool
}

// DefaultConfig returns a Config with sensible defaults for production use
func DefaultConfig() *Config {
	return &Config{
		CollectionInterval:  60 * time.Second,
		MovingAverageWindow: 5, // 5-sample moving average reduces noise
		EnableCPU:           true,
		EnableMemory:        true,
		EnableDisk:          true,
		EnableNetwork:       true,
	}
}

// collectorPool reduces allocations by reusing collector result structures
var collectorPool = sync.Pool{
	New: func() interface{} {
		return &cache.Metrics{}
	},
}

// getMetrics retrieves a Metrics struct from the pool
func getMetrics() *cache.Metrics {
	m := collectorPool.Get().(*cache.Metrics)
	// Reset all fields to zero values
	*m = cache.Metrics{}
	return m
}

// putMetrics returns a Metrics struct to the pool
func putMetrics(m *cache.Metrics) {
	if m != nil {
		collectorPool.Put(m)
	}
}

// movingAverage maintains a circular buffer for calculating moving averages
// This reduces noise in metrics without requiring historical database queries
type movingAverage struct {
	samples []float64
	index   int
	count   int
	size    int
	sum     float64
	mu      sync.RWMutex
}

// newMovingAverage creates a new moving average calculator with the specified window size
// A window size of 0 or 1 effectively disables smoothing
func newMovingAverage(size int) *movingAverage {
	if size <= 0 {
		size = 1
	}
	return &movingAverage{
		samples: make([]float64, size),
		size:    size,
	}
}

// Add adds a new sample and returns the current moving average
// This operation is O(1) and uses a circular buffer to avoid allocations
func (ma *movingAverage) Add(value float64) float64 {
	ma.mu.Lock()
	defer ma.mu.Unlock()

	// Remove old value from sum if buffer is full
	if ma.count == ma.size {
		ma.sum -= ma.samples[ma.index]
	}

	// Add new value
	ma.samples[ma.index] = value
	ma.sum += value

	// Update circular buffer index
	ma.index = (ma.index + 1) % ma.size

	// Track how many samples we have (up to size)
	if ma.count < ma.size {
		ma.count++
	}

	// Return average
	return ma.sum / float64(ma.count)
}

// Get returns the current moving average without adding a new sample
func (ma *movingAverage) Get() float64 {
	ma.mu.RLock()
	defer ma.mu.RUnlock()

	if ma.count == 0 {
		return 0
	}
	return ma.sum / float64(ma.count)
}

// Reset clears all samples and resets the moving average to zero
func (ma *movingAverage) Reset() {
	ma.mu.Lock()
	defer ma.mu.Unlock()

	ma.sum = 0
	ma.count = 0
	ma.index = 0
	for i := range ma.samples {
		ma.samples[i] = 0
	}
}

// metricsBuffer holds the current metrics being collected
// This is shared across all collectors to build a complete snapshot
type metricsBuffer struct {
	metrics *cache.Metrics
	mu      sync.RWMutex
}

// newMetricsBuffer creates a new buffer for collecting metrics
func newMetricsBuffer() *metricsBuffer {
	return &metricsBuffer{
		metrics: getMetrics(),
	}
}

// get returns a copy of the current metrics
// The caller should not modify the returned pointer
func (mb *metricsBuffer) get() *cache.Metrics {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	return mb.metrics
}

// updateCPU updates CPU metrics in the buffer
func (mb *metricsBuffer) updateCPU(usage float64) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	mb.metrics.CPUUsagePercent = usage
}

// updateMemory updates memory metrics in the buffer
func (mb *metricsBuffer) updateMemory(total, used, available uint64, percent float64) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	mb.metrics.MemoryTotal = total
	mb.metrics.MemoryUsed = used
	mb.metrics.MemoryAvailable = available
	mb.metrics.MemoryUsagePercent = percent
}

// updateDisk updates disk metrics in the buffer
func (mb *metricsBuffer) updateDisk(total, used, free uint64, percent float64) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	mb.metrics.DiskTotalBytes = total
	mb.metrics.DiskUsedBytes = used
	mb.metrics.DiskFreeBytes = free
	mb.metrics.DiskUsagePercent = percent
}

// updateNetwork updates network metrics in the buffer
func (mb *metricsBuffer) updateNetwork(bytesReceived, bytesSent uint64) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	mb.metrics.NetworkBytesReceived = bytesReceived
	mb.metrics.NetworkBytesSent = bytesSent
}

// finalize prepares the metrics for storage/transmission
// Sets the timestamp and client ID
func (mb *metricsBuffer) finalize(clientID string) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	mb.metrics.Timestamp = time.Now().UTC()
	mb.metrics.ClientID = clientID
}

// reset returns the current metrics to the pool and allocates a new one
func (mb *metricsBuffer) reset() {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	old := mb.metrics
	mb.metrics = getMetrics()
	putMetrics(old)
}
