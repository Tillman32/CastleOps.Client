package packagemgr

import (
	"context"
	"time"
)

// PackageManager defines the interface for cross-platform package management
// Implementations must be thread-safe and support context cancellation
type PackageManager interface {
	// Name returns the package manager identifier (chocolatey, homebrew, etc.)
	Name() string

	// IsInstalled checks if the package manager itself is installed
	IsInstalled(ctx context.Context) (bool, error)

	// Install installs the package manager itself (e.g., bootstrap Chocolatey)
	// Returns nil if already installed
	Install(ctx context.Context) error

	// InstallPackage installs a specific package
	InstallPackage(ctx context.Context, pkg *Package) error

	// UninstallPackage removes a package by name
	UninstallPackage(ctx context.Context, name string) error

	// ListInstalled returns all currently installed packages
	// This can be slow on systems with many packages, use sparingly
	ListInstalled(ctx context.Context) ([]*Package, error)

	// UpdatePackage updates a package to the latest or specified version
	// If name is empty, updates all packages (behavior varies by manager)
	UpdatePackage(ctx context.Context, name string) error

	// IsPackageInstalled checks if a specific package is installed
	// More efficient than ListInstalled for single package checks
	IsPackageInstalled(ctx context.Context, name string) (bool, error)
}

// Package represents a software package with metadata
type Package struct {
	// Name is the package identifier (e.g., "googlechrome", "git")
	Name string

	// Version is the installed or target version
	// Empty or "latest" means latest available
	Version string

	// Description is human-readable package description
	Description string

	// Installed indicates if the package is currently installed
	Installed bool

	// InstalledAt is when the package was installed (if available)
	InstalledAt *time.Time

	// Source is the package source/repository (e.g., "chocolatey", "homebrew")
	Source string

	// Options contains package manager specific options
	// For Chocolatey: --force, --ignore-checksums, etc.
	// For Homebrew: --cask, --HEAD, etc.
	Options map[string]string
}

// InstallOptions configures package installation behavior
type InstallOptions struct {
	// Version specifies the target version (empty/"latest" for newest)
	Version string

	// Force forces installation even if already installed
	Force bool

	// IgnoreErrors continues on non-critical errors
	IgnoreErrors bool

	// NoProgress disables progress output (cleaner logs)
	NoProgress bool

	// Timeout overrides the default operation timeout
	Timeout time.Duration

	// Custom options passed directly to the package manager
	CustomOptions map[string]string
}

// UninstallOptions configures package removal behavior
type UninstallOptions struct {
	// Force forces removal even if dependencies exist
	Force bool

	// RemoveData removes configuration/data files (if supported)
	RemoveData bool

	// Timeout overrides the default operation timeout
	Timeout time.Duration

	// Custom options passed directly to the package manager
	CustomOptions map[string]string
}

// UpdateOptions configures package update behavior
type UpdateOptions struct {
	// Version specifies the target version (empty/"latest" for newest)
	Version string

	// Force forces update even if same version installed
	Force bool

	// Timeout overrides the default operation timeout
	Timeout time.Duration

	// Custom options passed directly to the package manager
	CustomOptions map[string]string
}

// CommandOutput contains the result of a package manager command
type CommandOutput struct {
	// Stdout contains standard output
	Stdout string

	// Stderr contains standard error
	Stderr string

	// ExitCode is the command exit code
	ExitCode int

	// Duration is how long the command took
	Duration time.Duration

	// Success indicates if the command succeeded (exit code 0)
	Success bool
}

// Error types for package management operations
var (
	// ErrNotInstalled indicates the package manager is not installed
	ErrNotInstalled = &PackageError{Code: "NOT_INSTALLED", Message: "package manager not installed"}

	// ErrPackageNotFound indicates the requested package doesn't exist
	ErrPackageNotFound = &PackageError{Code: "PACKAGE_NOT_FOUND", Message: "package not found"}

	// ErrAlreadyInstalled indicates the package is already installed
	ErrAlreadyInstalled = &PackageError{Code: "ALREADY_INSTALLED", Message: "package already installed"}

	// ErrInstallFailed indicates installation failed
	ErrInstallFailed = &PackageError{Code: "INSTALL_FAILED", Message: "installation failed"}

	// ErrUninstallFailed indicates uninstallation failed
	ErrUninstallFailed = &PackageError{Code: "UNINSTALL_FAILED", Message: "uninstallation failed"}

	// ErrUpdateFailed indicates update failed
	ErrUpdateFailed = &PackageError{Code: "UPDATE_FAILED", Message: "update failed"}

	// ErrCommandTimeout indicates the operation timed out
	ErrCommandTimeout = &PackageError{Code: "TIMEOUT", Message: "operation timed out"}

	// ErrPermissionDenied indicates insufficient privileges
	ErrPermissionDenied = &PackageError{Code: "PERMISSION_DENIED", Message: "permission denied, may require admin/root"}
)

// PackageError provides structured error information
type PackageError struct {
	// Code is a machine-readable error code
	Code string

	// Message is a human-readable error message
	Message string

	// Details contains additional error context
	Details map[string]interface{}

	// Wrapped is the underlying error
	Wrapped error
}

// Error implements the error interface
func (e *PackageError) Error() string {
	if e.Wrapped != nil {
		return e.Message + ": " + e.Wrapped.Error()
	}
	return e.Message
}

// Unwrap returns the underlying error
func (e *PackageError) Unwrap() error {
	return e.Wrapped
}

// WithDetails adds additional error context
func (e *PackageError) WithDetails(key string, value interface{}) *PackageError {
	if e.Details == nil {
		e.Details = make(map[string]interface{})
	}
	e.Details[key] = value
	return e
}

// WrapError wraps an error with package error context
func WrapError(err error, code, message string) *PackageError {
	return &PackageError{
		Code:    code,
		Message: message,
		Wrapped: err,
	}
}
