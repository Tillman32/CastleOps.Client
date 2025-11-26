package packagemgr

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// Homebrew implements PackageManager for macOS using Homebrew
type Homebrew struct {
	logger     zerolog.Logger
	executable string // Path to brew
	timeout    time.Duration
}

// HomebrewConfig configures the Homebrew package manager
type HomebrewConfig struct {
	Logger     zerolog.Logger
	Executable string        // Optional: custom path to brew
	Timeout    time.Duration // Default timeout for operations (default: 5min)
}

// NewHomebrew creates a new Homebrew package manager instance
func NewHomebrew(config HomebrewConfig) *Homebrew {
	if config.Timeout == 0 {
		config.Timeout = 5 * time.Minute
	}

	executable := config.Executable
	if executable == "" {
		// Try to find brew in common locations
		executable = findBrewExecutable()
	}

	return &Homebrew{
		logger:     config.Logger,
		executable: executable,
		timeout:    config.Timeout,
	}
}

// findBrewExecutable locates the brew binary on the system
// Homebrew can be installed in different locations based on architecture
func findBrewExecutable() string {
	// Common Homebrew paths
	// Apple Silicon (M1/M2): /opt/homebrew/bin/brew
	// Intel Mac: /usr/local/bin/brew
	commonPaths := []string{
		"/opt/homebrew/bin/brew",
		"/usr/local/bin/brew",
		"/home/linuxbrew/.linuxbrew/bin/brew",
	}

	for _, path := range commonPaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Fallback to PATH lookup
	return "brew"
}

// Name returns the package manager identifier
func (h *Homebrew) Name() string {
	return "homebrew"
}

// IsInstalled checks if Homebrew is installed by running "brew --version"
func (h *Homebrew) IsInstalled(ctx context.Context) (bool, error) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return false, nil
	}

	// Try to run brew --version
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, h.executable, "--version")
	output, err := cmd.CombinedOutput()

	if err != nil {
		h.logger.Debug().
			Err(err).
			Str("output", string(output)).
			Msg("Homebrew not found")
		return false, nil
	}

	// Verify output contains "Homebrew"
	if strings.Contains(string(output), "Homebrew") {
		h.logger.Debug().
			Str("version", strings.TrimSpace(strings.Split(string(output), "\n")[0])).
			Msg("Homebrew detected")
		return true, nil
	}

	return false, nil
}

// Install installs Homebrew using the official installation script
// Requires user interaction for password on macOS
func (h *Homebrew) Install(ctx context.Context) error {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return fmt.Errorf("homebrew is only supported on macOS and Linux")
	}

	// Check if already installed
	installed, err := h.IsInstalled(ctx)
	if err != nil {
		return WrapError(err, "INSTALL_CHECK_FAILED", "failed to check if homebrew is installed")
	}
	if installed {
		h.logger.Info().Msg("Homebrew already installed")
		return nil
	}

	h.logger.Info().Msg("Installing Homebrew (may require user interaction for sudo password)")

	// Homebrew installation script
	// This is the official method from https://brew.sh
	installURL := "https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh"

	// Download and execute installation script
	// Note: This requires user interaction for sudo password
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	// Use bash to execute the installation script
	installCmd := fmt.Sprintf(`bash -c "$(curl -fsSL %s)"`, installURL)
	cmd := exec.CommandContext(ctx, "bash", "-c", installCmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Important: Connect stdin for user interaction
	cmd.Stdin = os.Stdin

	startTime := time.Now()
	err = cmd.Run()
	duration := time.Since(startTime)

	if err != nil {
		h.logger.Error().
			Err(err).
			Str("stdout", stdout.String()).
			Str("stderr", stderr.String()).
			Dur("duration", duration).
			Msg("Homebrew installation failed")

		return WrapError(err, "INSTALL_FAILED", "homebrew installation failed").
			WithDetails("stdout", stdout.String()).
			WithDetails("stderr", stderr.String())
	}

	// Update executable path after installation
	h.executable = findBrewExecutable()

	// Verify installation
	installed, err = h.IsInstalled(ctx)
	if err != nil || !installed {
		return WrapError(err, "INSTALL_VERIFY_FAILED", "homebrew installation verification failed")
	}

	h.logger.Info().
		Dur("duration", duration).
		Str("path", h.executable).
		Msg("Homebrew installed successfully")

	return nil
}

// InstallPackage installs a package using Homebrew
func (h *Homebrew) InstallPackage(ctx context.Context, pkg *Package) error {
	if pkg == nil || pkg.Name == "" {
		return fmt.Errorf("invalid package: name is required")
	}

	// Ensure Homebrew is installed
	installed, err := h.IsInstalled(ctx)
	if err != nil {
		return WrapError(err, "CHECK_FAILED", "failed to check homebrew installation")
	}
	if !installed {
		return ErrNotInstalled
	}

	h.logger.Info().
		Str("package", pkg.Name).
		Str("version", pkg.Version).
		Msg("Installing package via Homebrew")

	// Build command: brew install <name>
	// Note: Homebrew doesn't support version-specific installs directly like Chocolatey
	// You would need to use brew tap and formulas for specific versions
	args := []string{"install", pkg.Name}

	// Add custom options (e.g., --cask for GUI apps)
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
	output, err := h.executeCommand(ctx, args...)
	if err != nil {
		// Check if already installed (not necessarily an error)
		if strings.Contains(output.Stderr, "already installed") ||
			strings.Contains(output.Stdout, "already installed") {
			h.logger.Info().
				Str("package", pkg.Name).
				Msg("Package already installed")
			return nil
		}

		h.logger.Error().
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

	h.logger.Info().
		Str("package", pkg.Name).
		Dur("duration", output.Duration).
		Msg("Package installed successfully")

	return nil
}

// UninstallPackage removes a package using Homebrew
func (h *Homebrew) UninstallPackage(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("package name is required")
	}

	// Ensure Homebrew is installed
	installed, err := h.IsInstalled(ctx)
	if err != nil {
		return WrapError(err, "CHECK_FAILED", "failed to check homebrew installation")
	}
	if !installed {
		return ErrNotInstalled
	}

	h.logger.Info().
		Str("package", name).
		Msg("Uninstalling package via Homebrew")

	// Build command: brew uninstall <name>
	args := []string{"uninstall", name}

	// Execute command
	output, err := h.executeCommand(ctx, args...)
	if err != nil {
		h.logger.Error().
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

	h.logger.Info().
		Str("package", name).
		Dur("duration", output.Duration).
		Msg("Package uninstalled successfully")

	return nil
}

// UpdatePackage updates a package to the latest version
func (h *Homebrew) UpdatePackage(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("package name is required")
	}

	// Ensure Homebrew is installed
	installed, err := h.IsInstalled(ctx)
	if err != nil {
		return WrapError(err, "CHECK_FAILED", "failed to check homebrew installation")
	}
	if !installed {
		return ErrNotInstalled
	}

	h.logger.Info().
		Str("package", name).
		Msg("Updating package via Homebrew")

	// Build command: brew upgrade <name>
	args := []string{"upgrade", name}

	// Execute command
	output, err := h.executeCommand(ctx, args...)
	if err != nil {
		// Check if already up-to-date (not necessarily an error)
		if strings.Contains(output.Stderr, "already installed") ||
			strings.Contains(output.Stdout, "already installed") {
			h.logger.Info().
				Str("package", name).
				Msg("Package already up-to-date")
			return nil
		}

		h.logger.Error().
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

	h.logger.Info().
		Str("package", name).
		Dur("duration", output.Duration).
		Msg("Package updated successfully")

	return nil
}

// ListInstalled returns all packages installed via Homebrew
func (h *Homebrew) ListInstalled(ctx context.Context) ([]*Package, error) {
	// Ensure Homebrew is installed
	installed, err := h.IsInstalled(ctx)
	if err != nil {
		return nil, WrapError(err, "CHECK_FAILED", "failed to check homebrew installation")
	}
	if !installed {
		return nil, ErrNotInstalled
	}

	h.logger.Debug().Msg("Listing installed packages")

	// Build command: brew list --versions
	args := []string{"list", "--versions"}

	// Execute command
	output, err := h.executeCommand(ctx, args...)
	if err != nil {
		return nil, WrapError(err, "LIST_FAILED", "failed to list installed packages")
	}

	// Parse output - format is typically "PackageName Version1 Version2..."
	packages := h.parsePackageList(output.Stdout)

	h.logger.Debug().
		Int("count", len(packages)).
		Msg("Listed installed packages")

	return packages, nil
}

// IsPackageInstalled checks if a specific package is installed
func (h *Homebrew) IsPackageInstalled(ctx context.Context, name string) (bool, error) {
	if name == "" {
		return false, fmt.Errorf("package name is required")
	}

	// Ensure Homebrew is installed
	installed, err := h.IsInstalled(ctx)
	if err != nil {
		return false, WrapError(err, "CHECK_FAILED", "failed to check homebrew installation")
	}
	if !installed {
		return false, ErrNotInstalled
	}

	// Build command: brew list <name>
	args := []string{"list", name}

	// Execute command (will fail with exit code 1 if not installed)
	output, err := h.executeCommand(ctx, args...)
	if err != nil {
		// If exit code is 1 and error mentions "No such keg", package is not installed
		if output.ExitCode == 1 && strings.Contains(output.Stderr, "No such keg") {
			return false, nil
		}
		return false, WrapError(err, "CHECK_FAILED", "failed to check package installation")
	}

	return true, nil
}

// executeCommand runs a Homebrew command with timeout and returns output
func (h *Homebrew) executeCommand(ctx context.Context, args ...string) (*CommandOutput, error) {
	// Apply timeout
	cmdCtx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	// Build command
	cmd := exec.CommandContext(cmdCtx, h.executable, args...)

	// Set HOMEBREW_NO_AUTO_UPDATE to prevent automatic updates during operations
	// This speeds up commands significantly
	cmd.Env = append(os.Environ(), "HOMEBREW_NO_AUTO_UPDATE=1")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	h.logger.Debug().
		Str("command", h.executable).
		Strs("args", args).
		Msg("Executing Homebrew command")

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

	h.logger.Debug().
		Dur("duration", duration).
		Int("exit_code", output.ExitCode).
		Msg("Command completed")

	return output, nil
}

// parsePackageList parses the output of "brew list --versions"
func (h *Homebrew) parsePackageList(output string) []*Package {
	packages := make([]*Package, 0)

	lines := strings.Split(output, "\n")
	// Regex to match "PackageName Version(s)"
	// Format: "git 2.39.1" or "python@3.11 3.11.1 3.11.2"
	packageRegex := regexp.MustCompile(`^([^\s]+)\s+(.+)$`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse package name and version(s)
		matches := packageRegex.FindStringSubmatch(line)
		if len(matches) == 3 {
			name := matches[1]
			versions := strings.Fields(matches[2])

			// Use the first version if multiple are present
			version := ""
			if len(versions) > 0 {
				version = versions[0]
			}

			packages = append(packages, &Package{
				Name:      name,
				Version:   version,
				Source:    "homebrew",
				Installed: true,
			})
		}
	}

	return packages
}

// GetCellarPath returns the Cellar path for Homebrew installations
// This is useful for determining where packages are installed
func (h *Homebrew) GetCellarPath() string {
	// Determine base path from executable location
	if strings.HasPrefix(h.executable, "/opt/homebrew") {
		return "/opt/homebrew/Cellar"
	}
	return "/usr/local/Cellar"
}

// GetPackageInfo retrieves detailed information about an installed package
func (h *Homebrew) GetPackageInfo(ctx context.Context, name string) (*Package, error) {
	if name == "" {
		return nil, fmt.Errorf("package name is required")
	}

	// Check if installed
	installed, err := h.IsPackageInstalled(ctx, name)
	if err != nil {
		return nil, err
	}
	if !installed {
		return nil, ErrPackageNotFound
	}

	// Build command: brew info <name>
	args := []string{"info", name, "--json=v2"}

	output, err := h.executeCommand(ctx, args...)
	if err != nil {
		return nil, WrapError(err, "INFO_FAILED", "failed to get package info")
	}

	// For simplicity, return basic package info
	// Full JSON parsing would provide more details
	pkg := &Package{
		Name:      name,
		Source:    "homebrew",
		Installed: true,
	}

	// Try to extract version from cellar path if it exists
	cellarPath := filepath.Join(h.GetCellarPath(), name)
	if entries, err := os.ReadDir(cellarPath); err == nil && len(entries) > 0 {
		// Use the first directory as version
		pkg.Version = entries[0].Name()
		if info, err := entries[0].Info(); err == nil {
			installTime := info.ModTime()
			pkg.InstalledAt = &installTime
		}
	}

	// Parse description from output if available
	if strings.Contains(output.Stdout, name) {
		pkg.Description = name // Simplified - full JSON parsing would extract actual description
	}

	return pkg, nil
}
