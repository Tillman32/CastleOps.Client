package cache

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// cleanupInterval defines how often retention cleanup should run
const cleanupInterval = time.Hour

// MemoryCache is an in-memory implementation of the Cache interface
// Useful for testing and simple deployments
type MemoryCache struct {
	mu            sync.RWMutex
	commands      map[string]*Command
	metrics       []*Metrics
	retentionDays int
	logger        zerolog.Logger
	lastCleanup   time.Time
}

// NewMemoryCache creates a new in-memory cache
func NewMemoryCache(config MemoryConfig) *MemoryCache {
	if config.RetentionDays <= 0 {
		config.RetentionDays = 7
	}
	return &MemoryCache{
		commands:      make(map[string]*Command),
		metrics:       make([]*Metrics, 0),
		retentionDays: config.RetentionDays,
		logger:        config.Logger,
		lastCleanup:   time.Now(),
	}
}

// StoreCommand saves a command to the in-memory cache
func (m *MemoryCache) StoreCommand(ctx context.Context, cmd *Command) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.commands[cmd.CommandID] = cmd
	return nil
}

// GetPendingCommands retrieves all commands with pending status
func (m *MemoryCache) GetPendingCommands(ctx context.Context) ([]*Command, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var pending []*Command
	for _, cmd := range m.commands {
		if cmd.Status == StatusPending {
			pending = append(pending, cmd)
		}
	}
	return pending, nil
}

// MarkCommandComplete marks a command as completed
func (m *MemoryCache) MarkCommandComplete(ctx context.Context, commandID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cmd, exists := m.commands[commandID]; exists {
		cmd.Status = StatusComplete
		now := time.Now().UTC()
		cmd.CompletedAt = &now
	}
	return nil
}

// StoreMetrics saves metrics to the in-memory cache
func (m *MemoryCache) StoreMetrics(ctx context.Context, metrics *Metrics) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.metrics = append(m.metrics, metrics)

	// Only run cleanup periodically to avoid performance bottleneck
	if time.Since(m.lastCleanup) > cleanupInterval {
		m.cleanupOldMetrics()
		m.lastCleanup = time.Now()
	}

	return nil
}

// cleanupOldMetrics removes metrics beyond the retention period
// Must be called with mu lock held
func (m *MemoryCache) cleanupOldMetrics() {
	cutoff := time.Now().UTC().AddDate(0, 0, -m.retentionDays)
	var retained []*Metrics
	for _, metric := range m.metrics {
		if !metric.Timestamp.Before(cutoff) {
			retained = append(retained, metric)
		}
	}
	m.metrics = retained
}

// GetMetrics retrieves metrics within the specified time range
func (m *MemoryCache) GetMetrics(ctx context.Context, start, end time.Time) ([]*Metrics, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*Metrics
	for _, metric := range m.metrics {
		if (metric.Timestamp.Equal(start) || metric.Timestamp.After(start)) &&
			(metric.Timestamp.Equal(end) || metric.Timestamp.Before(end)) {
			result = append(result, metric)
		}
	}
	return result, nil
}

// Close releases resources (no-op for memory cache)
func (m *MemoryCache) Close() error {
	return nil
}

// Cleanup performs manual cleanup of old metrics beyond the retention period.
// This can be called on-demand instead of waiting for the periodic cleanup.
func (m *MemoryCache) Cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupOldMetrics()
	m.lastCleanup = time.Now()
}
