package packagemgr

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/castleops/client/internal/api"
	"github.com/rs/zerolog"
)

// CommandExecutorFactory creates command executors for package management operations
// This integrates the package manager with the API command handler
type CommandExecutorFactory struct {
	installer *Installer
	logger    zerolog.Logger
}

// CommandExecutorConfig configures the command executor factory
type CommandExecutorConfig struct {
	Installer *Installer
	Logger    zerolog.Logger
}

// NewCommandExecutorFactory creates a new command executor factory
func NewCommandExecutorFactory(config CommandExecutorConfig) *CommandExecutorFactory {
	return &CommandExecutorFactory{
		installer: config.Installer,
		logger:    config.Logger,
	}
}

// RegisterExecutors registers all package management executors with the command handler
// This should be called during agent initialization
func (f *CommandExecutorFactory) RegisterExecutors(handler *api.CommandHandler) {
	// Register install package executor
	handler.RegisterExecutor(
		api.CommandInstallPackage,
		f.createInstallPackageExecutor(),
	)

	// Register uninstall package executor
	handler.RegisterExecutor(
		api.CommandUninstallPackage,
		f.createUninstallPackageExecutor(),
	)

	// Register update package executor
	handler.RegisterExecutor(
		api.CommandUpdatePackage,
		f.createUpdatePackageExecutor(),
	)

	f.logger.Info().Msg("Registered package management command executors")
}

// createInstallPackageExecutor creates an executor for package installation commands
func (f *CommandExecutorFactory) createInstallPackageExecutor() api.CommandExecutor {
	return func(ctx context.Context, payload interface{}) (*api.CommandResult, error) {
		startTime := time.Now()

		// Parse payload
		var installPayload api.InstallPackagePayload
		if err := parsePayload(payload, &installPayload); err != nil {
			return &api.CommandResult{
				Status:        api.CommandStatusFailed,
				Error:         fmt.Sprintf("Invalid install package payload: %v", err),
				ExecutionTime: time.Since(startTime).Milliseconds(),
			}, nil
		}

		// Validate payload
		if installPayload.PackageName == "" {
			return &api.CommandResult{
				Status:        api.CommandStatusFailed,
				Error:         "Package name is required",
				ExecutionTime: time.Since(startTime).Milliseconds(),
			}, nil
		}

		f.logger.Info().
			Str("package", installPayload.PackageName).
			Str("version", installPayload.Version).
			Str("manager", installPayload.PackageManager).
			Msg("Processing install package command")

		// Create package object
		pkg := &Package{
			Name:    installPayload.PackageName,
			Version: installPayload.Version,
			Options: installPayload.Options,
		}

		// Execute installation
		err := f.installer.InstallPackage(ctx, installPayload.PackageManager, pkg)

		result := &api.CommandResult{
			ExecutionTime: time.Since(startTime).Milliseconds(),
		}

		if err != nil {
			result.Status = api.CommandStatusFailed
			result.Error = err.Error()
			result.Output = fmt.Sprintf("Failed to install package '%s': %v", installPayload.PackageName, err)

			f.logger.Error().
				Err(err).
				Str("package", installPayload.PackageName).
				Msg("Package installation failed")
		} else {
			result.Status = api.CommandStatusSuccess
			result.Output = fmt.Sprintf("Successfully installed package '%s'", installPayload.PackageName)

			f.logger.Info().
				Str("package", installPayload.PackageName).
				Int64("duration_ms", result.ExecutionTime).
				Msg("Package installed successfully")
		}

		return result, nil
	}
}

// createUninstallPackageExecutor creates an executor for package uninstallation commands
func (f *CommandExecutorFactory) createUninstallPackageExecutor() api.CommandExecutor {
	return func(ctx context.Context, payload interface{}) (*api.CommandResult, error) {
		startTime := time.Now()

		// Parse payload
		var uninstallPayload api.UninstallPackagePayload
		if err := parsePayload(payload, &uninstallPayload); err != nil {
			return &api.CommandResult{
				Status:        api.CommandStatusFailed,
				Error:         fmt.Sprintf("Invalid uninstall package payload: %v", err),
				ExecutionTime: time.Since(startTime).Milliseconds(),
			}, nil
		}

		// Validate payload
		if uninstallPayload.PackageName == "" {
			return &api.CommandResult{
				Status:        api.CommandStatusFailed,
				Error:         "Package name is required",
				ExecutionTime: time.Since(startTime).Milliseconds(),
			}, nil
		}

		f.logger.Info().
			Str("package", uninstallPayload.PackageName).
			Str("manager", uninstallPayload.PackageManager).
			Bool("force", uninstallPayload.Force).
			Msg("Processing uninstall package command")

		// Execute uninstallation
		err := f.installer.UninstallPackage(ctx, uninstallPayload.PackageManager, uninstallPayload.PackageName)

		result := &api.CommandResult{
			ExecutionTime: time.Since(startTime).Milliseconds(),
		}

		if err != nil {
			result.Status = api.CommandStatusFailed
			result.Error = err.Error()
			result.Output = fmt.Sprintf("Failed to uninstall package '%s': %v", uninstallPayload.PackageName, err)

			f.logger.Error().
				Err(err).
				Str("package", uninstallPayload.PackageName).
				Msg("Package uninstallation failed")
		} else {
			result.Status = api.CommandStatusSuccess
			result.Output = fmt.Sprintf("Successfully uninstalled package '%s'", uninstallPayload.PackageName)

			f.logger.Info().
				Str("package", uninstallPayload.PackageName).
				Int64("duration_ms", result.ExecutionTime).
				Msg("Package uninstalled successfully")
		}

		return result, nil
	}
}

// createUpdatePackageExecutor creates an executor for package update commands
func (f *CommandExecutorFactory) createUpdatePackageExecutor() api.CommandExecutor {
	return func(ctx context.Context, payload interface{}) (*api.CommandResult, error) {
		startTime := time.Now()

		// Parse payload
		var updatePayload api.UpdatePackagePayload
		if err := parsePayload(payload, &updatePayload); err != nil {
			return &api.CommandResult{
				Status:        api.CommandStatusFailed,
				Error:         fmt.Sprintf("Invalid update package payload: %v", err),
				ExecutionTime: time.Since(startTime).Milliseconds(),
			}, nil
		}

		// Validate payload
		if updatePayload.PackageName == "" {
			return &api.CommandResult{
				Status:        api.CommandStatusFailed,
				Error:         "Package name is required",
				ExecutionTime: time.Since(startTime).Milliseconds(),
			}, nil
		}

		f.logger.Info().
			Str("package", updatePayload.PackageName).
			Str("version", updatePayload.Version).
			Str("manager", updatePayload.PackageManager).
			Msg("Processing update package command")

		// Execute update
		err := f.installer.UpdatePackage(ctx, updatePayload.PackageManager, updatePayload.PackageName)

		result := &api.CommandResult{
			ExecutionTime: time.Since(startTime).Milliseconds(),
		}

		if err != nil {
			result.Status = api.CommandStatusFailed
			result.Error = err.Error()
			result.Output = fmt.Sprintf("Failed to update package '%s': %v", updatePayload.PackageName, err)

			f.logger.Error().
				Err(err).
				Str("package", updatePayload.PackageName).
				Msg("Package update failed")
		} else {
			result.Status = api.CommandStatusSuccess
			result.Output = fmt.Sprintf("Successfully updated package '%s'", updatePayload.PackageName)

			f.logger.Info().
				Str("package", updatePayload.PackageName).
				Int64("duration_ms", result.ExecutionTime).
				Msg("Package updated successfully")
		}

		return result, nil
	}
}

// parsePayload converts an interface{} payload to a specific type
func parsePayload(payload interface{}, target interface{}) error {
	// Re-marshal and unmarshal to convert interface{} to specific type
	bytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	if err := json.Unmarshal(bytes, target); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	return nil
}

// InitializePackageManagement sets up package management for the agent
// This is a convenience function to be called during agent initialization
func InitializePackageManagement(handler *api.CommandHandler, logger zerolog.Logger) error {
	logger.Info().Msg("Initializing package management subsystem")

	// Create default registry with platform-specific managers
	registry := DefaultRegistry(logger.With().Str("component", "packagemgr").Logger())

	// Create installer
	installer := NewInstaller(InstallerConfig{
		Registry: registry,
		Logger:   logger.With().Str("component", "installer").Logger(),
	})

	// Create executor factory
	factory := NewCommandExecutorFactory(CommandExecutorConfig{
		Installer: installer,
		Logger:    logger.With().Str("component", "executor").Logger(),
	})

	// Register executors with command handler
	factory.RegisterExecutors(handler)

	logger.Info().Msg("Package management subsystem initialized successfully")

	return nil
}

// GetManagerStatus returns the current status of all package managers
// This can be used for diagnostics and health checks
func GetManagerStatus(ctx context.Context, logger zerolog.Logger) ([]ManagerInfo, error) {
	registry := DefaultRegistry(logger)
	return registry.GetAllManagerInfo(ctx), nil
}

// EnsureManagerInstalled ensures a package manager is installed
// This can be called proactively during agent startup
func EnsureManagerInstalled(ctx context.Context, managerName string, logger zerolog.Logger) error {
	registry := DefaultRegistry(logger)
	installer := NewInstaller(InstallerConfig{
		Registry: registry,
		Logger:   logger,
	})

	_, err := installer.EnsurePackageManager(ctx, managerName)
	return err
}
