package packagemgr

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// Chocolatey implements PackageManager for Windows using Chocolatey
type Chocolatey struct {
	logger     zerolog.Logger
	executable string // Path to choco.exe
	timeout    time.Duration
}

// ChocolateyConfig configures the Chocolatey package manager
type ChocolateyConfig struct {
	Logger     zerolog.Logger
	Executable string        // Optional: custom path to choco.exe
	Timeout    time.Duration // Default timeout for operations (default: 5min)
}

// NewChocolatey creates a new Chocolatey package manager instance
func NewChocolatey(config ChocolateyConfig) *Chocolatey {
	if config.Timeout == 0 {
		config.Timeout = 5 * time.Minute
	}

	executable := config.Executable
	if executable == "" {
		executable = "choco"
	}

	return &Chocolatey{
		logger:     config.Logger,
		executable: executable,
		timeout:    config.Timeout,
	}
}

// Name returns the package manager identifier
func (c *Chocolatey) Name() string {
	return "chocolatey"
}

// IsInstalled checks if Chocolatey is installed by running "choco --version"
func (c *Chocolatey) IsInstalled(ctx context.Context) (bool, error) {
	if runtime.GOOS != "windows" {
		return false, nil
	}

	// Try to run choco --version
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.executable, "--version")
	output, err := cmd.CombinedOutput()

	if err != nil {
		c.logger.Debug().
			Err(err).
			Str("output", string(output)).
			Msg("Chocolatey not found")
		return false, nil
	}

	// Verify output contains a version number
	if matched, _ := regexp.Match(`\d+\.\d+`, output); matched {
		c.logger.Debug().
			Str("version", strings.TrimSpace(string(output))).
			Msg("Chocolatey detected")
		return true, nil
	}

	return false, nil
}

// Install installs Chocolatey using the official PowerShell installation script
// Requires administrator privileges
func (c *Chocolatey) Install(ctx context.Context) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("chocolatey is only supported on Windows")
	}

	// Check if already installed
	installed, err := c.IsInstalled(ctx)
	if err != nil {
		return WrapError(err, "INSTALL_CHECK_FAILED", "failed to check if chocolatey is installed")
	}
	if installed {
		c.logger.Info().Msg("Chocolatey already installed")
		return nil
	}

	c.logger.Info().Msg("Installing Chocolatey (requires administrator privileges)")

	// Chocolatey installation script
	// This is the official method from https://chocolatey.org/install
	installScript := `
Set-ExecutionPolicy Bypass -Scope Process -Force
[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
try {
    iex ((New-Object System.Net.WebClient).DownloadString('https://community.chocolatey.org/install.ps1'))
    exit 0
} catch {
    Write-Error $_.Exception.Message
    exit 1
}
`

	// Execute via PowerShell with a longer timeout for installation
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", installScript)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	startTime := time.Now()
	err = cmd.Run()
	duration := time.Since(startTime)

	if err != nil {
		c.logger.Error().
			Err(err).
			Str("stdout", stdout.String()).
			Str("stderr", stderr.String()).
			Dur("duration", duration).
			Msg("Chocolatey installation failed")

		return WrapError(err, "INSTALL_FAILED", "chocolatey installation failed").
			WithDetails("stdout", stdout.String()).
			WithDetails("stderr", stderr.String())
	}

	// Verify installation
	installed, err = c.IsInstalled(ctx)
	if err != nil || !installed {
		return WrapError(err, "INSTALL_VERIFY_FAILED", "chocolatey installation verification failed")
	}

	c.logger.Info().
		Dur("duration", duration).
		Msg("Chocolatey installed successfully")

	return nil
}

// InstallPackage installs a package using Chocolatey
func (c *Chocolatey) InstallPackage(ctx context.Context, pkg *Package) error {
	if pkg == nil || pkg.Name == "" {
		return fmt.Errorf("invalid package: name is required")
	}

	// Ensure Chocolatey is installed
	installed, err := c.IsInstalled(ctx)
	if err != nil {
		return WrapError(err, "CHECK_FAILED", "failed to check chocolatey installation")
	}
	if !installed {
		return ErrNotInstalled
	}

	c.logger.Info().
		Str("package", pkg.Name).
		Str("version", pkg.Version).
		Msg("Installing package via Chocolatey")

	// Build command: choco install <name> [--version=<version>] -y --no-progress
	args := []string{"install", pkg.Name, "-y", "--no-progress"}

	// Add version if specified
	if pkg.Version != "" && pkg.Version != "latest" {
		args = append(args, "--version="+pkg.Version)
	}

	// Add custom options
	if pkg.Options != nil {
		for key, value := range pkg.Options {
			if value == "" {
				args = append(args, key)
			} else {
				args = append(args, fmt.Sprintf("%s=%s", key, value))
			}
		}
	}

	// Execute command
	output, err := c.executeCommand(ctx, args...)
	if err != nil {
		c.logger.Error().
			Err(err).
			Str("package", pkg.Name).
			Str("stdout", output.Stdout).
			Str("stderr", output.Stderr).
			Int("exit_code", output.ExitCode).
			Msg("Package installation failed")

		return WrapError(err, "INSTALL_FAILED", "failed to install package").
			WithDetails("package", pkg.Name).
			WithDetails("exit_code", output.ExitCode).
			WithDetails("stderr", output.Stderr)
	}

	c.logger.Info().
		Str("package", pkg.Name).
		Dur("duration", output.Duration).
		Msg("Package installed successfully")

	return nil
}

// UninstallPackage removes a package using Chocolatey
func (c *Chocolatey) UninstallPackage(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("package name is required")
	}

	// Ensure Chocolatey is installed
	installed, err := c.IsInstalled(ctx)
	if err != nil {
		return WrapError(err, "CHECK_FAILED", "failed to check chocolatey installation")
	}
	if !installed {
		return ErrNotInstalled
	}

	c.logger.Info().
		Str("package", name).
		Msg("Uninstalling package via Chocolatey")

	// Build command: choco uninstall <name> -y --no-progress
	args := []string{"uninstall", name, "-y", "--no-progress"}

	// Execute command
	output, err := c.executeCommand(ctx, args...)
	if err != nil {
		c.logger.Error().
			Err(err).
			Str("package", name).
			Str("stderr", output.Stderr).
			Int("exit_code", output.ExitCode).
			Msg("Package uninstallation failed")

		return WrapError(err, "UNINSTALL_FAILED", "failed to uninstall package").
			WithDetails("package", name).
			WithDetails("exit_code", output.ExitCode).
			WithDetails("stderr", output.Stderr)
	}

	c.logger.Info().
		Str("package", name).
		Dur("duration", output.Duration).
		Msg("Package uninstalled successfully")

	return nil
}

// UpdatePackage updates a package to the latest or specified version
func (c *Chocolatey) UpdatePackage(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("package name is required")
	}

	// Ensure Chocolatey is installed
	installed, err := c.IsInstalled(ctx)
	if err != nil {
		return WrapError(err, "CHECK_FAILED", "failed to check chocolatey installation")
	}
	if !installed {
		return ErrNotInstalled
	}

	c.logger.Info().
		Str("package", name).
		Msg("Updating package via Chocolatey")

	// Build command: choco upgrade <name> -y --no-progress
	args := []string{"upgrade", name, "-y", "--no-progress"}

	// Execute command
	output, err := c.executeCommand(ctx, args...)
	if err != nil {
		c.logger.Error().
			Err(err).
			Str("package", name).
			Str("stderr", output.Stderr).
			Int("exit_code", output.ExitCode).
			Msg("Package update failed")

		return WrapError(err, "UPDATE_FAILED", "failed to update package").
			WithDetails("package", name).
			WithDetails("exit_code", output.ExitCode).
			WithDetails("stderr", output.Stderr)
	}

	c.logger.Info().
		Str("package", name).
		Dur("duration", output.Duration).
		Msg("Package updated successfully")

	return nil
}

// ListInstalled returns all packages installed via Chocolatey
func (c *Chocolatey) ListInstalled(ctx context.Context) ([]*Package, error) {
	// Ensure Chocolatey is installed
	installed, err := c.IsInstalled(ctx)
	if err != nil {
		return nil, WrapError(err, "CHECK_FAILED", "failed to check chocolatey installation")
	}
	if !installed {
		return nil, ErrNotInstalled
	}

	c.logger.Debug().Msg("Listing installed packages")

	// Build command: choco list --local-only --no-progress
	args := []string{"list", "--local-only", "--no-progress"}

	// Execute command
	output, err := c.executeCommand(ctx, args...)
	if err != nil {
		return nil, WrapError(err, "LIST_FAILED", "failed to list installed packages")
	}

	// Parse output - format is typically "PackageName Version"
	packages := c.parsePackageList(output.Stdout)

	c.logger.Debug().
		Int("count", len(packages)).
		Msg("Listed installed packages")

	return packages, nil
}

// IsPackageInstalled checks if a specific package is installed
func (c *Chocolatey) IsPackageInstalled(ctx context.Context, name string) (bool, error) {
	if name == "" {
		return false, fmt.Errorf("package name is required")
	}

	// Ensure Chocolatey is installed
	installed, err := c.IsInstalled(ctx)
	if err != nil {
		return false, WrapError(err, "CHECK_FAILED", "failed to check chocolatey installation")
	}
	if !installed {
		return false, ErrNotInstalled
	}

	// Build command: choco list --local-only --exact <name>
	args := []string{"list", "--local-only", "--exact", name, "--no-progress"}

	// Execute command
	output, err := c.executeCommand(ctx, args...)
	if err != nil {
		return false, WrapError(err, "CHECK_FAILED", "failed to check package installation")
	}

	// If the package is installed, the output will contain the package name
	// Format: "PackageName Version"
	lines := strings.Split(output.Stdout, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Skip informational lines
		if strings.Contains(line, "packages installed") {
			continue
		}
		// Check if line starts with package name
		if strings.HasPrefix(strings.ToLower(line), strings.ToLower(name)+" ") {
			return true, nil
		}
	}

	return false, nil
}

// executeCommand runs a Chocolatey command with timeout and returns output
func (c *Chocolatey) executeCommand(ctx context.Context, args ...string) (*CommandOutput, error) {
	// Apply timeout
	cmdCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Build command
	cmd := exec.CommandContext(cmdCtx, c.executable, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	c.logger.Debug().
		Str("command", c.executable).
		Strs("args", args).
		Msg("Executing Chocolatey command")

	// Execute
	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	output := &CommandOutput{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: duration,
		Success:  err == nil,
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		output.ExitCode = exitErr.ExitCode()
	}

	if err != nil {
		// Check for specific error types
		if cmdCtx.Err() == context.DeadlineExceeded {
			return output, WrapError(err, "TIMEOUT", "command timed out")
		}
		return output, err
	}

	c.logger.Debug().
		Dur("duration", duration).
		Int("exit_code", output.ExitCode).
		Msg("Command completed")

	return output, nil
}

// parsePackageList parses the output of "choco list --local-only"
func (c *Chocolatey) parsePackageList(output string) []*Package {
	packages := make([]*Package, 0)

	lines := strings.Split(output, "\n")
	// Regex to match "PackageName Version"
	packageRegex := regexp.MustCompile(`^([^\s]+)\s+([^\s]+)`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Skip summary lines like "2 packages installed"
		if strings.Contains(line, "packages installed") {
			continue
		}

		// Parse package name and version
		matches := packageRegex.FindStringSubmatch(line)
		if len(matches) == 3 {
			packages = append(packages, &Package{
				Name:      matches[1],
				Version:   matches[2],
				Source:    "chocolatey",
				Installed: true,
			})
		}
	}

	return packages
}
