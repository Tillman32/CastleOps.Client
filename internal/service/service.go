package service

import (
	"context"
	"fmt"
	"runtime"

	"github.com/rs/zerolog"
)

// Service defines the cross-platform interface for system service/daemon management.
// Implementations must handle platform-specific service installation, lifecycle,
// and configuration while providing a consistent interface.
type Service interface {
	// Install installs the service/daemon on the system.
	// This typically requires elevated privileges (admin/root).
	// Returns an error if installation fails or service already exists.
	Install() error

	// Uninstall removes the service/daemon from the system.
	// This stops the service if running and removes all configuration.
	// Returns an error if uninstallation fails.
	Uninstall() error

	// Start starts the installed service/daemon.
	// Returns an error if the service is not installed or fails to start.
	Start() error

	// Stop stops the running service/daemon.
	// Returns an error if the service is not running or fails to stop.
	Stop() error

	// Restart stops and starts the service.
	// Returns an error if restart fails.
	Restart() error

	// Status returns the current status of the service.
	// Returns StatusRunning, StatusStopped, StatusNotInstalled, or StatusUnknown.
	Status() (Status, error)

	// Run executes the service in service mode (blocking call).
	// This is called by the service manager and handles service lifecycle.
	// The provided function is the main service worker that will be executed.
	Run(worker func(ctx context.Context) error) error

	// Name returns the service name.
	Name() string

	// DisplayName returns the service display name.
	DisplayName() string
}

// Status represents the service state
type Status int

const (
	// StatusUnknown indicates the service status cannot be determined
	StatusUnknown Status = iota

	// StatusNotInstalled indicates the service is not installed
	StatusNotInstalled

	// StatusStopped indicates the service is installed but not running
	StatusStopped

	// StatusStarting indicates the service is starting
	StatusStarting

	// StatusRunning indicates the service is running
	StatusRunning

	// StatusStopping indicates the service is stopping
	StatusStopping
)

// String returns a string representation of the status
func (s Status) String() string {
	switch s {
	case StatusNotInstalled:
		return "not installed"
	case StatusStopped:
		return "stopped"
	case StatusStarting:
		return "starting"
	case StatusRunning:
		return "running"
	case StatusStopping:
		return "stopping"
	default:
		return "unknown"
	}
}

// Config holds service configuration
type Config struct {
	// Name is the service name (used as identifier)
	Name string

	// DisplayName is the human-readable service name
	DisplayName string

	// Description is the service description
	Description string

	// Executable is the full path to the service executable
	Executable string

	// Arguments are the command-line arguments passed to the executable
	Arguments []string

	// WorkingDirectory is the working directory for the service
	WorkingDirectory string

	// Dependencies are service dependencies (platform-specific)
	Dependencies []string

	// UserService indicates if this is a user-level service (macOS)
	// If false, it's a system-level service
	UserService bool

	// Logger for service operations
	Logger zerolog.Logger
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("service name is required")
	}
	if c.DisplayName == "" {
		return fmt.Errorf("service display name is required")
	}
	if c.Executable == "" {
		return fmt.Errorf("executable path is required")
	}
	return nil
}

// New creates a new platform-specific service instance.
// Returns an error if the configuration is invalid or the platform is unsupported.
func New(config *Config) (Service, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid service configuration: %w", err)
	}

	switch runtime.GOOS {
	case "darwin":
		return newDarwinService(config)
	case "windows":
		return newWindowsService(config)
	case "linux":
		// TODO: Implement systemd support
		return nil, fmt.Errorf("linux support not yet implemented")
	default:
		return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// IsElevated checks if the current process has elevated privileges.
// Returns true if running as root/administrator, false otherwise.
func IsElevated() bool {
	switch runtime.GOOS {
	case "darwin", "linux":
		return isElevatedUnix()
	case "windows":
		return isElevatedWindows()
	default:
		return false
	}
}

// RequireElevated returns an error if the process doesn't have elevated privileges
func RequireElevated() error {
	if !IsElevated() {
		switch runtime.GOOS {
		case "darwin", "linux":
			return fmt.Errorf("this operation requires root privileges, please run with sudo")
		case "windows":
			return fmt.Errorf("this operation requires administrator privileges, please run as administrator")
		default:
			return fmt.Errorf("this operation requires elevated privileges")
		}
	}
	return nil
}
