package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLoadWithValidFile tests loading configuration from a valid YAML file
func TestLoadWithValidFile(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	configContent := `
server:
  url: "https://test.example.com"
  tls_verify: true
  timeout: 30s

client:
  id: "test-client-123"
  token: "test-token-abc"

heartbeat:
  interval: 30s
  retry_attempts: 3

metrics:
  collection_interval: 60s
  batch_size: 100
  retention_days: 7

cache:
  type: "sqlite"
  path: "/tmp/test.db"

logging:
  level: "info"
  format: "json"

package_managers:
  preferred: "auto"
`

	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load() error = %v, wantErr false", err)
	}

	// Verify configuration values
	if cfg.Server.URL != "https://test.example.com" {
		t.Errorf("Server.URL = %q, want %q", cfg.Server.URL, "https://test.example.com")
	}
	if !cfg.Server.TLSVerify {
		t.Error("Server.TLSVerify = false, want true")
	}
	if cfg.Server.Timeout != 30*time.Second {
		t.Errorf("Server.Timeout = %v, want 30s", cfg.Server.Timeout)
	}

	if cfg.Client.ID != "test-client-123" {
		t.Errorf("Client.ID = %q, want %q", cfg.Client.ID, "test-client-123")
	}
	if cfg.Client.Token != "test-token-abc" {
		t.Errorf("Client.Token = %q, want %q", cfg.Client.Token, "test-token-abc")
	}

	if cfg.Heartbeat.Interval != 30*time.Second {
		t.Errorf("Heartbeat.Interval = %v, want 30s", cfg.Heartbeat.Interval)
	}
	if cfg.Heartbeat.RetryAttempts != 3 {
		t.Errorf("Heartbeat.RetryAttempts = %d, want 3", cfg.Heartbeat.RetryAttempts)
	}

	if cfg.Cache.Type != "sqlite" {
		t.Errorf("Cache.Type = %q, want %q", cfg.Cache.Type, "sqlite")
	}

	if cfg.Logging.Level != "info" {
		t.Errorf("Logging.Level = %q, want %q", cfg.Logging.Level, "info")
	}
}

// TestLoadWithMissingFile tests loading when config file doesn't exist
func TestLoadWithMissingFile(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("Load() should succeed with missing file, got error: %v", err)
	}

	// Should use defaults
	if cfg.Server.URL != DefaultServerURL {
		t.Errorf("Server.URL = %q, want default %q", cfg.Server.URL, DefaultServerURL)
	}
}

// TestLoadWithDefaults tests that defaults are applied correctly
func TestLoadWithDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "minimal.yaml")

	// Minimal config - should use defaults for most values
	minimalConfig := `
server:
  url: "https://test.example.com"
`

	if err := os.WriteFile(configFile, []byte(minimalConfig), 0644); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Check that defaults were applied
	if cfg.Heartbeat.Interval != DefaultHeartbeatInterval {
		t.Errorf("Heartbeat.Interval = %v, want default %v", cfg.Heartbeat.Interval, DefaultHeartbeatInterval)
	}
	if cfg.Metrics.CollectionInterval != DefaultMetricsCollectionInterval {
		t.Errorf("Metrics.CollectionInterval = %v, want default %v", cfg.Metrics.CollectionInterval, DefaultMetricsCollectionInterval)
	}
}

// TestEnvironmentVariableOverrides tests that environment variables override config file
func TestEnvironmentVariableOverrides(t *testing.T) {
	// Skip if running in environment where env vars can't be set
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	configContent := `
server:
  url: "https://fileconfig.example.com"
  timeout: 30s
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	// Set environment variable
	t.Setenv("CASTLEOPS_SERVER_URL", "https://envvar.example.com")

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Environment variable should override file config
	if cfg.Server.URL != "https://envvar.example.com" {
		t.Errorf("Server.URL = %q, want %q (from env var)", cfg.Server.URL, "https://envvar.example.com")
	}
}

// TestValidateRequiredFields tests configuration validation
func TestValidateRequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				Server: ServerConfig{
					URL:     "https://example.com",
					Timeout: 30 * time.Second,
				},
				Heartbeat: HeartbeatConfig{
					Interval: 30 * time.Second,
				},
				Metrics: MetricsConfig{
					CollectionInterval: 60 * time.Second,
					BatchSize:          100,
					RetentionDays:      7,
				},
				Cache: CacheConfig{
					Type: "sqlite",
				},
				Logging: LoggingConfig{
					Level: "info",
				},
			},
			wantErr: false,
		},
		{
			name: "missing server URL",
			config: &Config{
				Server: ServerConfig{
					Timeout: 30 * time.Second,
				},
				Heartbeat: HeartbeatConfig{
					Interval: 30 * time.Second,
				},
				Metrics: MetricsConfig{
					CollectionInterval: 60 * time.Second,
					BatchSize:          100,
					RetentionDays:      7,
				},
				Cache: CacheConfig{
					Type: "sqlite",
				},
				Logging: LoggingConfig{
					Level: "info",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid timeout (zero)",
			config: &Config{
				Server: ServerConfig{
					URL:     "https://example.com",
					Timeout: 0,
				},
				Heartbeat: HeartbeatConfig{
					Interval: 30 * time.Second,
				},
				Metrics: MetricsConfig{
					CollectionInterval: 60 * time.Second,
					BatchSize:          100,
					RetentionDays:      7,
				},
				Cache: CacheConfig{
					Type: "sqlite",
				},
				Logging: LoggingConfig{
					Level: "info",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid cache type",
			config: &Config{
				Server: ServerConfig{
					URL:     "https://example.com",
					Timeout: 30 * time.Second,
				},
				Heartbeat: HeartbeatConfig{
					Interval: 30 * time.Second,
				},
				Metrics: MetricsConfig{
					CollectionInterval: 60 * time.Second,
					BatchSize:          100,
					RetentionDays:      7,
				},
				Cache: CacheConfig{
					Type: "invalid",
				},
				Logging: LoggingConfig{
					Level: "info",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid log level",
			config: &Config{
				Server: ServerConfig{
					URL:     "https://example.com",
					Timeout: 30 * time.Second,
				},
				Heartbeat: HeartbeatConfig{
					Interval: 30 * time.Second,
				},
				Metrics: MetricsConfig{
					CollectionInterval: 60 * time.Second,
					BatchSize:          100,
					RetentionDays:      7,
				},
				Cache: CacheConfig{
					Type: "sqlite",
				},
				Logging: LoggingConfig{
					Level: "invalid_level",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestClientCredentialsThreadSafety tests concurrent access to credentials
func TestClientCredentialsThreadSafety(t *testing.T) {
	cfg := NewDefault()

	// Test concurrent writes and reads
	done := make(chan bool, 2)

	go func() {
		for i := 0; i < 100; i++ {
			cfg.UpdateClientCredentials("id-"+string(rune(i)), "token-"+string(rune(i)))
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			cfg.GetClientCredentials()
		}
		done <- true
	}()

	<-done
	<-done
}

// TestPathExpansion tests tilde and environment variable expansion
func TestPathExpansion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "absolute path",
			input:   "/tmp/test.db",
			wantErr: false,
		},
		{
			name:    "environment variable",
			input:   "$HOME/.castleops/data.db",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := expandPath(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("expandPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result == "" {
				t.Error("expandPath() returned empty string")
			}
		})
	}
}

// TestLoadOrDefault returns default config on error
func TestLoadOrDefault(t *testing.T) {
	cfg := LoadOrDefault("/nonexistent/config.yaml")
	if cfg == nil {
		t.Fatal("LoadOrDefault() returned nil")
	}

	// Should have defaults
	if cfg.Server.URL != DefaultServerURL {
		t.Errorf("Server.URL = %q, want default %q", cfg.Server.URL, DefaultServerURL)
	}
}

// TestGlobalConfigFunctions tests Get/Set/Reset functions
func TestGlobalConfigFunctions(t *testing.T) {
	// Reset to clean state
	Reset()

	// First Get should load or create default
	cfg1 := Get()
	if cfg1 == nil {
		t.Fatal("Get() returned nil")
	}

	// Second Get should return same instance
	cfg2 := Get()
	if cfg1 != cfg2 {
		t.Error("Get() should return same instance")
	}

	// Set should change global instance
	newCfg := NewDefault()
	newCfg.Server.URL = "https://changed.example.com"
	Set(newCfg)

	cfg3 := Get()
	if cfg3.Server.URL != "https://changed.example.com" {
		t.Errorf("Server.URL = %q, want %q", cfg3.Server.URL, "https://changed.example.com")
	}

	// Reset should clear the global instance
	Reset()
	// Get after reset should create a new instance (with defaults)
	cfg4 := Get()
	if cfg4.Server.URL != DefaultServerURL {
		t.Errorf("After Reset, Server.URL = %q, want default %q", cfg4.Server.URL, DefaultServerURL)
	}
}

// BenchmarkConfigLoad benchmarks configuration loading
func BenchmarkConfigLoad(b *testing.B) {
	tmpDir := b.TempDir()
	configFile := filepath.Join(tmpDir, "bench.yaml")

	configContent := `
server:
  url: "https://test.example.com"
  timeout: 30s
cache:
  type: "sqlite"
  path: "/tmp/test.db"
`

	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		b.Fatalf("Failed to create config file: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = Load(configFile)
	}
}

// BenchmarkGetClientCredentials benchmarks credential retrieval
func BenchmarkGetClientCredentials(b *testing.B) {
	cfg := NewDefault()
	cfg.UpdateClientCredentials("test-id", "test-token")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = cfg.GetClientCredentials()
	}
}
