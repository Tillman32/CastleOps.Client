// Package service provides Windows Service integration.
// This file implements the Service interface using Windows Service Control Manager.

//go:build windows

package service

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	// Default service start timeout
	serviceStartTimeout = 30 * time.Second

	// Default service stop timeout
	serviceStopTimeout = 30 * time.Second
)

// windowsService implements the Service interface for Windows
type windowsService struct {
	config      *Config
	logger      zerolog.Logger
	serviceName string
	eventLog    *eventlog.Log
}

// newWindowsService creates a new Windows service instance
func newWindowsService(config *Config) (Service, error) {
	ws := &windowsService{
		config:      config,
		logger:      config.Logger.With().Str("service", "windows").Logger(),
		serviceName: config.Name,
	}

	// Try to open event log (may not exist if not installed)
	elog, err := eventlog.Open(config.Name)
	if err == nil {
		ws.eventLog = elog
	}

	return ws, nil
}

// Install installs the Windows service
func (s *windowsService) Install() error {
	s.logger.Info().Str("name", s.serviceName).Msg("Installing Windows service")

	// Open service manager
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	// Check if service already exists
	winSvc, err := m.OpenService(s.serviceName)
	if err == nil {
		winSvc.Close()
		return fmt.Errorf("service %s already exists", s.serviceName)
	}

	// Prepare service configuration
	exePath := s.config.Executable
	if !filepath.IsAbs(exePath) {
		exePath, err = filepath.Abs(exePath)
		if err != nil {
			return fmt.Errorf("failed to get absolute path: %w", err)
		}
	}

	// Build service command with arguments
	args := s.config.Arguments
	serviceConfig := mgr.Config{
		DisplayName:      s.config.DisplayName,
		Description:      s.config.Description,
		StartType:        mgr.StartAutomatic,
		ServiceStartName: "NT AUTHORITY\\LocalService",
		Dependencies:     s.config.Dependencies,
	}

	// Create the service
	winSvc, err = m.CreateService(s.serviceName, exePath, serviceConfig, args...)
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}
	defer winSvc.Close()

	// Configure service recovery actions (restart on failure)
	recoveryActions := []mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 60 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 60 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 60 * time.Second},
	}

	if err := winSvc.SetRecoveryActions(recoveryActions, 86400); err != nil {
		s.logger.Warn().Err(err).Msg("Failed to set recovery actions")
		// Not fatal, continue
	}

	// Set up event log
	if err := eventlog.InstallAsEventCreate(s.serviceName, eventlog.Info|eventlog.Warning|eventlog.Error); err != nil {
		s.logger.Warn().Err(err).Msg("Failed to install event log")
		// Not fatal, continue
	}

	s.logger.Info().Msg("Service installed successfully")
	return nil
}

// Uninstall removes the Windows service
func (s *windowsService) Uninstall() error {
	s.logger.Info().Str("name", s.serviceName).Msg("Uninstalling Windows service")

	// Open service manager
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	// Open the service
	winSvc, err := m.OpenService(s.serviceName)
	if err != nil {
		return fmt.Errorf("service %s not found", s.serviceName)
	}
	defer winSvc.Close()

	// Try to stop the service first (ignore errors)
	_ = s.stopService(winSvc)

	// Delete the service
	if err := winSvc.Delete(); err != nil {
		return fmt.Errorf("failed to delete service: %w", err)
	}

	// Remove event log
	if err := eventlog.Remove(s.serviceName); err != nil {
		s.logger.Warn().Err(err).Msg("Failed to remove event log")
		// Not fatal
	}

	s.logger.Info().Msg("Service uninstalled successfully")
	return nil
}

// Start starts the Windows service
func (s *windowsService) Start() error {
	s.logger.Info().Msg("Starting service")

	// Open service manager
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	// Open the service
	winSvc, err := m.OpenService(s.serviceName)
	if err != nil {
		return fmt.Errorf("service %s not found", s.serviceName)
	}
	defer winSvc.Close()

	// Check current status
	status, err := winSvc.Query()
	if err != nil {
		return fmt.Errorf("failed to query service status: %w", err)
	}

	if status.State == svc.Running {
		s.logger.Info().Msg("Service is already running")
		return nil
	}

	// Start the service
	if err := winSvc.Start(); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	// Wait for service to start
	timeout := time.Now().Add(serviceStartTimeout)
	for {
		status, err := winSvc.Query()
		if err != nil {
			return fmt.Errorf("failed to query service status: %w", err)
		}

		if status.State == svc.Running {
			s.logger.Info().Msg("Service started successfully")
			return nil
		}

		if time.Now().After(timeout) {
			return fmt.Errorf("service start timeout")
		}

		time.Sleep(500 * time.Millisecond)
	}
}

// Stop stops the Windows service
func (s *windowsService) Stop() error {
	s.logger.Info().Msg("Stopping service")

	// Open service manager
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	// Open the service
	winSvc, err := m.OpenService(s.serviceName)
	if err != nil {
		return fmt.Errorf("service %s not found", s.serviceName)
	}
	defer winSvc.Close()

	return s.stopService(winSvc)
}

// stopService stops the service (internal helper)
func (s *windowsService) stopService(winSvc *mgr.Service) error {
	// Check current status
	status, err := winSvc.Query()
	if err != nil {
		return fmt.Errorf("failed to query service status: %w", err)
	}

	if status.State == svc.Stopped {
		s.logger.Info().Msg("Service is already stopped")
		return nil
	}

	// Send stop control
	status, err = winSvc.Control(svc.Stop)
	if err != nil {
		return fmt.Errorf("failed to stop service: %w", err)
	}

	// Wait for service to stop
	timeout := time.Now().Add(serviceStopTimeout)
	for {
		status, err := winSvc.Query()
		if err != nil {
			return fmt.Errorf("failed to query service status: %w", err)
		}

		if status.State == svc.Stopped {
			s.logger.Info().Msg("Service stopped successfully")
			return nil
		}

		if time.Now().After(timeout) {
			return fmt.Errorf("service stop timeout")
		}

		time.Sleep(500 * time.Millisecond)
	}
}

// Restart restarts the Windows service
func (s *windowsService) Restart() error {
	s.logger.Info().Msg("Restarting service")

	// Stop first
	if err := s.Stop(); err != nil {
		return err
	}

	// Wait a moment
	time.Sleep(1 * time.Second)

	// Start
	return s.Start()
}

// Status returns the service status
func (s *windowsService) Status() (Status, error) {
	// Open service manager
	m, err := mgr.Connect()
	if err != nil {
		return StatusUnknown, fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	// Open the service
	winSvc, err := m.OpenService(s.serviceName)
	if err != nil {
		return StatusNotInstalled, nil
	}
	defer winSvc.Close()

	// Query service status
	status, err := winSvc.Query()
	if err != nil {
		return StatusUnknown, fmt.Errorf("failed to query service status: %w", err)
	}

	// Map Windows status to our status
	switch status.State {
	case svc.Stopped:
		return StatusStopped, nil
	case svc.StartPending:
		return StatusStarting, nil
	case svc.StopPending:
		return StatusStopping, nil
	case svc.Running:
		return StatusRunning, nil
	default:
		return StatusUnknown, nil
	}
}

// Run executes the service in service mode (blocking)
func (s *windowsService) Run(worker func(ctx context.Context) error) error {
	s.logger.Info().Msg("Starting service in run mode")

	// Check if running as a service
	isService, err := svc.IsWindowsService()
	if err != nil {
		return fmt.Errorf("failed to check if running as service: %w", err)
	}

	if !isService {
		// Not running as a service, run worker directly
		s.logger.Info().Msg("Running in foreground mode")
		return worker(context.Background())
	}

	// Running as a service, use service handler
	s.logger.Info().Msg("Running in service mode")

	// Create service handler
	handler := &serviceHandler{
		worker: worker,
		logger: s.logger,
		elog:   s.eventLog,
	}

	// Run the service
	if err := svc.Run(s.serviceName, handler); err != nil {
		return fmt.Errorf("service run failed: %w", err)
	}

	return nil
}

// Name returns the service name
func (s *windowsService) Name() string {
	return s.config.Name
}

// DisplayName returns the service display name
func (s *windowsService) DisplayName() string {
	return s.config.DisplayName
}

// serviceHandler implements the svc.Handler interface
type serviceHandler struct {
	worker func(ctx context.Context) error
	logger zerolog.Logger
	elog   *eventlog.Log
	ctx    context.Context
	cancel context.CancelFunc
}

// Execute handles service control requests
func (h *serviceHandler) Execute(args []string, r <-chan svc.ChangeRequest, s chan<- svc.Status) (bool, uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown

	// Create context for worker
	h.ctx, h.cancel = context.WithCancel(context.Background())
	defer h.cancel()

	// Notify service is starting
	s <- svc.Status{State: svc.StartPending}

	// Start worker in background
	errChan := make(chan error, 1)
	go func() {
		errChan <- h.worker(h.ctx)
	}()

	// Notify service is running
	s <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
	if h.elog != nil {
		h.elog.Info(1, "Service started")
	}

	// Handle control requests
	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				s <- c.CurrentStatus

			case svc.Stop, svc.Shutdown:
				h.logger.Info().Uint32("command", uint32(c.Cmd)).Msg("Received stop command")
				if h.elog != nil {
					h.elog.Info(1, "Service stopping")
				}

				s <- svc.Status{State: svc.StopPending}

				// Cancel worker context
				h.cancel()

				// Wait for worker to finish (with timeout)
				select {
				case err := <-errChan:
					if err != nil {
						h.logger.Error().Err(err).Msg("Worker exited with error")
						if h.elog != nil {
							h.elog.Error(1, fmt.Sprintf("Worker error: %v", err))
						}
					}
				case <-time.After(30 * time.Second):
					h.logger.Warn().Msg("Worker shutdown timeout")
					if h.elog != nil {
						h.elog.Warning(1, "Worker shutdown timeout")
					}
				}

				s <- svc.Status{State: svc.Stopped}
				return false, 0

			default:
				h.logger.Warn().Uint32("command", uint32(c.Cmd)).Msg("Unexpected control request")
				if h.elog != nil {
					h.elog.Warning(1, fmt.Sprintf("Unexpected control request: %d", c.Cmd))
				}
			}

		case err := <-errChan:
			// Worker exited unexpectedly
			h.logger.Error().Err(err).Msg("Worker exited")
			if h.elog != nil {
				if err != nil {
					h.elog.Error(1, fmt.Sprintf("Worker exited with error: %v", err))
				} else {
					h.elog.Info(1, "Worker exited normally")
				}
			}

			s <- svc.Status{State: svc.Stopped}
			return false, 1
		}
	}
}
