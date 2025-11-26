package packagemgr

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"github.com/rs/zerolog"
)

// Registry manages available package managers and provides factory methods
// It supports automatic detection of the appropriate package manager for the platform
type Registry struct {
	managers map[string]PackageManager
	logger   zerolog.Logger
	mu       sync.RWMutex
}

// RegistryConfig configures the package manager registry
type RegistryConfig struct {
	Logger zerolog.Logger
}

// NewRegistry creates a new package manager registry
func NewRegistry(config RegistryConfig) *Registry {
	return &Registry{
		managers: make(map[string]PackageManager),
		logger:   config.Logger,
	}
}

// Register adds a package manager to the registry
// If a manager with the same name already exists, it will be replaced
func (r *Registry) Register(manager PackageManager) {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := manager.Name()
	r.managers[name] = manager

	r.logger.Debug().
		Str("manager", name).
		Msg("Registered package manager")
}

// Get retrieves a package manager by name
// Returns ErrNotFound if the manager doesn't exist
func (r *Registry) Get(name string) (PackageManager, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	manager, exists := r.managers[name]
	if !exists {
		return nil, fmt.Errorf("package manager '%s' not found in registry", name)
	}

	return manager, nil
}

// GetOrAuto retrieves a package manager by name, or auto-detects if name is "auto" or empty
// This is the primary method for getting a package manager in production code
func (r *Registry) GetOrAuto(ctx context.Context, name string) (PackageManager, error) {
	// If name is "auto" or empty, auto-detect
	if name == "" || name == "auto" {
		return r.AutoDetect(ctx)
	}

	// Otherwise, get by name
	return r.Get(name)
}

// AutoDetect automatically selects the appropriate package manager for the current platform
// It checks which managers are installed and returns the first available one
func (r *Registry) AutoDetect(ctx context.Context) (PackageManager, error) {
	r.logger.Debug().
		Str("os", runtime.GOOS).
		Msg("Auto-detecting package manager")

	// Get platform-specific preferred managers
	preferredManagers := r.getPreferredManagers()

	r.mu.RLock()
	defer r.mu.RUnlock()

	// Try each preferred manager in order
	for _, name := range preferredManagers {
		manager, exists := r.managers[name]
		if !exists {
			r.logger.Debug().
				Str("manager", name).
				Msg("Preferred manager not registered")
			continue
		}

		// Check if installed
		installed, err := manager.IsInstalled(ctx)
		if err != nil {
			r.logger.Warn().
				Err(err).
				Str("manager", name).
				Msg("Failed to check if manager is installed")
			continue
		}

		if installed {
			r.logger.Info().
				Str("manager", name).
				Msg("Auto-detected package manager")
			return manager, nil
		}

		r.logger.Debug().
			Str("manager", name).
			Msg("Manager not installed")
	}

	return nil, fmt.Errorf("no package manager detected for platform: %s", runtime.GOOS)
}

// getPreferredManagers returns the preferred package managers for the current platform
// Ordered by preference (most preferred first)
func (r *Registry) getPreferredManagers() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{"chocolatey", "winget", "scoop"}
	case "darwin":
		return []string{"homebrew", "macports"}
	case "linux":
		// Linux has many package managers, prefer homebrew (linuxbrew) if available
		// Otherwise, would need apt, yum, dnf, pacman, etc.
		return []string{"homebrew", "apt", "yum", "dnf", "pacman", "zypper"}
	default:
		return []string{}
	}
}

// List returns all registered package managers
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.managers))
	for name := range r.managers {
		names = append(names, name)
	}

	return names
}

// ListInstalled returns all registered package managers that are currently installed
func (r *Registry) ListInstalled(ctx context.Context) ([]PackageManager, error) {
	r.mu.RLock()
	managers := make([]PackageManager, 0, len(r.managers))
	for _, manager := range r.managers {
		managers = append(managers, manager)
	}
	r.mu.RUnlock()

	installed := make([]PackageManager, 0)

	for _, manager := range managers {
		isInstalled, err := manager.IsInstalled(ctx)
		if err != nil {
			r.logger.Warn().
				Err(err).
				Str("manager", manager.Name()).
				Msg("Failed to check if manager is installed")
			continue
		}

		if isInstalled {
			installed = append(installed, manager)
		}
	}

	return installed, nil
}

// HasManager checks if a package manager is registered
func (r *Registry) HasManager(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.managers[name]
	return exists
}

// Unregister removes a package manager from the registry
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.managers, name)

	r.logger.Debug().
		Str("manager", name).
		Msg("Unregistered package manager")
}

// Clear removes all package managers from the registry
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.managers = make(map[string]PackageManager)

	r.logger.Debug().Msg("Cleared all package managers from registry")
}

// DefaultRegistry creates a registry with default package managers for the current platform
// This is a convenience function for typical usage
func DefaultRegistry(logger zerolog.Logger) *Registry {
	registry := NewRegistry(RegistryConfig{Logger: logger})

	// Register platform-specific managers
	switch runtime.GOOS {
	case "windows":
		// Register Chocolatey for Windows
		registry.Register(NewChocolatey(ChocolateyConfig{
			Logger: logger.With().Str("manager", "chocolatey").Logger(),
		}))

	case "darwin":
		// Register Homebrew for macOS
		registry.Register(NewHomebrew(HomebrewConfig{
			Logger: logger.With().Str("manager", "homebrew").Logger(),
		}))

	case "linux":
		// Register Homebrew (Linuxbrew) for Linux
		registry.Register(NewHomebrew(HomebrewConfig{
			Logger: logger.With().Str("manager", "homebrew").Logger(),
		}))
		// Additional Linux package managers could be registered here
		// e.g., apt, yum, dnf, etc.
	}

	return registry
}

// GetDefault is a convenience function that creates a default registry and returns the auto-detected manager
func GetDefault(ctx context.Context, logger zerolog.Logger) (PackageManager, error) {
	registry := DefaultRegistry(logger)
	return registry.AutoDetect(ctx)
}

// GetDefaultOrInstall gets the default package manager and installs it if not present
// This is the most convenient method for ensuring a package manager is available
func GetDefaultOrInstall(ctx context.Context, logger zerolog.Logger) (PackageManager, error) {
	registry := DefaultRegistry(logger)

	// Try to auto-detect
	manager, err := registry.AutoDetect(ctx)
	if err == nil {
		return manager, nil
	}

	logger.Info().Msg("No package manager detected, attempting to install default manager")

	// No manager detected, try to install the default for this platform
	preferredManagers := registry.getPreferredManagers()
	if len(preferredManagers) == 0 {
		return nil, fmt.Errorf("no package manager available for platform: %s", runtime.GOOS)
	}

	// Try to get and install the first preferred manager
	preferredName := preferredManagers[0]
	manager, err = registry.Get(preferredName)
	if err != nil {
		return nil, fmt.Errorf("failed to get preferred manager '%s': %w", preferredName, err)
	}

	// Install the manager
	logger.Info().
		Str("manager", preferredName).
		Msg("Installing package manager")

	if err := manager.Install(ctx); err != nil {
		return nil, fmt.Errorf("failed to install package manager '%s': %w", preferredName, err)
	}

	// Verify installation
	installed, err := manager.IsInstalled(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to verify installation of '%s': %w", preferredName, err)
	}
	if !installed {
		return nil, fmt.Errorf("package manager '%s' not detected after installation", preferredName)
	}

	logger.Info().
		Str("manager", preferredName).
		Msg("Package manager installed successfully")

	return manager, nil
}

// ManagerInfo provides information about a package manager
type ManagerInfo struct {
	Name      string
	Installed bool
	Version   string
	Error     string
}

// GetAllManagerInfo returns information about all registered package managers
// This is useful for diagnostics and UI display
func (r *Registry) GetAllManagerInfo(ctx context.Context) []ManagerInfo {
	r.mu.RLock()
	managers := make([]PackageManager, 0, len(r.managers))
	for _, manager := range r.managers {
		managers = append(managers, manager)
	}
	r.mu.RUnlock()

	info := make([]ManagerInfo, 0, len(managers))

	for _, manager := range managers {
		managerInfo := ManagerInfo{
			Name: manager.Name(),
		}

		installed, err := manager.IsInstalled(ctx)
		if err != nil {
			managerInfo.Error = err.Error()
		} else {
			managerInfo.Installed = installed
		}

		info = append(info, managerInfo)
	}

	return info
}
