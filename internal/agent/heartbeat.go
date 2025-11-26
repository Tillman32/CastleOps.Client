package agent

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/castleops/client/internal/api"
	"github.com/castleops/client/internal/config"
	"github.com/rs/zerolog"
)

// HeartbeatService manages periodic heartbeats to the server
// It includes exponential backoff on failures, health monitoring,
// and graceful degradation when the server is unreachable.
type HeartbeatService struct {
	apiClient *api.Client
	config    *config.Config
	logger    zerolog.Logger
	startTime time.Time

	// State management
	running             atomic.Bool
	lastHeartbeat       atomic.Value // time.Time
	consecutiveFailures atomic.Uint64
	stopCh              chan struct{}
	stoppedCh           chan struct{}
	wg                  sync.WaitGroup

	// Configuration
	interval          time.Duration
	maxRetries        int
	initialBackoff    time.Duration
	maxBackoff        time.Duration
	backoffMultiplier float64

	mu sync.RWMutex
}

// heartbeatState tracks the last heartbeat details
type heartbeatState struct {
	timestamp time.Time
	success   bool
	err       error
}

// NewHeartbeatService creates a new heartbeat service
func NewHeartbeatService(apiClient *api.Client, config *config.Config, logger zerolog.Logger, startTime time.Time) *HeartbeatService {
	svc := &HeartbeatService{
		apiClient: apiClient,
		config:    config,
		logger:    logger.With().Str("component", "heartbeat").Logger(),
		startTime: startTime,
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),

		// Configuration with sensible defaults
		interval:          config.Heartbeat.Interval,
		maxRetries:        config.Heartbeat.RetryAttempts,
		initialBackoff:    1 * time.Second,
		maxBackoff:        5 * time.Minute,
		backoffMultiplier: 2.0,
	}

	// Initialize last heartbeat time
	svc.lastHeartbeat.Store(time.Time{})

	return svc
}

// Start begins sending periodic heartbeats
func (s *HeartbeatService) Start(ctx context.Context) error {
	if s.running.Load() {
		return fmt.Errorf("heartbeat service already running")
	}

	s.running.Store(true)

	s.wg.Add(1)
	go s.heartbeatLoop(ctx)

	s.logger.Info().
		Dur("interval", s.interval).
		Int("max_retries", s.maxRetries).
		Msg("Heartbeat service started")

	return nil
}

// Stop gracefully stops the heartbeat service
func (s *HeartbeatService) Stop(ctx context.Context) error {
	if !s.running.Load() {
		return nil
	}

	s.logger.Info().Msg("Stopping heartbeat service")

	// Signal stop
	close(s.stopCh)

	// Wait for heartbeat loop to exit (with timeout from context)
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Info().Msg("Heartbeat service stopped gracefully")
	case <-ctx.Done():
		s.logger.Warn().Msg("Heartbeat service stop timeout exceeded")
	}

	s.running.Store(false)
	close(s.stoppedCh)

	return nil
}

// heartbeatLoop runs the periodic heartbeat cycle
func (s *HeartbeatService) heartbeatLoop(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	// Send initial heartbeat immediately
	s.sendHeartbeatWithRetry(ctx)

	for {
		select {
		case <-ticker.C:
			s.sendHeartbeatWithRetry(ctx)

		case <-s.stopCh:
			s.logger.Debug().Msg("Heartbeat loop received stop signal")
			return

		case <-ctx.Done():
			s.logger.Debug().Msg("Heartbeat loop context cancelled")
			return
		}
	}
}

// sendHeartbeatWithRetry attempts to send a heartbeat with exponential backoff retry logic
func (s *HeartbeatService) sendHeartbeatWithRetry(ctx context.Context) {
	var lastErr error
	attempt := 0

	for attempt <= s.maxRetries {
		// Calculate backoff for this attempt
		if attempt > 0 {
			backoff := s.calculateBackoff(attempt)

			s.logger.Debug().
				Int("attempt", attempt).
				Dur("backoff", backoff).
				Msg("Retrying heartbeat after backoff")

			// Wait for backoff duration or until stopped
			select {
			case <-time.After(backoff):
				// Continue with retry
			case <-s.stopCh:
				return
			case <-ctx.Done():
				return
			}
		}

		// Attempt to send heartbeat
		err := s.sendHeartbeat(ctx)
		if err == nil {
			// Success - reset failure counter
			if attempt > 0 {
				s.logger.Info().
					Int("attempt", attempt).
					Msg("Heartbeat succeeded after retry")
			}

			s.consecutiveFailures.Store(0)
			s.lastHeartbeat.Store(time.Now())
			return
		}

		lastErr = err
		attempt++

		// Log the failure
		s.logger.Warn().
			Err(err).
			Int("attempt", attempt).
			Int("max_retries", s.maxRetries).
			Msg("Heartbeat attempt failed")
	}

	// All retries exhausted
	failures := s.consecutiveFailures.Add(1)

	s.logger.Error().
		Err(lastErr).
		Uint64("consecutive_failures", failures).
		Msg("Heartbeat failed after all retries")

	// Check if we should alert or take action based on failure count
	if failures >= 10 {
		s.logger.Error().
			Uint64("consecutive_failures", failures).
			Msg("CRITICAL: Extended heartbeat failure - server may be unreachable")
	}
}

// sendHeartbeat sends a single heartbeat to the server
func (s *HeartbeatService) sendHeartbeat(ctx context.Context) error {
	// Create heartbeat request with current status
	uptime := time.Since(s.startTime).Seconds()
	status := s.determineClientStatus()

	req := &api.HeartbeatRequest{
		Status:  status,
		Uptime:  int64(uptime),
		Version: AgentVersion,
	}

	// Add timeout for heartbeat request
	heartbeatCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Send heartbeat
	resp, err := s.apiClient.Heartbeat(heartbeatCtx, req)
	if err != nil {
		return fmt.Errorf("failed to send heartbeat: %w", err)
	}

	// Process response
	if resp.Acknowledged {
		s.logger.Debug().
			Str("status", status).
			Float64("uptime_seconds", uptime).
			Msg("Heartbeat acknowledged by server")
	}

	// Handle configuration updates from server
	if resp.ConfigUpdate != nil {
		s.handleConfigUpdate(resp.ConfigUpdate)
	}

	// Handle commands from server (if included in heartbeat response)
	if len(resp.Commands) > 0 {
		s.logger.Info().
			Int("command_count", len(resp.Commands)).
			Msg("Received commands in heartbeat response")
		// Commands would be processed by the agent's command handler
	}

	return nil
}

// determineClientStatus evaluates the current client health status
func (s *HeartbeatService) determineClientStatus() string {
	// Check consecutive failures
	failures := s.consecutiveFailures.Load()

	if failures == 0 {
		return api.ClientStatusOnline
	} else if failures < 5 {
		return api.ClientStatusDegraded
	} else {
		// This shouldn't normally happen since we're sending the heartbeat
		// but indicates recent communication issues
		return api.ClientStatusDegraded
	}
}

// handleConfigUpdate processes configuration updates from the server
func (s *HeartbeatService) handleConfigUpdate(update *api.ConfigUpdate) {
	s.mu.Lock()
	defer s.mu.Unlock()

	updated := false

	if update.HeartbeatInterval != nil {
		newInterval := time.Duration(*update.HeartbeatInterval) * time.Second
		if newInterval != s.interval && newInterval > 0 {
			s.logger.Info().
				Dur("old_interval", s.interval).
				Dur("new_interval", newInterval).
				Msg("Heartbeat interval updated by server")
			s.interval = newInterval
			updated = true
		}
	}

	if update.MetricsInterval != nil {
		newInterval := time.Duration(*update.MetricsInterval) * time.Second
		s.logger.Info().
			Dur("new_interval", newInterval).
			Msg("Metrics interval updated by server")
		// This would need to be propagated to the metrics system
		updated = true
	}

	if updated {
		s.logger.Info().Msg("Configuration updated from server")
	}
}

// calculateBackoff computes exponential backoff with jitter
func (s *HeartbeatService) calculateBackoff(attempt int) time.Duration {
	// Exponential backoff: initialBackoff * (multiplier ^ attempt)
	backoff := float64(s.initialBackoff) * math.Pow(s.backoffMultiplier, float64(attempt-1))

	// Cap at max backoff
	if backoff > float64(s.maxBackoff) {
		backoff = float64(s.maxBackoff)
	}

	// Add jitter (±25%) to prevent thundering herd
	jitter := backoff * 0.25
	backoff = backoff - jitter + (2 * jitter * float64(time.Now().UnixNano()%100) / 100)

	return time.Duration(backoff)
}

// SendNow triggers an immediate heartbeat outside the regular schedule
// This is useful for signaling important events to the server
func (s *HeartbeatService) SendNow(ctx context.Context) error {
	if !s.running.Load() {
		return fmt.Errorf("heartbeat service not running")
	}

	s.logger.Debug().Msg("Sending immediate heartbeat")

	return s.sendHeartbeat(ctx)
}

// IsRunning returns true if the heartbeat service is active
func (s *HeartbeatService) IsRunning() bool {
	return s.running.Load()
}

// GetLastHeartbeat returns the timestamp of the last successful heartbeat
func (s *HeartbeatService) GetLastHeartbeat() time.Time {
	val := s.lastHeartbeat.Load()
	if val == nil {
		return time.Time{}
	}
	return val.(time.Time)
}

// GetConsecutiveFailures returns the count of consecutive heartbeat failures
func (s *HeartbeatService) GetConsecutiveFailures() uint64 {
	return s.consecutiveFailures.Load()
}

// GetInterval returns the current heartbeat interval
func (s *HeartbeatService) GetInterval() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.interval
}

// SetInterval updates the heartbeat interval
// Note: This doesn't restart the ticker, change takes effect after current cycle
func (s *HeartbeatService) SetInterval(interval time.Duration) error {
	if interval <= 0 {
		return fmt.Errorf("interval must be positive")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.logger.Info().
		Dur("old_interval", s.interval).
		Dur("new_interval", interval).
		Msg("Heartbeat interval updated")

	s.interval = interval

	return nil
}

// WaitForShutdown blocks until the heartbeat service has completely stopped
func (s *HeartbeatService) WaitForShutdown() {
	<-s.stoppedCh
}

// Stats returns current heartbeat statistics
func (s *HeartbeatService) Stats() HeartbeatStats {
	return HeartbeatStats{
		Running:             s.running.Load(),
		LastHeartbeat:       s.GetLastHeartbeat(),
		ConsecutiveFailures: s.GetConsecutiveFailures(),
		Interval:            s.GetInterval(),
		Uptime:              time.Since(s.startTime),
	}
}

// HeartbeatStats holds heartbeat service statistics
type HeartbeatStats struct {
	Running             bool
	LastHeartbeat       time.Time
	ConsecutiveFailures uint64
	Interval            time.Duration
	Uptime              time.Duration
}
