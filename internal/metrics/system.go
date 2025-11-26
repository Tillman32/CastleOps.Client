package metrics

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/castleops/client/internal/cache"
	"github.com/rs/zerolog"
)

// System orchestrates all metrics collectors
// Provides high-level API for starting/stopping metrics collection
// Handles concurrent collection and graceful shutdown
type System struct {
	config     *Config
	buffer     *metricsBuffer
	collectors []Collector
	logger     zerolog.Logger
	clientID   string

	// State management
	running   atomic.Bool
	stopCh    chan struct{}
	stoppedCh chan struct{}
	wg        sync.WaitGroup

	// Collectors (initialized based on config)
	cpuCollector     *cpuCollector
	memoryCollector  *memoryCollector
	diskCollector    *diskCollector
	networkCollector *networkCollector

	// Callback for handling collected metrics
	// This is called after each successful collection
	onMetricsCollected func(ctx context.Context, metrics *cache.Metrics) error

	mu sync.RWMutex
}

// NewSystem creates a new metrics collection system
// The logger is used for operational logging (errors, warnings)
// The clientID is embedded in all collected metrics
func NewSystem(config *Config, logger zerolog.Logger, clientID string) (*System, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Validate config
	if config.CollectionInterval <= 0 {
		return nil, fmt.Errorf("invalid collection interval: %v", config.CollectionInterval)
	}

	s := &System{
		config:    config,
		buffer:    newMetricsBuffer(),
		logger:    logger.With().Str("component", "metrics").Logger(),
		clientID:  clientID,
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
	}

	// Initialize enabled collectors
	if err := s.initCollectors(); err != nil {
		return nil, fmt.Errorf("failed to initialize collectors: %w", err)
	}

	return s, nil
}

// initCollectors creates collector instances based on configuration
func (s *System) initCollectors() error {
	s.collectors = make([]Collector, 0, 4)

	if s.config.EnableCPU {
		s.cpuCollector = newCPUCollector(s.buffer, s.config.MovingAverageWindow)
		s.collectors = append(s.collectors, s.cpuCollector)
		s.logger.Debug().Msg("CPU collector enabled")
	}

	if s.config.EnableMemory {
		s.memoryCollector = newMemoryCollector(s.buffer, s.config.MovingAverageWindow)
		s.collectors = append(s.collectors, s.memoryCollector)
		s.logger.Debug().Msg("Memory collector enabled")
	}

	if s.config.EnableDisk {
		s.diskCollector = newDiskCollector(s.buffer, s.config.MovingAverageWindow, "")
		s.collectors = append(s.collectors, s.diskCollector)
		s.logger.Debug().Msg("Disk collector enabled")
	}

	if s.config.EnableNetwork {
		s.networkCollector = newNetworkCollector(s.buffer)
		s.collectors = append(s.collectors, s.networkCollector)
		s.logger.Debug().Msg("Network collector enabled")
	}

	if len(s.collectors) == 0 {
		return fmt.Errorf("no collectors enabled")
	}

	return nil
}

// SetMetricsCallback sets a function to be called after each successful collection
// This is typically used to store metrics in a cache or send them to a server
// The callback is called synchronously and should not block for long periods
func (s *System) SetMetricsCallback(callback func(ctx context.Context, metrics *cache.Metrics) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onMetricsCollected = callback
}

// Start begins periodic metrics collection
// Metrics are collected at the interval specified in Config
// Collection runs in a background goroutine and can be stopped with Stop()
func (s *System) Start(ctx context.Context) error {
	if s.running.Load() {
		return fmt.Errorf("system already running")
	}

	s.running.Store(true)

	// Start collection goroutine
	s.wg.Add(1)
	go s.collectionLoop(ctx)

	s.logger.Info().
		Dur("interval", s.config.CollectionInterval).
		Int("collectors", len(s.collectors)).
		Msg("Metrics collection started")

	return nil
}

// Stop gracefully shuts down the metrics collection system
// Waits for in-flight collections to complete
// Returns when all background goroutines have exited
func (s *System) Stop() error {
	if !s.running.Load() {
		return nil
	}

	s.logger.Info().Msg("Stopping metrics collection")

	// Signal stop
	close(s.stopCh)

	// Wait for collection loop to exit
	s.wg.Wait()

	s.running.Store(false)

	s.logger.Info().Msg("Metrics collection stopped")

	return nil
}

// collectionLoop runs the periodic metrics collection
// This is the main goroutine that orchestrates collection
func (s *System) collectionLoop(ctx context.Context) {
	defer s.wg.Done()
	defer close(s.stoppedCh)

	ticker := time.NewTicker(s.config.CollectionInterval)
	defer ticker.Stop()

	// Collect immediately on start (don't wait for first tick)
	s.collectOnce(ctx)

	for {
		select {
		case <-ticker.C:
			s.collectOnce(ctx)

		case <-s.stopCh:
			s.logger.Debug().Msg("Collection loop received stop signal")
			return

		case <-ctx.Done():
			s.logger.Debug().Msg("Collection loop context cancelled")
			return
		}
	}
}

// collectOnce performs a single collection cycle
// Runs all enabled collectors concurrently and aggregates results
func (s *System) collectOnce(ctx context.Context) {
	start := time.Now()

	// Create a context with timeout for this collection cycle
	// Ensures collectors don't block indefinitely
	collectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Run all collectors concurrently
	var wg sync.WaitGroup
	errorsCh := make(chan error, len(s.collectors))

	for _, collector := range s.collectors {
		wg.Add(1)
		go func(c Collector) {
			defer wg.Done()

			if err := c.Collect(collectCtx); err != nil {
				// Don't fail entire collection if one collector fails
				// Log the error and continue
				s.logger.Warn().
					Err(err).
					Str("collector", c.Name()).
					Msg("Collector failed")
				errorsCh <- err
			}
		}(collector)
	}

	// Wait for all collectors to finish
	wg.Wait()
	close(errorsCh)

	// Count errors
	errorCount := 0
	for range errorsCh {
		errorCount++
	}

	// Finalize metrics with timestamp and client ID
	s.buffer.finalize(s.clientID)

	// Get the collected metrics
	metrics := s.buffer.get()

	// Make a copy for the callback (buffer will be reused)
	metricsCopy := *metrics

	duration := time.Since(start)

	s.logger.Debug().
		Dur("duration", duration).
		Int("errors", errorCount).
		Float64("cpu_percent", metricsCopy.CPUUsagePercent).
		Float64("memory_percent", metricsCopy.MemoryUsagePercent).
		Float64("disk_percent", metricsCopy.DiskUsagePercent).
		Msg("Metrics collected")

	// Call metrics callback if set
	s.mu.RLock()
	callback := s.onMetricsCollected
	s.mu.RUnlock()

	if callback != nil {
		// Call callback with a fresh context (not the collection context)
		callbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := callback(callbackCtx, &metricsCopy); err != nil {
			s.logger.Error().
				Err(err).
				Msg("Metrics callback failed")
		}
	}

	// Reset buffer for next collection
	// This returns the old buffer to the pool
	s.buffer.reset()
}

// CollectNow triggers an immediate metrics collection
// This is useful for on-demand metrics gathering
// Returns the collected metrics or an error
func (s *System) CollectNow(ctx context.Context) (*cache.Metrics, error) {
	// Create a context with timeout
	collectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Create a temporary buffer for this collection
	tempBuffer := newMetricsBuffer()

	// Run all collectors concurrently against the temp buffer
	var wg sync.WaitGroup
	errorsCh := make(chan error, len(s.collectors))

	// Temporarily replace the buffer
	s.mu.Lock()
	originalBuffer := s.buffer
	s.buffer = tempBuffer

	// Update collector references
	if s.cpuCollector != nil {
		s.cpuCollector.buffer = tempBuffer
	}
	if s.memoryCollector != nil {
		s.memoryCollector.buffer = tempBuffer
	}
	if s.diskCollector != nil {
		s.diskCollector.buffer = tempBuffer
	}
	if s.networkCollector != nil {
		s.networkCollector.buffer = tempBuffer
	}
	s.mu.Unlock()

	// Collect from all collectors
	for _, collector := range s.collectors {
		wg.Add(1)
		go func(c Collector) {
			defer wg.Done()
			if err := c.Collect(collectCtx); err != nil {
				errorsCh <- err
			}
		}(collector)
	}

	// Wait for completion
	wg.Wait()
	close(errorsCh)

	// Restore original buffer
	s.mu.Lock()
	s.buffer = originalBuffer
	if s.cpuCollector != nil {
		s.cpuCollector.buffer = originalBuffer
	}
	if s.memoryCollector != nil {
		s.memoryCollector.buffer = originalBuffer
	}
	if s.diskCollector != nil {
		s.diskCollector.buffer = originalBuffer
	}
	if s.networkCollector != nil {
		s.networkCollector.buffer = originalBuffer
	}
	s.mu.Unlock()

	// Check for errors
	var errors []error
	for err := range errorsCh {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return nil, fmt.Errorf("collection failed with %d errors: %v", len(errors), errors)
	}

	// Finalize and return metrics
	tempBuffer.finalize(s.clientID)
	metrics := tempBuffer.get()
	metricsCopy := *metrics

	return &metricsCopy, nil
}

// GetConfig returns a copy of the current configuration
func (s *System) GetConfig() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return *s.config
}

// UpdateConfig updates the system configuration
// Note: Changes to enable/disable flags require a restart
// Collection interval changes take effect on the next cycle
func (s *System) UpdateConfig(config *Config) error {
	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}

	if config.CollectionInterval <= 0 {
		return fmt.Errorf("invalid collection interval: %v", config.CollectionInterval)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.config = config

	s.logger.Info().
		Dur("interval", config.CollectionInterval).
		Int("window", config.MovingAverageWindow).
		Msg("Configuration updated")

	return nil
}

// IsRunning returns true if the system is currently collecting metrics
func (s *System) IsRunning() bool {
	return s.running.Load()
}

// WaitForShutdown blocks until the system has completely shut down
// This is useful for graceful shutdown coordination
func (s *System) WaitForShutdown() {
	<-s.stoppedCh
}

// ResetCollectors clears all moving average history
// Useful when you want fresh measurements without historical smoothing
func (s *System) ResetCollectors() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cpuCollector != nil {
		s.cpuCollector.Reset()
	}
	if s.memoryCollector != nil {
		s.memoryCollector.Reset()
	}
	if s.diskCollector != nil {
		s.diskCollector.Reset()
	}
	if s.networkCollector != nil {
		s.networkCollector.Reset()
	}

	s.logger.Debug().Msg("All collectors reset")
}

// Stats returns statistics about the collection system
func (s *System) Stats() SystemStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return SystemStats{
		Running:            s.running.Load(),
		CollectorCount:     len(s.collectors),
		CollectionInterval: s.config.CollectionInterval,
		MovingAvgWindow:    s.config.MovingAverageWindow,
	}
}

// SystemStats holds statistics about the metrics collection system
type SystemStats struct {
	Running            bool
	CollectorCount     int
	CollectionInterval time.Duration
	MovingAvgWindow    int
}
