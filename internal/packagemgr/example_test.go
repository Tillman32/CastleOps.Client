package packagemgr_test

import (
	"context"
	"fmt"
	"time"

	"github.com/castleops/client/internal/api"
	"github.com/castleops/client/internal/packagemgr"
	"github.com/rs/zerolog"
)

// Example_basicUsage demonstrates basic package manager usage
func Example_basicUsage() {
	ctx := context.Background()
	logger := zerolog.New(nil).Level(zerolog.Disabled)

	// Auto-detect and get the default package manager for the platform
	manager, err := packagemgr.GetDefault(ctx, logger)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Using package manager: %s\n", manager.Name())

	// Check if a package is installed
	installed, err := manager.IsPackageInstalled(ctx, "git")
	if err != nil {
		fmt.Printf("Error checking package: %v\n", err)
		return
	}

	if installed {
		fmt.Println("Git is installed")
	} else {
		fmt.Println("Git is not installed")
	}
}

// Example_installerUsage demonstrates using the Installer helper
func Example_installerUsage() {
	ctx := context.Background()
	logger := zerolog.New(nil).Level(zerolog.Disabled)

	// Create an installer with default registry
	installer := packagemgr.NewInstaller(packagemgr.InstallerConfig{
		Logger: logger,
	})

	// Install a package (auto-detects package manager)
	pkg := &packagemgr.Package{
		Name:    "wget",
		Version: "latest",
	}

	err := installer.InstallPackage(ctx, "auto", pkg)
	if err != nil {
		fmt.Printf("Installation failed: %v\n", err)
		return
	}

	fmt.Println("Package installed successfully")

	// List all installed packages
	packages, err := installer.ListInstalledPackages(ctx, "auto")
	if err != nil {
		fmt.Printf("Failed to list packages: %v\n", err)
		return
	}

	fmt.Printf("Found %d installed packages\n", len(packages))
}

// Example_registryUsage demonstrates using the registry pattern
func Example_registryUsage() {
	ctx := context.Background()
	logger := zerolog.New(nil).Level(zerolog.Disabled)

	// Create a registry
	registry := packagemgr.NewRegistry(packagemgr.RegistryConfig{
		Logger: logger,
	})

	// Register Homebrew
	homebrew := packagemgr.NewHomebrew(packagemgr.HomebrewConfig{
		Logger: logger,
	})
	registry.Register(homebrew)

	// Register Chocolatey
	chocolatey := packagemgr.NewChocolatey(packagemgr.ChocolateyConfig{
		Logger: logger,
	})
	registry.Register(chocolatey)

	// Get all registered managers
	managers := registry.List()
	fmt.Printf("Registered managers: %v\n", managers)

	// Auto-detect which manager is installed
	manager, err := registry.AutoDetect(ctx)
	if err != nil {
		fmt.Printf("No package manager detected: %v\n", err)
		return
	}

	fmt.Printf("Detected manager: %s\n", manager.Name())

	// Get installed managers
	installed, err := registry.ListInstalled(ctx)
	if err != nil {
		fmt.Printf("Error listing installed managers: %v\n", err)
		return
	}

	fmt.Printf("Found %d installed package managers\n", len(installed))
}

// Example_commandIntegration demonstrates integrating with the API command handler
func Example_commandIntegration() {
	logger := zerolog.New(nil).Level(zerolog.Disabled)

	// Create a mock API client (in real usage, this would be a real client)
	client := &api.Client{}

	// Create a command handler
	handler := api.NewCommandHandler(api.CommandHandlerConfig{
		Client:                client,
		Logger:                logger,
		MaxConcurrentCommands: 5,
	})

	// Initialize package management and register executors
	err := packagemgr.InitializePackageManagement(handler, logger)
	if err != nil {
		fmt.Printf("Failed to initialize package management: %v\n", err)
		return
	}

	fmt.Println("Package management initialized")

	// Create a sample install command
	cmd := &api.Command{
		CommandID: "cmd-123",
		Type:      api.CommandInstallPackage,
		Payload: map[string]interface{}{
			"package_manager": "auto",
			"package_name":    "curl",
			"version":         "latest",
		},
		Timeout: 300,
	}

	// Handle the command (in real usage, this comes from the server)
	err = handler.HandleCommand(cmd)
	if err != nil {
		fmt.Printf("Failed to handle command: %v\n", err)
		return
	}

	fmt.Println("Command queued for execution")

	// In production, wait for command to complete
	time.Sleep(100 * time.Millisecond)

	// Stop the handler gracefully
	_ = handler.Stop(5 * time.Second)
}

// Example_chocolatey demonstrates Chocolatey-specific usage
func Example_chocolatey() {
	ctx := context.Background()
	logger := zerolog.New(nil).Level(zerolog.Disabled)

	// Create Chocolatey instance
	choco := packagemgr.NewChocolatey(packagemgr.ChocolateyConfig{
		Logger:  logger,
		Timeout: 5 * time.Minute,
	})

	// Check if Chocolatey is installed
	installed, err := choco.IsInstalled(ctx)
	if err != nil {
		fmt.Printf("Error checking installation: %v\n", err)
		return
	}

	if !installed {
		fmt.Println("Chocolatey not installed, installing...")
		err = choco.Install(ctx)
		if err != nil {
			fmt.Printf("Installation failed: %v\n", err)
			return
		}
	}

	fmt.Println("Chocolatey is ready")

	// Install a package with custom options
	pkg := &packagemgr.Package{
		Name:    "googlechrome",
		Version: "latest",
		Options: map[string]string{
			"--force": "", // Force reinstall
		},
	}

	err = choco.InstallPackage(ctx, pkg)
	if err != nil {
		fmt.Printf("Package installation failed: %v\n", err)
		return
	}

	fmt.Println("Package installed")
}

// Example_homebrew demonstrates Homebrew-specific usage
func Example_homebrew() {
	ctx := context.Background()
	logger := zerolog.New(nil).Level(zerolog.Disabled)

	// Create Homebrew instance
	brew := packagemgr.NewHomebrew(packagemgr.HomebrewConfig{
		Logger:  logger,
		Timeout: 5 * time.Minute,
	})

	// Check if Homebrew is installed
	installed, err := brew.IsInstalled(ctx)
	if err != nil {
		fmt.Printf("Error checking installation: %v\n", err)
		return
	}

	if !installed {
		fmt.Println("Homebrew not installed")
		// Note: Installing Homebrew requires user interaction for sudo password
		return
	}

	fmt.Println("Homebrew is ready")

	// Install a cask (GUI application)
	pkg := &packagemgr.Package{
		Name: "visual-studio-code",
		Options: map[string]string{
			"--cask": "", // Install as cask
		},
	}

	err = brew.InstallPackage(ctx, pkg)
	if err != nil {
		fmt.Printf("Package installation failed: %v\n", err)
		return
	}

	fmt.Println("Package installed")

	// Get package info
	pkgInfo, err := brew.GetPackageInfo(ctx, "git")
	if err != nil {
		fmt.Printf("Failed to get package info: %v\n", err)
		return
	}

	fmt.Printf("Package: %s, Version: %s\n", pkgInfo.Name, pkgInfo.Version)
}

// Example_errorHandling demonstrates error handling patterns
func Example_errorHandling() {
	ctx := context.Background()
	logger := zerolog.New(nil).Level(zerolog.Disabled)

	manager, err := packagemgr.GetDefault(ctx, logger)
	if err != nil {
		fmt.Printf("No package manager available: %v\n", err)
		return
	}

	// Try to install a package
	pkg := &packagemgr.Package{
		Name: "nonexistent-package-12345",
	}

	err = manager.InstallPackage(ctx, pkg)
	if err != nil {
		// Check for specific error types
		if pkgErr, ok := err.(*packagemgr.PackageError); ok {
			fmt.Printf("Package error: %s (code: %s)\n", pkgErr.Message, pkgErr.Code)

			// Check for specific error codes
			switch pkgErr.Code {
			case "PACKAGE_NOT_FOUND":
				fmt.Println("Package does not exist in repository")
			case "PERMISSION_DENIED":
				fmt.Println("Requires administrator privileges")
			case "TIMEOUT":
				fmt.Println("Operation timed out")
			default:
				fmt.Printf("Unknown error: %s\n", pkgErr.Code)
			}
		} else {
			fmt.Printf("General error: %v\n", err)
		}
	}
}

// Example_multipleManagers demonstrates using multiple package managers
func Example_multipleManagers() {
	ctx := context.Background()
	logger := zerolog.New(nil).Level(zerolog.Disabled)

	// Create installer
	installer := packagemgr.NewInstaller(packagemgr.InstallerConfig{
		Logger: logger,
	})

	// Get all installed package managers
	managers, err := installer.GetInstalledManagers(ctx)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Found %d installed package managers\n", len(managers))

	for _, mgr := range managers {
		fmt.Printf("- %s\n", mgr.Name())

		// List packages from each manager
		packages, err := mgr.ListInstalled(ctx)
		if err != nil {
			fmt.Printf("  Failed to list packages: %v\n", err)
			continue
		}

		fmt.Printf("  Packages: %d\n", len(packages))
	}
}

// Example_contextCancellation demonstrates proper context cancellation
func Example_contextCancellation() {
	logger := zerolog.New(nil).Level(zerolog.Disabled)

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	manager, err := packagemgr.GetDefault(ctx, logger)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Install a package with the timeout context
	pkg := &packagemgr.Package{
		Name: "wget",
	}

	err = manager.InstallPackage(ctx, pkg)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			fmt.Println("Installation timed out")
		} else {
			fmt.Printf("Installation failed: %v\n", err)
		}
		return
	}

	fmt.Println("Installation completed within timeout")
}
