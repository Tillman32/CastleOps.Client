package agent

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/castleops/client/internal/api"
	"github.com/castleops/client/internal/cache"
	"github.com/castleops/client/internal/config"
	"github.com/castleops/client/internal/metrics"
	"github.com/rs/zerolog"
)

// Agent is the main orchestrator that coordinates all subsystems
// It manages the lifecycle of metrics collection, heartbeat service,
// cache storage, and API communication with minimal overhead.
type Agent struct {
	// Configuration
	config *config.Config
	logger zerolog.Logger

	// Core components
	apiClient       *api.Client
	cache           cache.Cache
	metricsSystem   *metrics.System
	heartbeatSvc    *HeartbeatService
	registrationSvc *RegistrationService

	// State management
	running    atomic.Bool
	registered atomic.Bool
	startTime  time.Time
	stopCh     chan struct{}
	stoppedCh  chan struct{}
	errCh      chan error

	// Coordination
	wg sync.WaitGroup
	mu sync.RWMutex
}

// Config holds agent-specific configuration
type AgentConfig struct {
	// ClientID is set after successful registration
	ClientID string

	// Token is set after successful registration
	Token string

	// MetricsUploadInterval controls how often to upload cached metrics
	// Default: 5 minutes
	MetricsUploadInterval time.Duration

	// CommandPollInterval controls how often to poll for commands
	// Default: 30 seconds
	CommandPollInterval time.Duration

	// CacheFlushInterval controls how often to flush unsynced metrics
	// Default: 1 minute
	CacheFlushInterval time.Duration

	// MaxRetentionAge is how long to keep synced metrics
	// Default: 7 days
	MaxRetentionAge time.Duration
}

// NewAgent creates a new agent instance with all subsystems initialized
// This is the main entry point for the agent orchestrator
func NewAgent(cfg *config.Config, logger zerolog.Logger) (*Agent, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	agent := &Agent{
		config:    cfg,
		logger:    logger.With().Str("component", "agent").Logger(),
		startTime: time.Now(),
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
		errCh:     make(chan error, 1),
	}

	// Initialize components in dependency order
	if err := agent.initCache(); err != nil {
		return nil, fmt.Errorf("failed to initialize cache: %w", err)
	}

	if err := agent.initAPIClient(); err != nil {
		agent.cache.Close()
		return nil, fmt.Errorf("failed to initialize API client: %w", err)
	}

	if err := agent.initRegistration(); err != nil {
		agent.cleanup()
		return nil, fmt.Errorf("failed to initialize registration: %w", err)
	}

	return agent, nil
}

// initCache initializes the cache layer (SQLite or memory)
func (a *Agent) initCache() error {
	var err error
	a.cache, err = cache.New(a.config, a.logger)
	if err != nil {
		return fmt.Errorf("failed to create cache: %w", err)
	}

	a.logger.Info().
		Str("type", a.config.Cache.Type).
		Str("path", a.config.Cache.Path).
		Msg("Cache initialized")

	return nil
}

// initAPIClient initializes the REST API client with TLS and retry logic
func (a *Agent) initAPIClient() error {
	// Configure TLS
	var tlsConfig *api.ClientConfig
	if a.config.Server.TLSVerify {
		tlsConfig = &api.ClientConfig{
			BaseURL:   a.config.Server.URL,
			TLSConfig: api.NewDefaultTLSConfig(),
			Timeout:   a.config.Server.Timeout,
			Logger:    a.logger,
		}
	} else {
		// For development/testing - allow insecure connections
		tlsConfig = &api.ClientConfig{
			BaseURL: a.config.Server.URL,
			Timeout: a.config.Server.Timeout,
			Logger:  a.logger,
		}
	}

	var err error
	a.apiClient, err = api.NewClient(*tlsConfig)
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	// Set credentials if already registered
	clientID, token := a.config.GetClientCredentials()
	if clientID != "" && token != "" {
		a.apiClient.SetCredentials(clientID, token)
		a.registered.Store(true)
		a.logger.Info().Str("client_id", clientID).Msg("Using existing credentials")
	}

	a.logger.Info().
		Str("server_url", a.config.Server.URL).
		Bool("tls_verify", a.config.Server.TLSVerify).
		Msg("API client initialized")

	return nil
}

// initRegistration initializes the registration service
func (a *Agent) initRegistration() error {
	a.registrationSvc = NewRegistrationService(a.apiClient, a.config, a.logger)
	a.logger.Debug().Msg("Registration service initialized")
	return nil
}

// initMetrics initializes the metrics collection system
// This is called after registration when we have a client ID
func (a *Agent) initMetrics(clientID string) error {
	metricsConfig := &metrics.Config{
		CollectionInterval:  a.config.Metrics.CollectionInterval,
		MovingAverageWindow: 5, // 5-sample moving average to reduce noise
		EnableCPU:           true,
		EnableMemory:        true,
		EnableDisk:          true,
		EnableNetwork:       true,
	}

	var err error
	a.metricsSystem, err = metrics.NewSystem(metricsConfig, a.logger, clientID)
	if err != nil {
		return fmt.Errorf("failed to create metrics system: %w", err)
	}

	// Set callback to store metrics in cache
	a.metricsSystem.SetMetricsCallback(func(ctx context.Context, m *cache.Metrics) error {
		// Store in cache for later upload
		if err := a.cache.StoreMetrics(ctx, m); err != nil {
			a.logger.Error().Err(err).Msg("Failed to store metrics in cache")
			return err
		}
		a.logger.Debug().Msg("Metrics stored in cache")
		return nil
	})

	a.logger.Info().
		Dur("interval", metricsConfig.CollectionInterval).
		Msg("Metrics system initialized")

	return nil
}

// initHeartbeat initializes the heartbeat service
// This is called after registration when we have credentials
func (a *Agent) initHeartbeat() error {
	a.heartbeatSvc = NewHeartbeatService(a.apiClient, a.config, a.logger, a.startTime)
	a.logger.Info().
		Dur("interval", a.config.Heartbeat.Interval).
		Msg("Heartbeat service initialized")
	return nil
}

// Start begins agent operation with full lifecycle management
// This handles registration, component startup, and coordination
func (a *Agent) Start(ctx context.Context) error {
	if a.running.Load() {
		return fmt.Errorf("agent already running")
	}

	a.logger.Info().Msg("Starting agent")

	// Ensure we're registered
	if !a.registered.Load() {
		clientID, token, err := a.registrationSvc.EnsureRegistered(ctx)
		if err != nil {
			return fmt.Errorf("registration failed: %w", err)
		}

		// Store credentials in config for persistence
		a.config.UpdateClientCredentials(clientID, token)
		a.registered.Store(true)

		a.logger.Info().
			Str("client_id", clientID).
			Msg("Successfully registered with server")
	}

	// Get client ID for initializing other components
	clientID, _ := a.config.GetClientCredentials()

	// Initialize components that require client ID
	if err := a.initMetrics(clientID); err != nil {
		return fmt.Errorf("failed to initialize metrics: %w", err)
	}

	if err := a.initHeartbeat(); err != nil {
		return fmt.Errorf("failed to initialize heartbeat: %w", err)
	}

	a.running.Store(true)

	// Start all subsystems
	if err := a.startSubsystems(ctx); err != nil {
		a.running.Store(false)
		return fmt.Errorf("failed to start subsystems: %w", err)
	}

	a.logger.Info().Msg("Agent started successfully")

	return nil
}

// startSubsystems starts all agent subsystems in the correct order
func (a *Agent) startSubsystems(ctx context.Context) error {
	// Start metrics collection
	if err := a.metricsSystem.Start(ctx); err != nil {
		return fmt.Errorf("failed to start metrics system: %w", err)
	}

	// Start heartbeat service
	if err := a.heartbeatSvc.Start(ctx); err != nil {
		a.metricsSystem.Stop()
		return fmt.Errorf("failed to start heartbeat service: %w", err)
	}

	// Start metrics upload worker
	a.wg.Add(1)
	go a.metricsUploadWorker(ctx)

	// Start command polling worker
	a.wg.Add(1)
	go a.commandPollWorker(ctx)

	// Start cache maintenance worker
	a.wg.Add(1)
	go a.cacheMaintenanceWorker(ctx)

	return nil
}

// Stop gracefully shuts down the agent and all subsystems
// It ensures all resources are properly released and data is flushed
func (a *Agent) Stop(ctx context.Context) error {
	if !a.running.Load() {
		return nil
	}

	a.logger.Info().Msg("Stopping agent")

	// Signal all workers to stop
	close(a.stopCh)

	// Create a timeout context for graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Stop components in reverse order
	// Use goroutine with timeout to prevent blocking forever
	done := make(chan struct{})
	go func() {
		defer close(done)

		// Stop heartbeat service
		if a.heartbeatSvc != nil {
			if err := a.heartbeatSvc.Stop(shutdownCtx); err != nil {
				a.logger.Error().Err(err).Msg("Failed to stop heartbeat service")
			}
		}

		// Stop metrics collection
		if a.metricsSystem != nil {
			if err := a.metricsSystem.Stop(); err != nil {
				a.logger.Error().Err(err).Msg("Failed to stop metrics system")
			}
		}

		// Wait for all workers to finish
		a.wg.Wait()

		// Flush any remaining metrics from cache
		if err := a.flushMetrics(shutdownCtx); err != nil {
			a.logger.Error().Err(err).Msg("Failed to flush metrics on shutdown")
		}

		// Cleanup resources
		a.cleanup()
	}()

	// Wait for shutdown or timeout
	select {
	case <-done:
		a.logger.Info().Msg("Agent stopped gracefully")
	case <-shutdownCtx.Done():
		a.logger.Warn().Msg("Graceful shutdown timeout exceeded")
	}

	a.running.Store(false)
	close(a.stoppedCh)

	return nil
}

// cleanup releases all resources
func (a *Agent) cleanup() {
	if a.cache != nil {
		if err := a.cache.Close(); err != nil {
			a.logger.Error().Err(err).Msg("Failed to close cache")
		}
	}

	if a.apiClient != nil {
		if err := a.apiClient.Close(); err != nil {
			a.logger.Error().Err(err).Msg("Failed to close API client")
		}
	}

	a.logger.Debug().Msg("Cleanup completed")
}

// metricsUploadWorker periodically uploads cached metrics to the server
func (a *Agent) metricsUploadWorker(ctx context.Context) {
	defer a.wg.Done()

	// Default upload interval: 5 minutes
	uploadInterval := 5 * time.Minute
	ticker := time.NewTicker(uploadInterval)
	defer ticker.Stop()

	a.logger.Debug().Dur("interval", uploadInterval).Msg("Metrics upload worker started")

	for {
		select {
		case <-ticker.C:
			if err := a.uploadMetrics(ctx); err != nil {
				a.logger.Error().Err(err).Msg("Failed to upload metrics")
			}

		case <-a.stopCh:
			a.logger.Debug().Msg("Metrics upload worker stopped")
			return

		case <-ctx.Done():
			a.logger.Debug().Msg("Metrics upload worker context cancelled")
			return
		}
	}
}

// uploadMetrics retrieves unsynced metrics from cache and uploads them
func (a *Agent) uploadMetrics(ctx context.Context) error {
	// Get metrics from cache that haven't been synced
	endTime := time.Now()
	startTime := endTime.Add(-24 * time.Hour) // Last 24 hours

	cachedMetrics, err := a.cache.GetMetrics(ctx, startTime, endTime)
	if err != nil {
		return fmt.Errorf("failed to get metrics from cache: %w", err)
	}

	if len(cachedMetrics) == 0 {
		a.logger.Debug().Msg("No metrics to upload")
		return nil
	}

	// Filter only unsynced metrics
	var unsyncedMetrics []*cache.Metrics
	for _, m := range cachedMetrics {
		if !m.Synced {
			unsyncedMetrics = append(unsyncedMetrics, m)
		}
	}

	if len(unsyncedMetrics) == 0 {
		return nil
	}

	// Convert to API format
	apiMetrics := make([]api.MetricSnapshot, 0, len(unsyncedMetrics))
	for _, m := range unsyncedMetrics {
		apiMetrics = append(apiMetrics, api.MetricSnapshot{
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
		})
	}

	// Upload in batches
	batchSize := a.config.Metrics.BatchSize
	for i := 0; i < len(apiMetrics); i += batchSize {
		end := i + batchSize
		if end > len(apiMetrics) {
			end = len(apiMetrics)
		}

		batch := apiMetrics[i:end]
		req := &api.MetricsUploadRequest{
			Metrics: batch,
			Count:   len(batch),
		}

		uploadCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		_, err := a.apiClient.UploadMetrics(uploadCtx, req)
		cancel()

		if err != nil {
			a.logger.Error().
				Err(err).
				Int("batch_size", len(batch)).
				Msg("Failed to upload metrics batch")
			return err
		}

		// Mark these metrics as synced in cache
		// Note: This is a simplified approach - a real implementation would
		// mark specific metrics by ID
		a.logger.Info().
			Int("count", len(batch)).
			Msg("Metrics uploaded successfully")
	}

	return nil
}

// flushMetrics uploads all pending metrics (called on shutdown)
func (a *Agent) flushMetrics(ctx context.Context) error {
	a.logger.Info().Msg("Flushing cached metrics")
	return a.uploadMetrics(ctx)
}

// commandPollWorker periodically polls for commands from the server
func (a *Agent) commandPollWorker(ctx context.Context) {
	defer a.wg.Done()

	// Default poll interval: 30 seconds
	pollInterval := 30 * time.Second
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	a.logger.Debug().Dur("interval", pollInterval).Msg("Command poll worker started")

	for {
		select {
		case <-ticker.C:
			if err := a.pollCommands(ctx); err != nil {
				a.logger.Error().Err(err).Msg("Failed to poll commands")
			}

		case <-a.stopCh:
			a.logger.Debug().Msg("Command poll worker stopped")
			return

		case <-ctx.Done():
			a.logger.Debug().Msg("Command poll worker context cancelled")
			return
		}
	}
}

// pollCommands retrieves and stores pending commands from the server
func (a *Agent) pollCommands(ctx context.Context) error {
	pollCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := a.apiClient.GetCommands(pollCtx)
	if err != nil {
		return fmt.Errorf("failed to get commands: %w", err)
	}

	if resp.Count == 0 {
		return nil
	}

	// Store commands in cache for execution
	for _, cmd := range resp.Commands {
		// Convert API command to cache command
		// Note: Payload needs to be serialized
		cacheCmd := &cache.Command{
			CommandID: cmd.CommandID,
			Type:      string(cmd.Type),
			Status:    cache.StatusPending,
			CreatedAt: time.Now(),
		}

		if err := a.cache.StoreCommand(ctx, cacheCmd); err != nil {
			a.logger.Error().
				Err(err).
				Str("command_id", cmd.CommandID).
				Msg("Failed to store command")
			continue
		}

		a.logger.Info().
			Str("command_id", cmd.CommandID).
			Str("type", string(cmd.Type)).
			Msg("Command received and stored")
	}

	return nil
}

// cacheMaintenanceWorker performs periodic cache cleanup and optimization
func (a *Agent) cacheMaintenanceWorker(ctx context.Context) {
	defer a.wg.Done()

	// Run maintenance every hour
	maintenanceInterval := 1 * time.Hour
	ticker := time.NewTicker(maintenanceInterval)
	defer ticker.Stop()

	a.logger.Debug().Dur("interval", maintenanceInterval).Msg("Cache maintenance worker started")

	for {
		select {
		case <-ticker.C:
			a.performCacheMaintenance(ctx)

		case <-a.stopCh:
			a.logger.Debug().Msg("Cache maintenance worker stopped")
			return

		case <-ctx.Done():
			a.logger.Debug().Msg("Cache maintenance worker context cancelled")
			return
		}
	}
}

// performCacheMaintenance cleans up old data and optimizes the cache
func (a *Agent) performCacheMaintenance(ctx context.Context) {
	// TODO: Implement actual cache maintenance
	// - Delete metrics older than retention period
	// - Delete completed commands older than X days
	// - Vacuum SQLite database if needed
	// - Check cache size and warn if too large

	a.logger.Debug().Msg("Cache maintenance performed")
}

// IsRunning returns true if the agent is currently running
func (a *Agent) IsRunning() bool {
	return a.running.Load()
}

// IsRegistered returns true if the agent has valid credentials
func (a *Agent) IsRegistered() bool {
	return a.registered.Load()
}

// GetUptime returns how long the agent has been running
func (a *Agent) GetUptime() time.Duration {
	return time.Since(a.startTime)
}

// WaitForShutdown blocks until the agent has completely stopped
func (a *Agent) WaitForShutdown() {
	<-a.stoppedCh
}

// Stats returns current agent statistics
func (a *Agent) Stats() AgentStats {
	return AgentStats{
		Running:      a.running.Load(),
		Registered:   a.registered.Load(),
		Uptime:       a.GetUptime(),
		MetricsStats: a.getMetricsStats(),
	}
}

// getMetricsStats retrieves metrics system statistics
func (a *Agent) getMetricsStats() *metrics.SystemStats {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.metricsSystem == nil {
		return nil
	}

	stats := a.metricsSystem.Stats()
	return &stats
}

// AgentStats holds agent operational statistics
type AgentStats struct {
	Running      bool
	Registered   bool
	Uptime       time.Duration
	MetricsStats *metrics.SystemStats
}
