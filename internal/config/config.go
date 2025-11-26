package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/viper"
)

// Config represents the complete application configuration
// All fields are designed to minimize allocations during access
type Config struct {
	Server         ServerConfig         `mapstructure:"server"`
	Client         ClientConfig         `mapstructure:"client"`
	Heartbeat      HeartbeatConfig      `mapstructure:"heartbeat"`
	Metrics        MetricsConfig        `mapstructure:"metrics"`
	Cache          CacheConfig          `mapstructure:"cache"`
	Logging        LoggingConfig        `mapstructure:"logging"`
	PackageManagers PackageManagersConfig `mapstructure:"package_managers"`

	// Internal fields for efficient access
	mu     sync.RWMutex
	logger zerolog.Logger
}

// ServerConfig holds server connection settings
type ServerConfig struct {
	URL       string        `mapstructure:"url"`
	TLSVerify bool          `mapstructure:"tls_verify"`
	Timeout   time.Duration `mapstructure:"timeout"`
}

// ClientConfig holds client identification
type ClientConfig struct {
	ID    string `mapstructure:"id"`
	Token string `mapstructure:"token"`
}

// HeartbeatConfig holds heartbeat service settings
type HeartbeatConfig struct {
	Interval      time.Duration `mapstructure:"interval"`
	RetryAttempts int           `mapstructure:"retry_attempts"`
}

// MetricsConfig holds metrics collection settings
type MetricsConfig struct {
	CollectionInterval time.Duration `mapstructure:"collection_interval"`
	BatchSize          int           `mapstructure:"batch_size"`
	RetentionDays      int           `mapstructure:"retention_days"`
}

// CacheConfig holds cache backend settings
type CacheConfig struct {
	Type string `mapstructure:"type"`
	Path string `mapstructure:"path"`
}

// LoggingConfig holds logging settings
type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Path   string `mapstructure:"path"`
	Format string `mapstructure:"format"` // json or console
}

// PackageManagersConfig holds package manager settings
type PackageManagersConfig struct {
	Preferred string `mapstructure:"preferred"`
}

var (
	// globalConfig is the cached configuration instance
	// This is safe for concurrent reads after initialization
	globalConfig     *Config
	globalConfigOnce sync.Once
	globalConfigMu   sync.RWMutex
)

// Load reads configuration from file and environment variables
// It uses viper for flexible configuration management with the following precedence:
// 1. Environment variables (highest priority)
// 2. Configuration file
// 3. Default values (lowest priority)
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Set up viper configuration
	v.SetConfigType("yaml")

	if configPath != "" {
		// Use specified config file
		v.SetConfigFile(configPath)
	} else {
		// Search for config in standard locations
		v.SetConfigName("config")
		v.AddConfigPath(".")
		v.AddConfigPath("./configs")
		v.AddConfigPath("/etc/castleops")
		v.AddConfigPath("$HOME/.castleops")
	}

	// Enable environment variable overrides
	// Environment variables are prefixed with CASTLEOPS_ and use underscores
	// Example: CASTLEOPS_SERVER_URL, CASTLEOPS_LOGGING_LEVEL
	v.SetEnvPrefix("CASTLEOPS")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Set defaults to ensure we always have valid values
	setDefaults(v)

	// Read configuration file (if it exists)
	if err := v.ReadInConfig(); err != nil {
		// It's acceptable if config file doesn't exist - we'll use defaults
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// Unmarshal into Config struct
	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate and post-process configuration
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	if err := cfg.expandPaths(); err != nil {
		return nil, fmt.Errorf("failed to expand paths: %w", err)
	}

	return cfg, nil
}

// LoadOrDefault loads configuration or returns defaults on error
// This is useful for scenarios where you want the application to start
// even if configuration is missing or invalid
func LoadOrDefault(configPath string) *Config {
	cfg, err := Load(configPath)
	if err != nil {
		// Return default configuration
		return NewDefault()
	}
	return cfg
}

// Get returns the global configuration instance
// This is thread-safe and ensures the configuration is loaded only once
func Get() *Config {
	globalConfigMu.RLock()
	if globalConfig != nil {
		defer globalConfigMu.RUnlock()
		return globalConfig
	}
	globalConfigMu.RUnlock()

	// Need to initialize
	globalConfigMu.Lock()
	defer globalConfigMu.Unlock()

	// Double-check in case another goroutine initialized while we waited
	if globalConfig != nil {
		return globalConfig
	}

	// Load with default path
	cfg, err := Load("")
	if err != nil {
		// Fall back to defaults
		cfg = NewDefault()
	}
	globalConfig = cfg
	return globalConfig
}

// Set updates the global configuration instance
// This is primarily useful for testing
func Set(cfg *Config) {
	globalConfigMu.Lock()
	defer globalConfigMu.Unlock()
	globalConfig = cfg
}

// Reset clears the global configuration (useful for testing)
func Reset() {
	globalConfigMu.Lock()
	defer globalConfigMu.Unlock()
	globalConfig = nil
}

// validate ensures configuration values are valid
func (c *Config) validate() error {
	// Validate server URL
	if c.Server.URL == "" {
		return fmt.Errorf("server.url is required")
	}

	// Validate timeout
	if c.Server.Timeout <= 0 {
		return fmt.Errorf("server.timeout must be positive")
	}

	// Validate heartbeat interval
	if c.Heartbeat.Interval <= 0 {
		return fmt.Errorf("heartbeat.interval must be positive")
	}

	// Validate metrics collection interval
	if c.Metrics.CollectionInterval <= 0 {
		return fmt.Errorf("metrics.collection_interval must be positive")
	}

	// Validate batch size
	if c.Metrics.BatchSize <= 0 {
		return fmt.Errorf("metrics.batch_size must be positive")
	}

	// Validate retention days
	if c.Metrics.RetentionDays < 0 {
		return fmt.Errorf("metrics.retention_days cannot be negative")
	}

	// Validate cache type
	if c.Cache.Type != "sqlite" && c.Cache.Type != "memory" {
		return fmt.Errorf("cache.type must be 'sqlite' or 'memory'")
	}

	// Validate log level
	validLevels := map[string]bool{
		"trace": true, "debug": true, "info": true,
		"warn": true, "error": true, "fatal": true, "panic": true,
	}
	if !validLevels[c.Logging.Level] {
		return fmt.Errorf("logging.level must be one of: trace, debug, info, warn, error, fatal, panic")
	}

	// Validate log format
	if c.Logging.Format != "" && c.Logging.Format != "json" && c.Logging.Format != "console" {
		return fmt.Errorf("logging.format must be 'json' or 'console'")
	}

	return nil
}

// expandPaths expands environment variables and tildes in path configurations
func (c *Config) expandPaths() error {
	var err error

	// Expand cache path
	if c.Cache.Path != "" {
		c.Cache.Path, err = expandPath(c.Cache.Path)
		if err != nil {
			return fmt.Errorf("failed to expand cache.path: %w", err)
		}
	}

	// Expand log path
	if c.Logging.Path != "" {
		c.Logging.Path, err = expandPath(c.Logging.Path)
		if err != nil {
			return fmt.Errorf("failed to expand logging.path: %w", err)
		}
	}

	return nil
}

// expandPath expands ~ and environment variables in a path
func expandPath(path string) (string, error) {
	// Expand environment variables
	expanded := os.ExpandEnv(path)

	// Expand tilde
	if strings.HasPrefix(expanded, "~") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		expanded = filepath.Join(homeDir, expanded[1:])
	}

	return expanded, nil
}

// UpdateClientCredentials atomically updates client ID and token
// This is thread-safe and efficient for runtime updates
func (c *Config) UpdateClientCredentials(id, token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Client.ID = id
	c.Client.Token = token
}

// GetClientCredentials atomically retrieves client ID and token
// Returns copies to avoid race conditions
func (c *Config) GetClientCredentials() (id, token string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Client.ID, c.Client.Token
}

// setDefaults configures default values in viper
func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.url", DefaultServerURL)
	v.SetDefault("server.tls_verify", DefaultTLSVerify)
	v.SetDefault("server.timeout", DefaultServerTimeout)

	// Client defaults (empty until registration)
	v.SetDefault("client.id", "")
	v.SetDefault("client.token", "")

	// Heartbeat defaults
	v.SetDefault("heartbeat.interval", DefaultHeartbeatInterval)
	v.SetDefault("heartbeat.retry_attempts", DefaultHeartbeatRetryAttempts)

	// Metrics defaults
	v.SetDefault("metrics.collection_interval", DefaultMetricsCollectionInterval)
	v.SetDefault("metrics.batch_size", DefaultMetricsBatchSize)
	v.SetDefault("metrics.retention_days", DefaultMetricsRetentionDays)

	// Cache defaults
	v.SetDefault("cache.type", DefaultCacheType)
	v.SetDefault("cache.path", DefaultCachePath)

	// Logging defaults
	v.SetDefault("logging.level", DefaultLogLevel)
	v.SetDefault("logging.path", DefaultLogPath)
	v.SetDefault("logging.format", DefaultLogFormat)

	// Package manager defaults
	v.SetDefault("package_managers.preferred", DefaultPackageManagerPreferred)
}
