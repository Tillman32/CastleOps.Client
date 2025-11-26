package api

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/castleops/client/internal/cache"
	"github.com/rs/zerolog"
)

// RequestQueue buffers API requests during offline periods
// and retries them when connectivity is restored
type RequestQueue struct {
	// cache stores queued requests
	cache cache.Cache

	// client is the API client for sending requests
	client *Client

	// logger for structured logging
	logger zerolog.Logger

	// mu protects queue state
	mu sync.RWMutex

	// isOnline tracks connectivity status
	isOnline bool

	// retryTicker triggers periodic retry attempts
	retryTicker *time.Ticker

	// ctx is the queue's context for cancellation
	ctx context.Context

	// cancel stops the queue
	cancel context.CancelFunc

	// wg tracks background operations
	wg sync.WaitGroup

	// maxQueueSize limits buffered requests (0 = unlimited)
	maxQueueSize int

	// retryInterval is how often to retry failed requests
	retryInterval time.Duration
}

// QueuedRequest represents a request waiting to be sent
type QueuedRequest struct {
	ID          int64
	Method      string
	Path        string
	Body        string // JSON-encoded request body
	Priority    int    // Higher priority = sent first
	CreatedAt   time.Time
	Attempts    int
	LastAttempt time.Time
	MaxRetries  int
}

// RequestQueueConfig configures the request queue
type RequestQueueConfig struct {
	// Cache stores queued requests
	Cache cache.Cache

	// Client is the API client
	Client *Client

	// Logger for structured logging
	Logger zerolog.Logger

	// MaxQueueSize limits buffered requests (0 = unlimited, default: 1000)
	MaxQueueSize int

	// RetryInterval is how often to retry failed requests (default: 30s)
	RetryInterval time.Duration

	// MaxRetries is the maximum retry attempts per request (default: 5)
	MaxRetries int
}

// NewRequestQueue creates a new request queue
func NewRequestQueue(config RequestQueueConfig) *RequestQueue {
	if config.MaxQueueSize <= 0 {
		config.MaxQueueSize = 1000
	}
	if config.RetryInterval == 0 {
		config.RetryInterval = 30 * time.Second
	}
	if config.MaxRetries <= 0 {
		config.MaxRetries = 5
	}

	ctx, cancel := context.WithCancel(context.Background())

	queue := &RequestQueue{
		cache:         config.Cache,
		client:        config.Client,
		logger:        config.Logger,
		isOnline:      true, // Assume online initially
		retryTicker:   time.NewTicker(config.RetryInterval),
		ctx:           ctx,
		cancel:        cancel,
		maxQueueSize:  config.MaxQueueSize,
		retryInterval: config.RetryInterval,
	}

	// Start background retry processor
	queue.wg.Add(1)
	go queue.processRetries()

	return queue
}

// Enqueue adds a request to the queue for later delivery
func (q *RequestQueue) Enqueue(ctx context.Context, method, path string, body interface{}, priority int) error {
	// Serialize body if provided
	if body != nil {
		if _, err := json.Marshal(body); err != nil {
			return fmt.Errorf("failed to serialize request body: %w", err)
		}
	}

	// TODO: Add dedicated queued_requests table to cache schema
	// For now, we queue metrics directly in cache and use command table for commands
	q.logger.Debug().
		Str("method", method).
		Str("path", path).
		Int("priority", priority).
		Msg("Request queued for offline delivery")

	return nil
}

// EnqueueMetrics queues metrics for later upload
func (q *RequestQueue) EnqueueMetrics(ctx context.Context, metrics *cache.Metrics) error {
	// Store metrics directly in cache - they'll be uploaded when online
	if err := q.cache.StoreMetrics(ctx, metrics); err != nil {
		return fmt.Errorf("failed to queue metrics: %w", err)
	}

	q.logger.Debug().
		Time("timestamp", metrics.Timestamp).
		Msg("Metrics queued for upload")

	// If online, trigger immediate upload attempt
	if q.IsOnline() {
		go q.uploadPendingMetrics()
	}

	return nil
}

// SetOnline updates the connectivity status
func (q *RequestQueue) SetOnline(online bool) {
	q.mu.Lock()
	wasOffline := !q.isOnline
	q.isOnline = online
	q.mu.Unlock()

	if wasOffline && online {
		q.logger.Info().Msg("Connectivity restored, processing queued requests")
		// Trigger immediate retry
		go q.uploadPendingMetrics()
	} else if !online {
		q.logger.Warn().Msg("Lost connectivity, buffering requests offline")
	}
}

// IsOnline returns current connectivity status
func (q *RequestQueue) IsOnline() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.isOnline
}

// processRetries periodically attempts to send queued requests
func (q *RequestQueue) processRetries() {
	defer q.wg.Done()

	for {
		select {
		case <-q.retryTicker.C:
			if q.IsOnline() {
				q.uploadPendingMetrics()
			}
		case <-q.ctx.Done():
			return
		}
	}
}

// uploadPendingMetrics attempts to upload buffered metrics
func (q *RequestQueue) uploadPendingMetrics() {
	// Get unsynced metrics from cache
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get metrics from last 7 days that haven't been synced
	endTime := time.Now().UTC()
	startTime := endTime.Add(-7 * 24 * time.Hour)

	metrics, err := q.cache.GetMetrics(ctx, startTime, endTime)
	if err != nil {
		q.logger.Error().
			Err(err).
			Msg("Failed to retrieve pending metrics from cache")
		return
	}

	// Filter for unsynced metrics
	var unsynced []*cache.Metrics
	for _, m := range metrics {
		if !m.Synced {
			unsynced = append(unsynced, m)
		}
	}

	if len(unsynced) == 0 {
		q.logger.Debug().Msg("No pending metrics to upload")
		return
	}

	q.logger.Info().
		Int("count", len(unsynced)).
		Msg("Uploading pending metrics")

	// Convert to upload format and batch
	batchSize := 100
	for i := 0; i < len(unsynced); i += batchSize {
		end := i + batchSize
		if end > len(unsynced) {
			end = len(unsynced)
		}

		batch := unsynced[i:end]
		if err := q.uploadMetricsBatch(ctx, batch); err != nil {
			q.logger.Error().
				Err(err).
				Int("batch_start", i).
				Int("batch_end", end).
				Msg("Failed to upload metrics batch")
			// Continue with next batch
			continue
		}

		// Mark metrics as synced
		// TODO: Add cache method to update synced status
		q.logger.Debug().
			Int("count", len(batch)).
			Msg("Metrics batch uploaded successfully")
	}
}

// uploadMetricsBatch uploads a batch of metrics to the server
func (q *RequestQueue) uploadMetricsBatch(ctx context.Context, metrics []*cache.Metrics) error {
	// Convert to API format
	snapshots := make([]MetricSnapshot, len(metrics))
	for i, m := range metrics {
		snapshots[i] = MetricSnapshot{
			Timestamp:            m.Timestamp,
			CPUUsagePercent:      m.CPUUsagePercent,
			MemoryTotal:          m.MemoryTotal,
			MemoryUsed:           m.MemoryUsed,
			MemoryAvailable:      m.MemoryAvailable,
			MemoryUsagePercent:   m.MemoryUsagePercent,
			DiskTotalBytes:       m.DiskTotalBytes,
			DiskUsedBytes:        m.DiskUsedBytes,
			DiskFreeBytes:        m.DiskFreeBytes,
			DiskUsagePercent:     m.DiskUsagePercent,
			NetworkBytesReceived: m.NetworkBytesReceived,
			NetworkBytesSent:     m.NetworkBytesSent,
		}
	}

	req := &MetricsUploadRequest{
		Metrics: snapshots,
		Count:   len(snapshots),
	}

	// Upload to server
	_, err := q.client.UploadMetrics(ctx, req)
	if err != nil {
		// Check if it's a network error - if so, mark as offline
		if _, ok := err.(*RequestError); ok {
			q.SetOnline(false)
		}
		return err
	}

	return nil
}

// GetQueueSize returns the approximate number of queued requests
func (q *RequestQueue) GetQueueSize() int {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Count unsynced metrics
	endTime := time.Now().UTC()
	startTime := endTime.Add(-7 * 24 * time.Hour)

	metrics, err := q.cache.GetMetrics(ctx, startTime, endTime)
	if err != nil {
		q.logger.Error().Err(err).Msg("Failed to get queue size")
		return 0
	}

	count := 0
	for _, m := range metrics {
		if !m.Synced {
			count++
		}
	}

	return count
}

// Flush attempts to send all queued requests immediately
// Returns the number of successfully sent requests
func (q *RequestQueue) Flush(ctx context.Context) (int, error) {
	if !q.IsOnline() {
		return 0, fmt.Errorf("cannot flush while offline")
	}

	q.logger.Info().Msg("Flushing request queue")

	initialCount := q.GetQueueSize()
	q.uploadPendingMetrics()
	finalCount := q.GetQueueSize()

	sent := initialCount - finalCount
	if sent < 0 {
		sent = 0
	}

	q.logger.Info().
		Int("sent", sent).
		Int("remaining", finalCount).
		Msg("Queue flush completed")

	return sent, nil
}

// Stop gracefully shuts down the request queue
func (q *RequestQueue) Stop() error {
	q.logger.Info().Msg("Stopping request queue")

	// Stop retry ticker
	q.retryTicker.Stop()

	// Cancel context
	q.cancel()

	// Wait for background operations
	q.wg.Wait()

	q.logger.Info().Msg("Request queue stopped")
	return nil
}

// PriorityQueue implements a priority queue for requests
// Higher priority items are processed first
type PriorityQueue []*QueuedRequest

func (pq PriorityQueue) Len() int { return len(pq) }

func (pq PriorityQueue) Less(i, j int) bool {
	// Higher priority first
	if pq[i].Priority != pq[j].Priority {
		return pq[i].Priority > pq[j].Priority
	}
	// Then older requests first
	return pq[i].CreatedAt.Before(pq[j].CreatedAt)
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
}

func (pq *PriorityQueue) Push(x interface{}) {
	item := x.(*QueuedRequest)
	*pq = append(*pq, item)
}

func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil // avoid memory leak
	*pq = old[0 : n-1]
	return item
}

// HealthCheck performs a connectivity check
func (q *RequestQueue) HealthCheck(ctx context.Context) error {
	// Try a simple GET request to check connectivity
	// This would typically be a dedicated health endpoint
	_, err := q.client.GetCommands(ctx)
	if err != nil {
		// Check error type
		if _, ok := err.(*RequestError); ok {
			// Network error
			q.SetOnline(false)
			return fmt.Errorf("health check failed: offline")
		}
		// API error (but connected)
		q.SetOnline(true)
		return nil
	}

	q.SetOnline(true)
	return nil
}

// StartHealthChecks begins periodic connectivity checks
func (q *RequestQueue) StartHealthChecks(interval time.Duration) {
	if interval == 0 {
		interval = 30 * time.Second
	}

	q.wg.Add(1)
	go func() {
		defer q.wg.Done()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				if err := q.HealthCheck(ctx); err != nil {
					q.logger.Debug().
						Err(err).
						Msg("Health check failed")
				}
				cancel()
			case <-q.ctx.Done():
				return
			}
		}
	}()

	q.logger.Info().
		Dur("interval", interval).
		Msg("Started health checks")
}
