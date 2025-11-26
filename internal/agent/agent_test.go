package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/castleops/client/internal/api"
	"github.com/castleops/client/internal/cache"
	"github.com/castleops/client/internal/config"
	"github.com/rs/zerolog"
)

// testLogger creates a no-op logger for testing
func testLogger() zerolog.Logger {
	return zerolog.New(os.Stderr).Level(zerolog.Disabled)
}

// createTestConfig creates a minimal valid config for testing
func createTestConfig(t *testing.T) *config.Config {
	tmpDir := t.TempDir()

	return &config.Config{
		Server: config.ServerConfig{
			URL:       "https://localhost:8080",
			TLSVerify: false,
			Timeout:   30 * time.Second,
		},
		Client: config.ClientConfig{
			ID:    "",
			Token: "",
		},
		Heartbeat: config.HeartbeatConfig{
			Interval:      30 * time.Second,
			RetryAttempts: 3,
		},
		Metrics: config.MetricsConfig{
			CollectionInterval: 60 * time.Second,
			BatchSize:          100,
			RetentionDays:      7,
		},
		Cache: config.CacheConfig{
			Type: "memory",
			Path: filepath.Join(tmpDir, "test.db"),
		},
		Logging: config.LoggingConfig{
			Level:  "error",
			Format: "json",
		},
		PackageManagers: config.PackageManagersConfig{
			Preferred: "auto",
		},
	}
}

// TestNewAgent tests agent creation with valid config
func TestNewAgent(t *testing.T) {
	cfg := createTestConfig(t)
	logger := testLogger()

	agent, err := NewAgent(cfg, logger)
	if err != nil {
		t.Fatalf("NewAgent() error = %v, wantErr false", err)
	}

	if agent == nil {
		t.Fatal("NewAgent() returned nil")
	}

	if agent.config != cfg {
		t.Error("Agent config not set correctly")
	}

	// Cleanup
	agent.cleanup()
}

// TestNewAgentWithNilConfig tests that NewAgent rejects nil config
func TestNewAgentWithNilConfig(t *testing.T) {
	logger := testLogger()

	_, err := NewAgent(nil, logger)
	if err == nil {
		t.Error("NewAgent() should fail with nil config")
	}
}

// TestAgentInitialization tests that all components are initialized
func TestAgentInitialization(t *testing.T) {
	cfg := createTestConfig(t)
	logger := testLogger()

	agent, err := NewAgent(cfg, logger)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	defer agent.cleanup()

	if agent.cache == nil {
		t.Error("Cache not initialized")
	}

	if agent.apiClient == nil {
		t.Error("API client not initialized")
	}

	if agent.registrationSvc == nil {
		t.Error("Registration service not initialized")
	}
}

// TestAgentIsRunning tests the IsRunning state flag
func TestAgentIsRunning(t *testing.T) {
	cfg := createTestConfig(t)
	logger := testLogger()

	agent, err := NewAgent(cfg, logger)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	defer agent.cleanup()

	if agent.IsRunning() {
		t.Error("Agent should not be running initially")
	}

	agent.running.Store(true)
	if !agent.IsRunning() {
		t.Error("Agent should report as running")
	}
}

// TestAgentIsRegistered tests the IsRegistered state flag
func TestAgentIsRegistered(t *testing.T) {
	cfg := createTestConfig(t)
	logger := testLogger()

	agent, err := NewAgent(cfg, logger)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	defer agent.cleanup()

	if agent.IsRegistered() {
		t.Error("Agent should not be registered initially")
	}

	agent.registered.Store(true)
	if !agent.IsRegistered() {
		t.Error("Agent should report as registered")
	}
}

// TestAgentGetUptime tests uptime calculation
func TestAgentGetUptime(t *testing.T) {
	cfg := createTestConfig(t)
	logger := testLogger()

	agent, err := NewAgent(cfg, logger)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	defer agent.cleanup()

	uptime := agent.GetUptime()
	if uptime < 0 {
		t.Errorf("Uptime should be non-negative, got %v", uptime)
	}

	// Uptime should be very small initially
	if uptime > 100*time.Millisecond {
		t.Errorf("Uptime should be small initially, got %v", uptime)
	}
}

// TestAgentStats tests stats collection
func TestAgentStats(t *testing.T) {
	cfg := createTestConfig(t)
	logger := testLogger()

	agent, err := NewAgent(cfg, logger)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	defer agent.cleanup()

	stats := agent.Stats()

	if stats.Running {
		t.Error("Stats.Running should be false initially")
	}

	if stats.Registered {
		t.Error("Stats.Registered should be false initially")
	}

	if stats.Uptime < 0 {
		t.Errorf("Stats.Uptime should be non-negative, got %v", stats.Uptime)
	}
}

// TestAgentCannotStartTwice tests that agent can't be started twice
func TestAgentCannotStartTwice(t *testing.T) {
	cfg := createTestConfig(t)
	cfg.Client.ID = "test-id"
	cfg.Client.Token = "test-token"
	logger := testLogger()

	agent, err := NewAgent(cfg, logger)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	defer agent.cleanup()

	agent.running.Store(true)

	err = agent.Start(context.Background())
	if err == nil {
		t.Error("Start() should fail when already running")
	}
}

// TestAgentStopWhenNotRunning tests that Stop is safe when not running
func TestAgentStopWhenNotRunning(t *testing.T) {
	cfg := createTestConfig(t)
	logger := testLogger()

	agent, err := NewAgent(cfg, logger)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	defer agent.cleanup()

	// Should not error when stopping a non-running agent
	err = agent.Stop(context.Background())
	if err != nil {
		t.Errorf("Stop() on non-running agent error = %v, wantErr false", err)
	}
}

// TestAgentContextCancellation tests agent shutdown on context cancellation
func TestAgentContextCancellation(t *testing.T) {
	cfg := createTestConfig(t)
	cfg.Client.ID = "test-id"
	cfg.Client.Token = "test-token"
	logger := testLogger()

	agent, err := NewAgent(cfg, logger)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	defer agent.cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start in a goroutine with a timeout
	go func() {
		// This should timeout and eventually stop
		_ = agent.Start(ctx)
	}()

	// Give it time to start
	time.Sleep(100 * time.Millisecond)

	// Should be running (or at least attempted)
	// Cancel context and wait for shutdown
	cancel()
	time.Sleep(500 * time.Millisecond)
}

// TestCacheInitialization tests that cache is properly initialized
func TestCacheInitialization(t *testing.T) {
	cfg := createTestConfig(t)
	logger := testLogger()

	agent, err := NewAgent(cfg, logger)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	defer agent.cleanup()

	// Test with memory cache
	ctx := context.Background()
	testMetrics := &cache.Metrics{
		ClientID:           "test",
		Timestamp:          time.Now(),
		CPUUsagePercent:    50.0,
		MemoryUsagePercent: 60.0,
	}

	err = agent.cache.StoreMetrics(ctx, testMetrics)
	if err != nil {
		t.Errorf("Failed to store metrics in initialized cache: %v", err)
	}
}

// TestAPIClientInitialization tests API client setup
func TestAPIClientInitialization(t *testing.T) {
	cfg := createTestConfig(t)
	logger := testLogger()

	agent, err := NewAgent(cfg, logger)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	defer agent.cleanup()

	if agent.apiClient == nil {
		t.Fatal("API client not initialized")
	}

	// Verify credentials are set if already registered
	cfg.Client.ID = "pre-registered-id"
	cfg.Client.Token = "pre-registered-token"

	agent2, err := NewAgent(cfg, logger)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	defer agent2.cleanup()

	clientID, token := agent2.apiClient.GetCredentials()
	if clientID != "pre-registered-id" || token != "pre-registered-token" {
		t.Errorf("Credentials not set correctly: got %q, %q", clientID, token)
	}
}

// BenchmarkNewAgent benchmarks agent creation
func BenchmarkNewAgent(b *testing.B) {
	cfg := createTestConfig(&testing.T{})
	logger := testLogger()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		agent, _ := NewAgent(cfg, logger)
		if agent != nil {
			agent.cleanup()
		}
	}
}

// BenchmarkAgentStats benchmarks stats retrieval
func BenchmarkAgentStats(b *testing.B) {
	cfg := createTestConfig(&testing.T{})
	logger := testLogger()

	agent, _ := NewAgent(cfg, logger)
	defer agent.cleanup()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = agent.Stats()
	}
}
