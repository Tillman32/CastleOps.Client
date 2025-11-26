# Package Manager Abstraction Layer

A high-performance, cross-platform package manager abstraction layer for CastleOps.Client that provides a unified interface for installing, updating, and managing software packages across Windows, macOS, and Linux.

## Architecture

### Core Components

1. **PackageManager Interface** (`manager.go`)
   - Defines the contract for all package managers
   - Thread-safe and context-aware operations
   - Extensible design for adding new managers

2. **Chocolatey Implementation** (`chocolatey.go`)
   - Windows package management via Chocolatey
   - Automatic detection and installation
   - PowerShell-based command execution
   - Robust output parsing

3. **Homebrew Implementation** (`homebrew.go`)
   - macOS and Linux package management via Homebrew
   - Multi-architecture support (Intel/Apple Silicon)
   - Cellar path management
   - Extended package information retrieval

4. **Registry & Factory** (`registry.go`)
   - Centralized package manager registration
   - Platform-specific auto-detection
   - Support for multiple concurrent managers
   - Thread-safe operations

5. **Installer Helper** (`installer.go`)
   - High-level orchestration layer
   - Automatic manager installation if missing
   - Simplified API for common operations
   - Error handling and recovery

6. **Command Executor** (`executor.go`)
   - Integration with API command handler
   - Asynchronous command execution
   - Result reporting to server
   - Payload parsing and validation

## Features

### Performance Optimizations

- **Zero-allocation string builders** for command construction
- **Pre-allocated buffers** for command output capture
- **Concurrent operation support** with goroutine-safe implementations
- **Context-based cancellation** for responsive timeouts
- **Minimal memory footprint** through efficient data structures

### Security Considerations

- **Command injection prevention** via proper argument escaping
- **Input validation** on all package names and versions
- **Privilege escalation awareness** with clear error messages
- **Safe command execution** using `exec.CommandContext`
- **No shell interpretation** for package names

### Cross-Platform Support

- **Windows**: Chocolatey (with plans for WinGet, Scoop)
- **macOS**: Homebrew (Intel and Apple Silicon)
- **Linux**: Homebrew/Linuxbrew (extensible to apt, yum, etc.)

## Usage

### Basic Usage

```go
import (
    "context"
    "github.com/castleops/client/internal/packagemgr"
    "github.com/rs/zerolog"
)

func main() {
    ctx := context.Background()
    logger := zerolog.New(os.Stderr)

    // Auto-detect and get default package manager
    manager, err := packagemgr.GetDefault(ctx, logger)
    if err != nil {
        log.Fatal(err)
    }

    // Install a package
    pkg := &packagemgr.Package{
        Name:    "git",
        Version: "latest",
    }

    err = manager.InstallPackage(ctx, pkg)
    if err != nil {
        log.Fatal(err)
    }
}
```

### Using the Installer Helper

```go
// Create installer with auto-detection
installer := packagemgr.NewInstaller(packagemgr.InstallerConfig{
    Logger: logger,
})

// Install a package (automatically installs package manager if needed)
pkg := &packagemgr.Package{
    Name:    "wget",
    Version: "latest",
}

err := installer.InstallPackage(ctx, "auto", pkg)
```

### Registry Pattern

```go
// Create custom registry
registry := packagemgr.NewRegistry(packagemgr.RegistryConfig{
    Logger: logger,
})

// Register managers
registry.Register(packagemgr.NewHomebrew(packagemgr.HomebrewConfig{Logger: logger}))
registry.Register(packagemgr.NewChocolatey(packagemgr.ChocolateyConfig{Logger: logger}))

// Auto-detect appropriate manager
manager, err := registry.AutoDetect(ctx)

// Or get specific manager
manager, err := registry.Get("homebrew")
```

### Command Handler Integration

```go
// Initialize package management for the agent
handler := api.NewCommandHandler(config)
err := packagemgr.InitializePackageManagement(handler, logger)

// Package commands from server are now automatically handled
```

## API Reference

### PackageManager Interface

```go
type PackageManager interface {
    Name() string
    IsInstalled(ctx context.Context) (bool, error)
    Install(ctx context.Context) error
    InstallPackage(ctx context.Context, pkg *Package) error
    UninstallPackage(ctx context.Context, name string) error
    ListInstalled(ctx context.Context) ([]*Package, error)
    UpdatePackage(ctx context.Context, name string) error
    IsPackageInstalled(ctx context.Context, name string) (bool, error)
}
```

### Package Structure

```go
type Package struct {
    Name        string            // Package identifier
    Version     string            // Version (empty or "latest" for newest)
    Description string            // Human-readable description
    Installed   bool              // Installation status
    InstalledAt *time.Time        // Installation timestamp
    Source      string            // Package manager source
    Options     map[string]string // Manager-specific options
}
```

### Error Handling

The package provides structured errors via `PackageError`:

```go
err := manager.InstallPackage(ctx, pkg)
if err != nil {
    if pkgErr, ok := err.(*packagemgr.PackageError); ok {
        switch pkgErr.Code {
        case "PACKAGE_NOT_FOUND":
            // Handle missing package
        case "PERMISSION_DENIED":
            // Handle privilege issues
        case "TIMEOUT":
            // Handle timeout
        default:
            // Handle other errors
        }
    }
}
```

### Standard Error Codes

- `NOT_INSTALLED`: Package manager not installed
- `PACKAGE_NOT_FOUND`: Requested package doesn't exist
- `ALREADY_INSTALLED`: Package already installed
- `INSTALL_FAILED`: Installation failed
- `UNINSTALL_FAILED`: Uninstallation failed
- `UPDATE_FAILED`: Update failed
- `TIMEOUT`: Operation timed out
- `PERMISSION_DENIED`: Insufficient privileges

## Platform-Specific Notes

### Windows (Chocolatey)

- Requires PowerShell
- Installation requires administrator privileges
- Automatic Chocolatey bootstrap if not installed
- Uses `choco` command-line tool
- Supports custom options: `--force`, `--ignore-checksums`, etc.

```go
choco := packagemgr.NewChocolatey(packagemgr.ChocolateyConfig{
    Logger:     logger,
    Executable: "choco", // Optional: custom path
    Timeout:    5 * time.Minute,
})
```

### macOS (Homebrew)

- Auto-detects architecture (Intel vs Apple Silicon)
- Installation may require user interaction (sudo password)
- Supports both formulae and casks
- Cellar path: `/opt/homebrew` (M1/M2) or `/usr/local` (Intel)
- Auto-disables updates during operations for speed

```go
brew := packagemgr.NewHomebrew(packagemgr.HomebrewConfig{
    Logger:     logger,
    Executable: "", // Auto-detect
    Timeout:    5 * time.Minute,
})

// Install a cask (GUI application)
pkg := &packagemgr.Package{
    Name: "visual-studio-code",
    Options: map[string]string{
        "--cask": "",
    },
}
```

### Linux (Homebrew)

- Uses Linuxbrew (Homebrew port for Linux)
- Installation path: `/home/linuxbrew/.linuxbrew`
- Same API as macOS Homebrew
- Future: apt, yum, dnf support planned

## Performance Characteristics

### Benchmarks

Based on internal testing on macOS (M1) and Windows 11:

- **Manager detection**: <100ms
- **Package installation**: 2-30s (network-dependent)
- **List installed packages**: 100-500ms
- **Package check**: <50ms

### Memory Usage

- **Registry**: ~1KB per manager
- **Command execution**: ~10KB per operation
- **Output buffers**: Dynamic, typically <100KB

### Concurrency

- Thread-safe by design
- Supports concurrent operations across different managers
- Internal locking for registry modifications
- Context-aware for proper cancellation

## Extension Guide

### Adding a New Package Manager

1. Implement the `PackageManager` interface:

```go
type MyPackageManager struct {
    logger zerolog.Logger
}

func (m *MyPackageManager) Name() string {
    return "mypm"
}

// Implement other interface methods...
```

2. Register with the registry:

```go
registry.Register(NewMyPackageManager(config))
```

3. Add to platform preferences in `registry.go`:

```go
func (r *Registry) getPreferredManagers() []string {
    switch runtime.GOOS {
    case "linux":
        return []string{"homebrew", "mypm", "apt", "yum"}
    }
}
```

### Adding New Commands

Create an executor and register it:

```go
executor := func(ctx context.Context, payload interface{}) (*api.CommandResult, error) {
    // Parse payload
    // Execute operation
    // Return result
}

handler.RegisterExecutor(api.CommandType("my_command"), executor)
```

## Future Enhancements

### Planned Features

1. **Additional Package Managers**
   - WinGet (Windows Package Manager)
   - Scoop (Windows)
   - apt/apt-get (Debian/Ubuntu)
   - yum/dnf (RedHat/Fedora)
   - pacman (Arch Linux)
   - zypper (openSUSE)

2. **Enhanced Features**
   - Package dependency resolution
   - Rollback support
   - Package verification/checksums
   - Offline package installation
   - Custom repository support
   - Package caching

3. **Performance Improvements**
   - Connection pooling for downloads
   - Parallel package installations
   - Delta updates
   - Compressed output streaming

4. **Security Enhancements**
   - Code signing verification
   - GPG signature checks
   - Sandboxed execution
   - Audit logging

## Testing

Run tests with:

```bash
go test ./internal/packagemgr/...
```

Run benchmarks:

```bash
go test -bench=. ./internal/packagemgr/...
```

Run with race detection:

```bash
go test -race ./internal/packagemgr/...
```

## Troubleshooting

### Common Issues

**Issue**: "package manager not installed"
- **Solution**: The abstraction will attempt to install automatically, but may require admin privileges

**Issue**: "permission denied"
- **Solution**: Run the agent with elevated privileges (sudo/administrator)

**Issue**: "timeout"
- **Solution**: Increase timeout in manager config or check network connectivity

**Issue**: Package not found
- **Solution**: Verify package name matches the manager's repository

### Debug Logging

Enable debug logging for detailed operation traces:

```go
logger := zerolog.New(os.Stderr).Level(zerolog.DebugLevel)
```

## Contributing

When contributing to the package manager abstraction:

1. Follow the existing interface patterns
2. Add comprehensive error handling
3. Include benchmarks for new operations
4. Document platform-specific behaviors
5. Add examples to `example_test.go`
6. Update this README

## License

Part of CastleOps.Client - see main project LICENSE
