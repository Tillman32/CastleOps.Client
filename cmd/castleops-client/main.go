package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/castleops/client/internal/agent"
	"github.com/castleops/client/internal/config"
	"github.com/castleops/client/internal/service"
	"github.com/rs/zerolog"
)

const (
	appName    = "castleops-client"
	appVersion = "0.1.0"
)

var (
	// Command-line flags
	configPath    = flag.String("config", "", "Path to configuration file")
	versionFlag   = flag.Bool("version", false, "Print version and exit")
	installFlag   = flag.Bool("install", false, "Install as system service")
	uninstallFlag = flag.Bool("uninstall", false, "Uninstall system service")
	serviceFlag   = flag.Bool("service", false, "Run as service (used by service manager)")
)

func main() {
	// Parse command-line flags
	flag.Parse()

	// Handle version flag
	if *versionFlag {
		fmt.Printf("%s version %s (%s/%s)\n", appName, appVersion, runtime.GOOS, runtime.GOARCH)
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	logger := initLogger(cfg)
	logger.Info().
		Str("version", appVersion).
		Str("platform", runtime.GOOS).
		Str("arch", runtime.GOARCH).
		Msg("Starting CastleOps Client")

	// Handle service installation/uninstallation
	if *installFlag {
		if err := installService(cfg, logger); err != nil {
			logger.Fatal().Err(err).Msg("Failed to install service")
		}
		logger.Info().Msg("Service installed successfully")
		return
	}

	if *uninstallFlag {
		if err := uninstallService(cfg, logger); err != nil {
			logger.Fatal().Err(err).Msg("Failed to uninstall service")
		}
		logger.Info().Msg("Service uninstalled successfully")
		return
	}

	// Check if running as service
	if *serviceFlag || shouldRunAsService() {
		// Run in service mode
		if err := runAsService(cfg, logger); err != nil {
			logger.Fatal().Err(err).Msg("Service execution failed")
		}
	} else {
		// Run in foreground mode
		if err := run(cfg, logger); err != nil {
			logger.Fatal().Err(err).Msg("Agent failed")
		}
	}
}

// run starts the main agent with graceful shutdown handling
func run(cfg *config.Config, logger zerolog.Logger) error {
	// Create context with cancellation for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	// Create error channel for agent errors
	errChan := make(chan error, 1)

	// Initialize the agent orchestrator
	logger.Info().Msg("Initializing agent")
	agentInstance, err := agent.NewAgent(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	// Start the agent in a goroutine
	go func() {
		if err := agentInstance.Start(ctx); err != nil {
			logger.Error().Err(err).Msg("Agent startup failed")
			errChan <- err
			return
		}

		// Wait for agent to complete
		agentInstance.WaitForShutdown()
		logger.Debug().Msg("Agent shutdown complete")
	}()

	logger.Info().Msg("Agent started successfully")
	logger.Info().
		Dur("heartbeat_interval", cfg.Heartbeat.Interval).
		Dur("metrics_interval", cfg.Metrics.CollectionInterval).
		Str("cache_type", cfg.Cache.Type).
		Msg("Configuration loaded")

	// Wait for shutdown signal or error
	select {
	case sig := <-sigChan:
		logger.Info().Str("signal", sig.String()).Msg("Received shutdown signal")
		cancel()

		// Give components time to shut down gracefully
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		// Initiate graceful shutdown of the agent
		logger.Info().Msg("Graceful shutdown initiated")
		if err := agentInstance.Stop(shutdownCtx); err != nil {
			logger.Error().Err(err).Msg("Error during agent shutdown")
		}

		// Wait for shutdown to complete or timeout
		shutdownComplete := make(chan struct{})
		go func() {
			agentInstance.WaitForShutdown()
			close(shutdownComplete)
		}()

		select {
		case <-shutdownComplete:
			logger.Info().Msg("Agent shutdown completed successfully")
		case <-shutdownCtx.Done():
			logger.Warn().Msg("Graceful shutdown timeout exceeded")
		}

	case err := <-errChan:
		logger.Error().Err(err).Msg("Agent error")
		cancel()

		// Attempt cleanup even after error
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		agentInstance.Stop(shutdownCtx)

		return err
	}

	logger.Info().Msg("Agent stopped")
	return nil
}

// initLogger configures and returns a zerolog logger based on configuration
func initLogger(cfg *config.Config) zerolog.Logger {
	// Parse log level
	level, err := zerolog.ParseLevel(cfg.Logging.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	// Configure output format
	var logger zerolog.Logger
	if cfg.Logging.Format == "console" {
		// Human-readable console output
		output := zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
			NoColor:    false,
		}
		logger = zerolog.New(output).With().Timestamp().Logger()
	} else {
		// JSON output (default)
		logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
	}

	// TODO: Add file output if cfg.Logging.Path is set
	// This will require proper file rotation and error handling

	return logger
}

// installService installs the application as a system service
func installService(cfg *config.Config, logger zerolog.Logger) error {
	logger.Info().Msg("Installing service")

	// Check for elevated privileges
	if err := service.RequireElevated(); err != nil {
		return err
	}

	// Get executable path
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Resolve symlinks
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	// Get config file path
	configPath := *configPath
	if configPath == "" {
		configPath = getDefaultConfigPath()
	}

	// Make config path absolute
	if !filepath.IsAbs(configPath) {
		configPath, err = filepath.Abs(configPath)
		if err != nil {
			return fmt.Errorf("failed to get absolute config path: %w", err)
		}
	}

	// Create service configuration
	svcConfig := &service.Config{
		Name:             "com.castleops.client",
		DisplayName:      "CastleOps Client",
		Description:      "CastleOps desktop agent for system monitoring and management",
		Executable:       exePath,
		Arguments:        []string{"-config", configPath, "-service"},
		WorkingDirectory: filepath.Dir(exePath),
		UserService:      false, // System-level service by default
		Logger:           logger,
	}

	// Create service instance
	svc, err := service.New(svcConfig)
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}

	// Install the service
	if err := svc.Install(); err != nil {
		return fmt.Errorf("failed to install service: %w", err)
	}

	logger.Info().
		Str("name", svc.Name()).
		Str("executable", exePath).
		Str("config", configPath).
		Msg("Service installed successfully")

	return nil
}

// uninstallService removes the application system service
func uninstallService(cfg *config.Config, logger zerolog.Logger) error {
	logger.Info().Msg("Uninstalling service")

	// Check for elevated privileges
	if err := service.RequireElevated(); err != nil {
		return err
	}

	// Create service configuration (minimal config for uninstall)
	svcConfig := &service.Config{
		Name:        "com.castleops.client",
		DisplayName: "CastleOps Client",
		Description: "CastleOps desktop agent",
		Executable:  "/tmp/dummy", // Not used for uninstall
		Logger:      logger,
	}

	// Create service instance
	svc, err := service.New(svcConfig)
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}

	// Uninstall the service
	if err := svc.Uninstall(); err != nil {
		return fmt.Errorf("failed to uninstall service: %w", err)
	}

	logger.Info().Str("name", svc.Name()).Msg("Service uninstalled successfully")

	return nil
}

// runAsService runs the application in service mode
func runAsService(cfg *config.Config, logger zerolog.Logger) error {
	logger.Info().Msg("Running in service mode")

	// Create service configuration
	svcConfig := &service.Config{
		Name:        "com.castleops.client",
		DisplayName: "CastleOps Client",
		Description: "CastleOps desktop agent",
		Executable:  "/tmp/dummy", // Not used in run mode
		Logger:      logger,
	}

	// Create service instance
	svc, err := service.New(svcConfig)
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}

	// Define the service worker function
	worker := func(ctx context.Context) error {
		return runAgent(ctx, cfg, logger)
	}

	// Run the service (blocking call)
	return svc.Run(worker)
}

// runAgent runs the agent with the provided context
func runAgent(ctx context.Context, cfg *config.Config, logger zerolog.Logger) error {
	logger.Info().Msg("Initializing agent")

	// Initialize the agent orchestrator
	agentInstance, err := agent.NewAgent(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	// Start the agent
	if err := agentInstance.Start(ctx); err != nil {
		return fmt.Errorf("failed to start agent: %w", err)
	}

	logger.Info().Msg("Agent started successfully in service mode")

	// Wait for context cancellation (triggered by service manager)
	<-ctx.Done()

	logger.Info().Msg("Service stop requested, shutting down agent")

	// Create shutdown context with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop the agent
	if err := agentInstance.Stop(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("Error during agent shutdown")
		return err
	}

	// Wait for complete shutdown
	agentInstance.WaitForShutdown()

	logger.Info().Msg("Agent shutdown complete")
	return nil
}

// shouldRunAsService detects if the application is running as a service
func shouldRunAsService() bool {
	// On Windows, detect if running as a Windows Service
	if runtime.GOOS == "windows" {
		return isWindowsService()
	}
	return false
}

// getDefaultConfigPath returns the default configuration file path
func getDefaultConfigPath() string {
	switch runtime.GOOS {
	case "darwin":
		homeDir, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(homeDir, "Library", "Application Support", "CastleOps", "config.yaml")
		}
		return "/Library/Application Support/CastleOps/config.yaml"
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "CastleOps", "config.yaml")
	default:
		return "/etc/castleops/config.yaml"
	}
}
