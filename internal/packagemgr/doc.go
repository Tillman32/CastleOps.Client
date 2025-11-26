// Package packagemgr provides a cross-platform abstraction layer for software package management.
//
// It supports multiple package managers including Chocolatey (Windows) and Homebrew (macOS/Linux),
// with an extensible design that allows easy addition of new package managers.
//
// # Architecture
//
// The package is built around several key components:
//
//   - PackageManager interface: Defines the contract for all package managers
//   - Registry: Manages available package managers and provides factory methods
//   - Installer: High-level orchestration layer for package operations
//   - Command Executors: Integration with the API command handler
//
// # Performance
//
// The implementation is optimized for minimal resource usage:
//
//   - Zero-allocation string building where possible
//   - Pre-allocated buffers for command output
//   - Context-based cancellation for responsive timeouts
//   - Thread-safe concurrent operations
//
// # Security
//
// Security is a primary concern:
//
//   - Command injection prevention via proper argument escaping
//   - Input validation on all package names and versions
//   - Privilege escalation awareness
//   - No shell interpretation for user-provided input
//
// # Usage
//
// Basic usage with auto-detection:
//
//	ctx := context.Background()
//	logger := zerolog.New(os.Stderr)
//
//	// Auto-detect and get default package manager
//	manager, err := packagemgr.GetDefault(ctx, logger)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Install a package
//	pkg := &packagemgr.Package{
//	    Name:    "git",
//	    Version: "latest",
//	}
//
//	err = manager.InstallPackage(ctx, pkg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// Using the Installer helper:
//
//	installer := packagemgr.NewInstaller(packagemgr.InstallerConfig{
//	    Logger: logger,
//	})
//
//	pkg := &packagemgr.Package{
//	    Name:    "wget",
//	    Version: "latest",
//	}
//
//	// Automatically installs package manager if needed
//	err := installer.InstallPackage(ctx, "auto", pkg)
//
// Integration with API command handler:
//
//	handler := api.NewCommandHandler(config)
//	err := packagemgr.InitializePackageManagement(handler, logger)
//	// Package commands from server are now automatically handled
//
// # Extensibility
//
// Adding a new package manager is straightforward:
//
//	type MyPackageManager struct {
//	    logger zerolog.Logger
//	}
//
//	func (m *MyPackageManager) Name() string {
//	    return "mypm"
//	}
//
//	// Implement other PackageManager interface methods...
//
//	// Register with registry
//	registry.Register(NewMyPackageManager(config))
//
// # Error Handling
//
// The package provides structured errors via PackageError:
//
//	err := manager.InstallPackage(ctx, pkg)
//	if err != nil {
//	    if pkgErr, ok := err.(*packagemgr.PackageError); ok {
//	        switch pkgErr.Code {
//	        case "PACKAGE_NOT_FOUND":
//	            // Handle missing package
//	        case "PERMISSION_DENIED":
//	            // Handle privilege issues
//	        case "TIMEOUT":
//	            // Handle timeout
//	        }
//	    }
//	}
//
// # Platform Support
//
// Windows (Chocolatey):
//   - Requires PowerShell
//   - Automatic bootstrap if not installed
//   - Administrator privileges required for installation
//
// macOS (Homebrew):
//   - Auto-detects architecture (Intel/Apple Silicon)
//   - May require user interaction for sudo password during installation
//   - Supports both formulae and casks
//
// Linux (Homebrew/Linuxbrew):
//   - Uses Linuxbrew port
//   - Extensible to apt, yum, dnf, pacman, etc.
//
// For more details, see the README.md file in this directory.
package packagemgr
