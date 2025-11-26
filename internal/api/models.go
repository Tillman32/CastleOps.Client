package api

import (
	"time"
)

// RegisterRequest contains client registration information
type RegisterRequest struct {
	// Hostname of the client machine
	Hostname string `json:"hostname"`

	// OS identifies the operating system (darwin, windows, linux)
	OS string `json:"os"`

	// OSVersion contains the OS version string
	OSVersion string `json:"os_version"`

	// Architecture identifies the CPU architecture (amd64, arm64, etc.)
	Architecture string `json:"architecture"`

	// AgentVersion is the version of this client software
	AgentVersion string `json:"agent_version"`
}

// RegisterResponse contains credentials and configuration from server
type RegisterResponse struct {
	// ClientID is the unique identifier assigned by the server
	ClientID string `json:"client_id"`

	// Token is the authentication token for subsequent requests
	Token string `json:"token"`

	// HeartbeatInterval specifies how often to send heartbeats (in seconds)
	// If 0, use client default
	HeartbeatInterval int `json:"heartbeat_interval,omitempty"`

	// MetricsInterval specifies how often to collect metrics (in seconds)
	// If 0, use client default
	MetricsInterval int `json:"metrics_interval,omitempty"`
}

// HeartbeatRequest contains client health status
type HeartbeatRequest struct {
	// Status indicates client health (online, degraded, offline)
	Status string `json:"status"`

	// Uptime is the time since client started (in seconds)
	Uptime int64 `json:"uptime"`

	// LastMetricTime is when metrics were last collected
	LastMetricTime time.Time `json:"last_metric_time,omitempty"`

	// PendingCommands is the count of commands awaiting execution
	PendingCommands int `json:"pending_commands,omitempty"`

	// Version is the current agent version
	Version string `json:"version"`
}

// HeartbeatResponse contains server acknowledgment and configuration updates
type HeartbeatResponse struct {
	// Acknowledged confirms the heartbeat was received
	Acknowledged bool `json:"acknowledged"`

	// ConfigUpdate contains optional configuration changes
	ConfigUpdate *ConfigUpdate `json:"config_update,omitempty"`

	// Commands contains any new commands to execute
	Commands []*Command `json:"commands,omitempty"`
}

// ConfigUpdate represents configuration changes from the server
type ConfigUpdate struct {
	// HeartbeatInterval updates the heartbeat frequency (seconds)
	HeartbeatInterval *int `json:"heartbeat_interval,omitempty"`

	// MetricsInterval updates the metrics collection frequency (seconds)
	MetricsInterval *int `json:"metrics_interval,omitempty"`
}

// MetricsUploadRequest contains batched metrics data
type MetricsUploadRequest struct {
	// Metrics contains the array of metric snapshots
	Metrics []MetricSnapshot `json:"metrics"`

	// Count is the number of metrics in this batch
	Count int `json:"count"`
}

// MetricSnapshot represents a single metrics collection point
type MetricSnapshot struct {
	// Timestamp when metrics were collected (UTC)
	Timestamp time.Time `json:"timestamp"`

	// CPU metrics
	CPUUsagePercent float64 `json:"cpu_usage_percent"`

	// Memory metrics (bytes)
	MemoryTotal          uint64  `json:"memory_total"`
	MemoryUsed           uint64  `json:"memory_used"`
	MemoryAvailable      uint64  `json:"memory_available"`
	MemoryUsagePercent   float64 `json:"memory_usage_percent"`

	// Disk metrics (bytes)
	DiskTotalBytes   uint64  `json:"disk_total_bytes"`
	DiskUsedBytes    uint64  `json:"disk_used_bytes"`
	DiskFreeBytes    uint64  `json:"disk_free_bytes"`
	DiskUsagePercent float64 `json:"disk_usage_percent"`

	// Network metrics (cumulative bytes)
	NetworkBytesReceived uint64 `json:"network_bytes_received"`
	NetworkBytesSent     uint64 `json:"network_bytes_sent"`
}

// MetricsUploadResponse confirms metrics receipt
type MetricsUploadResponse struct {
	// Received is the count of metrics successfully stored
	Received int `json:"received"`

	// Acknowledged confirms processing
	Acknowledged bool `json:"acknowledged"`
}

// Command represents a command from the server to execute
type Command struct {
	// CommandID is the unique identifier for this command
	CommandID string `json:"command_id"`

	// Type specifies the command type
	Type CommandType `json:"type"`

	// Payload contains command-specific parameters (JSON)
	Payload interface{} `json:"payload"`

	// Priority indicates execution priority (0=normal, higher=more urgent)
	Priority int `json:"priority,omitempty"`

	// Timeout specifies max execution time in seconds (0=no timeout)
	Timeout int `json:"timeout,omitempty"`
}

// CommandType represents the type of command
type CommandType string

const (
	// CommandInstallPackage installs a software package
	CommandInstallPackage CommandType = "install_package"

	// CommandUninstallPackage removes a software package
	CommandUninstallPackage CommandType = "uninstall_package"

	// CommandUpdatePackage updates a software package
	CommandUpdatePackage CommandType = "update_package"

	// CommandQueryMetrics requests specific metrics
	CommandQueryMetrics CommandType = "query_metrics"

	// CommandUpdateConfig changes client configuration
	CommandUpdateConfig CommandType = "update_config"

	// CommandRestartAgent restarts the agent process
	CommandRestartAgent CommandType = "restart_agent"

	// CommandExecuteScript runs a custom script
	CommandExecuteScript CommandType = "execute_script"
)

// InstallPackagePayload contains parameters for package installation
type InstallPackagePayload struct {
	// PackageManager specifies which manager to use (chocolatey, homebrew, auto)
	PackageManager string `json:"package_manager"`

	// PackageName is the name of the package to install
	PackageName string `json:"package_name"`

	// Version specifies the version to install (empty or "latest" for newest)
	Version string `json:"version,omitempty"`

	// Options contains additional package manager specific options
	Options map[string]string `json:"options,omitempty"`
}

// UninstallPackagePayload contains parameters for package removal
type UninstallPackagePayload struct {
	// PackageManager specifies which manager to use
	PackageManager string `json:"package_manager"`

	// PackageName is the name of the package to uninstall
	PackageName string `json:"package_name"`

	// Force indicates whether to force removal
	Force bool `json:"force,omitempty"`
}

// UpdatePackagePayload contains parameters for package updates
type UpdatePackagePayload struct {
	// PackageManager specifies which manager to use
	PackageManager string `json:"package_manager"`

	// PackageName is the name of the package to update (empty for all)
	PackageName string `json:"package_name,omitempty"`

	// Version specifies the target version (empty or "latest" for newest)
	Version string `json:"version,omitempty"`
}

// QueryMetricsPayload contains parameters for metric queries
type QueryMetricsPayload struct {
	// MetricTypes specifies which metrics to query (cpu, memory, disk, network)
	MetricTypes []string `json:"metric_types"`

	// StartTime is the beginning of the time range (optional)
	StartTime *time.Time `json:"start_time,omitempty"`

	// EndTime is the end of the time range (optional)
	EndTime *time.Time `json:"end_time,omitempty"`
}

// UpdateConfigPayload contains configuration changes
type UpdateConfigPayload struct {
	// HeartbeatInterval updates heartbeat frequency (seconds)
	HeartbeatInterval *int `json:"heartbeat_interval,omitempty"`

	// MetricsInterval updates metrics collection frequency (seconds)
	MetricsInterval *int `json:"metrics_interval,omitempty"`

	// LogLevel updates logging level (trace, debug, info, warn, error)
	LogLevel *string `json:"log_level,omitempty"`
}

// ExecuteScriptPayload contains parameters for script execution
type ExecuteScriptPayload struct {
	// Script is the script content to execute
	Script string `json:"script"`

	// Interpreter specifies the interpreter (bash, powershell, python)
	Interpreter string `json:"interpreter"`

	// WorkingDirectory is the directory to execute from
	WorkingDirectory string `json:"working_directory,omitempty"`

	// Environment contains environment variables to set
	Environment map[string]string `json:"environment,omitempty"`
}

// CommandResultRequest contains the result of command execution
type CommandResultRequest struct {
	// CommandID identifies which command this result is for
	CommandID string `json:"command_id"`

	// Status indicates execution result (success, failed, timeout)
	Status string `json:"status"`

	// Output contains stdout/stderr from the command
	Output string `json:"output,omitempty"`

	// Error contains error message if status is failed
	Error string `json:"error,omitempty"`

	// ExecutionTime is how long the command took (milliseconds)
	ExecutionTime int64 `json:"execution_time"`

	// CompletedAt is when execution finished
	CompletedAt time.Time `json:"completed_at"`
}

// CommandResultResponse acknowledges result receipt
type CommandResultResponse struct {
	// Acknowledged confirms the result was received
	Acknowledged bool `json:"acknowledged"`
}

// ErrorResponse represents an API error
type ErrorResponse struct {
	// Error is the error message
	Error string `json:"error"`

	// Code is an optional error code
	Code string `json:"code,omitempty"`

	// Details contains additional error context
	Details map[string]interface{} `json:"details,omitempty"`
}

// CommandsResponse contains pending commands for the client
type CommandsResponse struct {
	// Commands contains the list of commands to execute
	Commands []*Command `json:"commands"`

	// Count is the number of commands
	Count int `json:"count"`
}

// CommandStatus constants for execution results
const (
	CommandStatusSuccess = "success"
	CommandStatusFailed  = "failed"
	CommandStatusTimeout = "timeout"
)

// ClientStatus constants for heartbeat
const (
	ClientStatusOnline   = "online"
	ClientStatusDegraded = "degraded"
	ClientStatusOffline  = "offline"
)
