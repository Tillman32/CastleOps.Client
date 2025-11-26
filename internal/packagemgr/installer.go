package packagemgr

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
)

// Installer provides high-level package installation orchestration
// It handles package manager detection, installation, and verification
type Installer struct {
	registry *Registry
	logger   zerolog.Logger
}

// InstallerConfig configures the installer
type InstallerConfig struct {
	Registry *Registry
	Logger   zerolog.Logger
}

// NewInstaller creates a new installer instance
func NewInstaller(config InstallerConfig) *Installer {
	if config.Registry == nil {
		// Create default registry if none provided
		config.Registry = DefaultRegistry(config.Logger)
	}

	return &Installer{
		registry: config.Registry,
		logger:   config.Logger,
	}
}

// EnsurePackageManager ensures a package manager is available
// If managerName is "auto" or empty, it auto-detects and installs if needed
// Otherwise, it ensures the specified manager is installed
func (i *Installer) EnsurePackageManager(ctx context.Context, managerName string) (PackageManager, error) {
	i.logger.Debug().
		Str("manager", managerName).
		Msg("Ensuring package manager is available")

	// If auto or empty, use auto-detect with fallback to install
	if managerName == "" || managerName == "auto" {
		return i.ensureAutoDetected(ctx)
	}

	// Get the specific manager
	manager, err := i.registry.Get(managerName)
	if err != nil {
		return nil, fmt.Errorf("package manager '%s' not found: %w", managerName, err)
	}

	// Check if installed
	installed, err := manager.IsInstalled(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check if '%s' is installed: %w", managerName, err)
	}

	if installed {
		i.logger.Debug().
			Str("manager", managerName).
			Msg("Package manager already installed")
		return manager, nil
	}

	// Install the manager
	i.logger.Info().
		Str("manager", managerName).
		Msg("Installing package manager")

	if err := manager.Install(ctx); err != nil {
		return nil, fmt.Errorf("failed to install package manager '%s': %w", managerName, err)
	}

	// Verify installation
	installed, err = manager.IsInstalled(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to verify installation of '%s': %w", managerName, err)
	}

	if !installed {
		return nil, fmt.Errorf("package manager '%s' not detected after installation", managerName)
	}

	i.logger.Info().
		Str("manager", managerName).
		Msg("Package manager installed and verified")

	return manager, nil
}

// ensureAutoDetected auto-detects a package manager and installs if none are available
func (i *Installer) ensureAutoDetected(ctx context.Context) (PackageManager, error) {
	// Try to auto-detect
	manager, err := i.registry.AutoDetect(ctx)
	if err == nil {
		i.logger.Debug().
			Str("manager", manager.Name()).
			Msg("Package manager auto-detected")
		return manager, nil
	}

	i.logger.Info().Msg("No package manager detected, attempting to install default")

	// No manager detected, get default for platform
	preferredManagers := i.registry.getPreferredManagers()
	if len(preferredManagers) == 0 {
		return nil, fmt.Errorf("no package manager available for this platform")
	}

	// Try to install the first preferred manager
	var lastErr error
	for _, preferredName := range preferredManagers {
		manager, err := i.registry.Get(preferredName)
		if err != nil {
			i.logger.Debug().
				Err(err).
				Str("manager", preferredName).
				Msg("Preferred manager not registered, skipping")
			continue
		}

		i.logger.Info().
			Str("manager", preferredName).
			Msg("Attempting to install package manager")

		if err := manager.Install(ctx); err != nil {
			i.logger.Warn().
				Err(err).
				Str("manager", preferredName).
				Msg("Failed to install package manager")
			lastErr = err
			continue
		}

		// Verify installation
		installed, err := manager.IsInstalled(ctx)
		if err != nil {
			i.logger.Warn().
				Err(err).
				Str("manager", preferredName).
				Msg("Failed to verify installation")
			lastErr = err
			continue
		}

		if !installed {
			err := fmt.Errorf("not detected after installation")
			i.logger.Warn().
				Err(err).
				Str("manager", preferredName).
				Msg("Installation verification failed")
			lastErr = err
			continue
		}

		// Success!
		i.logger.Info().
			Str("manager", preferredName).
			Msg("Package manager installed successfully")
		return manager, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("failed to install any package manager: %w", lastErr)
	}

	return nil, fmt.Errorf("no package manager could be installed")
}

// InstallPackage installs a package using the specified or auto-detected manager
func (i *Installer) InstallPackage(ctx context.Context, managerName string, pkg *Package) error {
	if pkg == nil || pkg.Name == "" {
		return fmt.Errorf("invalid package: name is required")
	}

	// Ensure package manager is available
	manager, err := i.EnsurePackageManager(ctx, managerName)
	if err != nil {
		return fmt.Errorf("failed to ensure package manager: %w", err)
	}

	i.logger.Info().
		Str("manager", manager.Name()).
		Str("package", pkg.Name).
		Msg("Installing package")

	// Install the package
	startTime := time.Now()
	err = manager.InstallPackage(ctx, pkg)
	duration := time.Since(startTime)

	if err != nil {
		i.logger.Error().
			Err(err).
			Str("manager", manager.Name()).
			Str("package", pkg.Name).
			Dur("duration", duration).
			Msg("Package installation failed")
		return err
	}

	i.logger.Info().
		Str("manager", manager.Name()).
		Str("package", pkg.Name).
		Dur("duration", duration).
		Msg("Package installed successfully")

	return nil
}

// UninstallPackage removes a package using the specified or auto-detected manager
func (i *Installer) UninstallPackage(ctx context.Context, managerName string, packageName string) error {
	if packageName == "" {
		return fmt.Errorf("package name is required")
	}

	// Get package manager (don't install if not present)
	var manager PackageManager
	var err error

	if managerName == "" || managerName == "auto" {
		manager, err = i.registry.AutoDetect(ctx)
	} else {
		manager, err = i.registry.Get(managerName)
	}

	if err != nil {
		return fmt.Errorf("failed to get package manager: %w", err)
	}

	// Verify manager is installed
	installed, err := manager.IsInstalled(ctx)
	if err != nil {
		return fmt.Errorf("failed to check if manager is installed: %w", err)
	}
	if !installed {
		return ErrNotInstalled
	}

	i.logger.Info().
		Str("manager", manager.Name()).
		Str("package", packageName).
		Msg("Uninstalling package")

	// Uninstall the package
	startTime := time.Now()
	err = manager.UninstallPackage(ctx, packageName)
	duration := time.Since(startTime)

	if err != nil {
		i.logger.Error().
			Err(err).
			Str("manager", manager.Name()).
			Str("package", packageName).
			Dur("duration", duration).
			Msg("Package uninstallation failed")
		return err
	}

	i.logger.Info().
		Str("manager", manager.Name()).
		Str("package", packageName).
		Dur("duration", duration).
		Msg("Package uninstalled successfully")

	return nil
}

// UpdatePackage updates a package using the specified or auto-detected manager
func (i *Installer) UpdatePackage(ctx context.Context, managerName string, packageName string) error {
	if packageName == "" {
		return fmt.Errorf("package name is required")
	}

	// Get package manager (don't install if not present)
	var manager PackageManager
	var err error

	if managerName == "" || managerName == "auto" {
		manager, err = i.registry.AutoDetect(ctx)
	} else {
		manager, err = i.registry.Get(managerName)
	}

	if err != nil {
		return fmt.Errorf("failed to get package manager: %w", err)
	}

	// Verify manager is installed
	installed, err := manager.IsInstalled(ctx)
	if err != nil {
		return fmt.Errorf("failed to check if manager is installed: %w", err)
	}
	if !installed {
		return ErrNotInstalled
	}

	i.logger.Info().
		Str("manager", manager.Name()).
		Str("package", packageName).
		Msg("Updating package")

	// Update the package
	startTime := time.Now()
	err = manager.UpdatePackage(ctx, packageName)
	duration := time.Since(startTime)

	if err != nil {
		i.logger.Error().
			Err(err).
			Str("manager", manager.Name()).
			Str("package", packageName).
			Dur("duration", duration).
			Msg("Package update failed")
		return err
	}

	i.logger.Info().
		Str("manager", manager.Name()).
		Str("package", packageName).
		Dur("duration", duration).
		Msg("Package updated successfully")

	return nil
}

// ListInstalledPackages lists all installed packages for the specified or auto-detected manager
func (i *Installer) ListInstalledPackages(ctx context.Context, managerName string) ([]*Package, error) {
	// Get package manager
	var manager PackageManager
	var err error

	if managerName == "" || managerName == "auto" {
		manager, err = i.registry.AutoDetect(ctx)
	} else {
		manager, err = i.registry.Get(managerName)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get package manager: %w", err)
	}

	// Verify manager is installed
	installed, err := manager.IsInstalled(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check if manager is installed: %w", err)
	}
	if !installed {
		return nil, ErrNotInstalled
	}

	i.logger.Debug().
		Str("manager", manager.Name()).
		Msg("Listing installed packages")

	// List packages
	packages, err := manager.ListInstalled(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list installed packages: %w", err)
	}

	i.logger.Debug().
		Str("manager", manager.Name()).
		Int("count", len(packages)).
		Msg("Listed installed packages")

	return packages, nil
}

// IsPackageInstalled checks if a package is installed using the specified or auto-detected manager
func (i *Installer) IsPackageInstalled(ctx context.Context, managerName string, packageName string) (bool, error) {
	if packageName == "" {
		return false, fmt.Errorf("package name is required")
	}

	// Get package manager
	var manager PackageManager
	var err error

	if managerName == "" || managerName == "auto" {
		manager, err = i.registry.AutoDetect(ctx)
	} else {
		manager, err = i.registry.Get(managerName)
	}

	if err != nil {
		return false, fmt.Errorf("failed to get package manager: %w", err)
	}

	// Verify manager is installed
	installed, err := manager.IsInstalled(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to check if manager is installed: %w", err)
	}
	if !installed {
		return false, ErrNotInstalled
	}

	// Check if package is installed
	return manager.IsPackageInstalled(ctx, packageName)
}

// GetInstalledManagers returns all package managers that are currently installed
func (i *Installer) GetInstalledManagers(ctx context.Context) ([]PackageManager, error) {
	return i.registry.ListInstalled(ctx)
}

// GetManagerInfo returns information about all registered managers
func (i *Installer) GetManagerInfo(ctx context.Context) []ManagerInfo {
	return i.registry.GetAllManagerInfo(ctx)
}
