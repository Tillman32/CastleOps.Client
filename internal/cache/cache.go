// Package cache provides command and metrics caching functionality for persistence
// across agent restarts and offline operation.
package cache

import (
	"context"
	"time"

	"github.com/rs/zerolog"
)

// Command status constants
const (
	StatusPending   = "pending"
	StatusComplete  = "complete"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

// Command represents a cached command for persistence
type Command struct {
	// ID is the database primary key (auto-generated)
	ID int64

	// CommandID is the unique identifier for the command
	CommandID string

	// Type is the command type (e.g., "install_package", "execute_script")
	Type string

	// Payload is the JSON-serialized command payload
	Payload string

	// Status is the current command status
	Status string

	// Result contains the execution result or error message
	Result string

	// CreatedAt is when the command was received
	CreatedAt time.Time

	// CompletedAt is when the command finished execution
	CompletedAt time.Time
}

// Metrics represents collected system metrics for caching
type Metrics struct {
	// ID is the database primary key (auto-generated)
	ID int64

	// Timestamp is when the metrics were collected
	Timestamp time.Time

	// ClientID identifies the client that collected these metrics
	ClientID string

	// CPU metrics
	CPUUsagePercent float64

	// Memory metrics
	MemoryTotal        uint64
	MemoryUsed         uint64
	MemoryAvailable    uint64
	MemoryUsagePercent float64

	// Disk metrics
	DiskTotalBytes   uint64
	DiskUsedBytes    uint64
	DiskFreeBytes    uint64
	DiskUsagePercent float64

	// Network metrics
	NetworkBytesReceived uint64
	NetworkBytesSent     uint64

	// Synced indicates if this metric has been uploaded to the server
	Synced bool
}

// MemoryConfig configures the in-memory cache
type MemoryConfig struct {
	// RetentionDays is how long to keep metrics (default: 7)
	RetentionDays int

	// Logger for structured logging
	Logger zerolog.Logger
}

// Cache defines the interface for command and metrics caching
type Cache interface {
	// StoreCommand saves a command to the cache
	StoreCommand(ctx context.Context, cmd *Command) error

	// GetPendingCommands retrieves all commands with pending status
	GetPendingCommands(ctx context.Context) ([]*Command, error)

	// MarkCommandComplete marks a command as completed
	MarkCommandComplete(ctx context.Context, commandID string) error

	// StoreMetrics saves metrics to the cache
	StoreMetrics(ctx context.Context, metrics *Metrics) error

	// GetMetrics retrieves metrics within the specified time range
	GetMetrics(ctx context.Context, start, end time.Time) ([]*Metrics, error)

	// Close releases any resources held by the cache
	Close() error
}
