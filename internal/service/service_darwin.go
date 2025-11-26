// Package service provides macOS launchd service integration.
// This file implements the Service interface using macOS launchd and launchctl.

//go:build darwin

package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/template"

	"github.com/rs/zerolog"
)

const (
	// LaunchAgentsDir is the user-level launch agents directory
	launchAgentsDir = "Library/LaunchAgents"

	// LaunchDaemonsDir is the system-level launch daemons directory
	launchDaemonsDir = "/Library/LaunchDaemons"

	// Plist template for launchd configuration
	plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>{{.Label}}</string>

    <key>ProgramArguments</key>
    <array>
        <string>{{.Executable}}</string>
{{- range .Arguments}}
        <string>{{.}}</string>
{{- end}}
    </array>

    <key>RunAtLoad</key>
    <true/>

    <key>KeepAlive</key>
    <dict>
        <key>SuccessfulExit</key>
        <false/>
        <key>Crashed</key>
        <true/>
    </dict>

    <key>StandardOutPath</key>
    <string>{{.StdoutPath}}</string>

    <key>StandardErrorPath</key>
    <string>{{.StderrPath}}</string>

    <key>WorkingDirectory</key>
    <string>{{.WorkingDirectory}}</string>

    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
    </dict>

    <key>ThrottleInterval</key>
    <integer>10</integer>

    <key>ProcessType</key>
    <string>Background</string>

    <key>Nice</key>
    <integer>0</integer>
</dict>
</plist>
`
)

// darwinService implements the Service interface for macOS using launchd
type darwinService struct {
	config     *Config
	logger     zerolog.Logger
	plistPath  string
	plistDir   string
	label      string
	stdoutPath string
	stderrPath string
}

// plistData holds the data for plist template rendering
type plistData struct {
	Label            string
	Executable       string
	Arguments        []string
	WorkingDirectory string
	StdoutPath       string
	StderrPath       string
}

// newDarwinService creates a new macOS launchd service instance
func newDarwinService(config *Config) (Service, error) {
	svc := &darwinService{
		config: config,
		logger: config.Logger.With().Str("service", "launchd").Logger(),
		label:  config.Name,
	}

	// Determine plist directory based on user vs system service
	if config.UserService {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		svc.plistDir = filepath.Join(homeDir, launchAgentsDir)
	} else {
		svc.plistDir = launchDaemonsDir
	}

	// Construct plist file path
	svc.plistPath = filepath.Join(svc.plistDir, fmt.Sprintf("%s.plist", config.Name))

	// Set up log paths
	logDir := getLogDirectory(config.UserService)
	svc.stdoutPath = filepath.Join(logDir, "stdout.log")
	svc.stderrPath = filepath.Join(logDir, "stderr.log")

	return svc, nil
}

// Install installs the launchd service
func (s *darwinService) Install() error {
	s.logger.Info().
		Str("plist_path", s.plistPath).
		Bool("user_service", s.config.UserService).
		Msg("Installing launchd service")

	// Check if already installed
	if _, err := os.Stat(s.plistPath); err == nil {
		return fmt.Errorf("service already installed at %s", s.plistPath)
	}

	// Create plist directory if it doesn't exist
	if err := os.MkdirAll(s.plistDir, 0755); err != nil {
		return fmt.Errorf("failed to create plist directory: %w", err)
	}

	// Create log directory
	logDir := filepath.Dir(s.stdoutPath)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Generate plist content
	plistContent, err := s.generatePlist()
	if err != nil {
		return fmt.Errorf("failed to generate plist: %w", err)
	}

	// Write plist file
	if err := os.WriteFile(s.plistPath, []byte(plistContent), 0644); err != nil {
		return fmt.Errorf("failed to write plist file: %w", err)
	}

	s.logger.Info().Msg("Plist file created successfully")

	// Load the service
	if err := s.load(); err != nil {
		// Clean up plist file on failure
		os.Remove(s.plistPath)
		return fmt.Errorf("failed to load service: %w", err)
	}

	s.logger.Info().Msg("Service installed and loaded successfully")
	return nil
}

// Uninstall removes the launchd service
func (s *darwinService) Uninstall() error {
	s.logger.Info().Str("plist_path", s.plistPath).Msg("Uninstalling launchd service")

	// Check if installed
	if _, err := os.Stat(s.plistPath); os.IsNotExist(err) {
		return fmt.Errorf("service not installed")
	}

	// Try to stop the service first (ignore errors if not running)
	_ = s.Stop()

	// Unload the service (ignore errors if not loaded)
	_ = s.unload()

	// Remove plist file
	if err := os.Remove(s.plistPath); err != nil {
		return fmt.Errorf("failed to remove plist file: %w", err)
	}

	s.logger.Info().Msg("Service uninstalled successfully")
	return nil
}

// Start starts the service
func (s *darwinService) Start() error {
	s.logger.Info().Msg("Starting service")

	// Check if service is installed
	if _, err := os.Stat(s.plistPath); os.IsNotExist(err) {
		return fmt.Errorf("service not installed, run install first")
	}

	// Use launchctl start
	cmd := exec.Command("launchctl", "start", s.label)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start service: %w (output: %s)", err, string(output))
	}

	s.logger.Info().Msg("Service started successfully")
	return nil
}

// Stop stops the service
func (s *darwinService) Stop() error {
	s.logger.Info().Msg("Stopping service")

	// Use launchctl stop
	cmd := exec.Command("launchctl", "stop", s.label)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to stop service: %w (output: %s)", err, string(output))
	}

	s.logger.Info().Msg("Service stopped successfully")
	return nil
}

// Restart restarts the service
func (s *darwinService) Restart() error {
	s.logger.Info().Msg("Restarting service")

	// Stop first (ignore errors)
	_ = s.Stop()

	// Start
	return s.Start()
}

// Status returns the service status
func (s *darwinService) Status() (Status, error) {
	// Check if plist file exists
	if _, err := os.Stat(s.plistPath); os.IsNotExist(err) {
		return StatusNotInstalled, nil
	}

	// Use launchctl list to check if service is loaded
	cmd := exec.Command("launchctl", "list", s.label)
	output, err := cmd.CombinedOutput()

	if err != nil {
		// If command fails, service is not loaded
		if strings.Contains(string(output), "Could not find service") {
			return StatusStopped, nil
		}
		return StatusUnknown, fmt.Errorf("failed to get service status: %w", err)
	}

	// Parse output to determine if running
	// Output format: PID    Status    Label
	// If PID is "-", service is not running
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, s.label) {
			fields := strings.Fields(line)
			if len(fields) >= 1 {
				pid := fields[0]
				if pid == "-" || pid == "" {
					return StatusStopped, nil
				}
				return StatusRunning, nil
			}
		}
	}

	return StatusStopped, nil
}

// Run executes the service in service mode (blocking)
func (s *darwinService) Run(worker func(ctx context.Context) error) error {
	s.logger.Info().Msg("Starting service in run mode")

	// Create context that listens for termination signals
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// Run worker in goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- worker(ctx)
	}()

	// Wait for signal or worker completion
	select {
	case sig := <-sigChan:
		s.logger.Info().Str("signal", sig.String()).Msg("Received signal, shutting down")
		cancel()

		// Wait for worker to finish with timeout
		select {
		case err := <-errChan:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}

	case err := <-errChan:
		if err != nil {
			s.logger.Error().Err(err).Msg("Worker exited with error")
		}
		return err
	}
}

// Name returns the service name
func (s *darwinService) Name() string {
	return s.config.Name
}

// DisplayName returns the service display name
func (s *darwinService) DisplayName() string {
	return s.config.DisplayName
}

// generatePlist generates the plist XML content
func (s *darwinService) generatePlist() (string, error) {
	tmpl, err := template.New("plist").Parse(plistTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse plist template: %w", err)
	}

	data := plistData{
		Label:            s.label,
		Executable:       s.config.Executable,
		Arguments:        s.config.Arguments,
		WorkingDirectory: s.config.WorkingDirectory,
		StdoutPath:       s.stdoutPath,
		StderrPath:       s.stderrPath,
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute plist template: %w", err)
	}

	return buf.String(), nil
}

// load loads the service using launchctl
func (s *darwinService) load() error {
	cmd := exec.Command("launchctl", "load", s.plistPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl load failed: %w (output: %s)", err, string(output))
	}
	return nil
}

// unload unloads the service using launchctl
func (s *darwinService) unload() error {
	cmd := exec.Command("launchctl", "unload", s.plistPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl unload failed: %w (output: %s)", err, string(output))
	}
	return nil
}

// getLogDirectory returns the appropriate log directory based on service type
func getLogDirectory(userService bool) string {
	if userService {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(homeDir, "Library", "Logs", "CastleOps")
		}
	}
	return "/Library/Logs/CastleOps"
}
