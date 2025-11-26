package config

import (
	"runtime"
	"time"
)

// Default configuration values
// These are optimized for minimal resource usage while maintaining functionality
const (
	// Server defaults
	DefaultServerURL     = "https://api.castleops.com"
	DefaultTLSVerify     = true
	DefaultServerTimeout = 30 * time.Second

	// Heartbeat defaults
	// 30 seconds provides good responsiveness without excessive overhead
	DefaultHeartbeatInterval      = 30 * time.Second
	DefaultHeartbeatRetryAttempts = 3

	// Metrics defaults
	// 60 second collection interval balances data granularity with performance
	DefaultMetricsCollectionInterval = 60 * time.Second
	DefaultMetricsBatchSize          = 100
	DefaultMetricsRetentionDays      = 7

	// Cache defaults
	DefaultCacheType = "sqlite"

	// Logging defaults
	DefaultLogLevel  = "info"
	DefaultLogFormat = "json"

	// Package manager defaults
	DefaultPackageManagerPreferred = "auto"
)

var (
	// Platform-specific defaults
	// These are determined at runtime based on the operating system
	DefaultCachePath string
	DefaultLogPath   string
)

func init() {
	// Initialize platform-specific defaults
	initPlatformDefaults()
}

// initPlatformDefaults sets up OS-specific default paths
func initPlatformDefaults() {
	switch runtime.GOOS {
	case "darwin":
		// macOS defaults
		DefaultCachePath = "~/Library/Application Support/CastleOps/data.db"
		DefaultLogPath = "~/Library/Logs/CastleOps/client.log"

	case "windows":
		// Windows defaults
		DefaultCachePath = "%LOCALAPPDATA%\\CastleOps\\data.db"
		DefaultLogPath = "%LOCALAPPDATA%\\CastleOps\\Logs\\client.log"

	case "linux":
		// Linux defaults (for future support)
		DefaultCachePath = "/var/lib/castleops/data.db"
		DefaultLogPath = "/var/log/castleops/client.log"

	default:
		// Fallback to current directory
		DefaultCachePath = "./data.db"
		DefaultLogPath = "./client.log"
	}
}

// NewDefault creates a new Config with all default values
// This is useful for testing or when no configuration file exists
func NewDefault() *Config {
	return &Config{
		Server: ServerConfig{
			URL:       DefaultServerURL,
			TLSVerify: DefaultTLSVerify,
			Timeout:   DefaultServerTimeout,
		},
		Client: ClientConfig{
			ID:    "",
			Token: "",
		},
		Heartbeat: HeartbeatConfig{
			Interval:      DefaultHeartbeatInterval,
			RetryAttempts: DefaultHeartbeatRetryAttempts,
		},
		Metrics: MetricsConfig{
			CollectionInterval: DefaultMetricsCollectionInterval,
			BatchSize:          DefaultMetricsBatchSize,
			RetentionDays:      DefaultMetricsRetentionDays,
		},
		Cache: CacheConfig{
			Type: DefaultCacheType,
			Path: DefaultCachePath,
		},
		Logging: LoggingConfig{
			Level:  DefaultLogLevel,
			Path:   DefaultLogPath,
			Format: DefaultLogFormat,
		},
		PackageManagers: PackageManagersConfig{
			Preferred: DefaultPackageManagerPreferred,
		},
	}
}

// GetPlatformDefaults returns platform-specific default paths
// This is useful for documentation or displaying defaults to users
func GetPlatformDefaults() map[string]string {
	return map[string]string{
		"cache_path": DefaultCachePath,
		"log_path":   DefaultLogPath,
	}
}
