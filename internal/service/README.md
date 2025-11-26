# Service Package

This package provides cross-platform service/daemon management for the CastleOps Client.

## Features

- **Cross-platform abstraction**: Single interface for macOS, Windows, and Linux services
- **macOS launchd integration**: Native Launch Agents/Daemons support
- **Windows Service integration**: Native Windows Service Control Manager support
- **Automatic restart**: Service auto-recovery on failure
- **Clean lifecycle management**: Install, uninstall, start, stop, restart, status
- **Privilege checking**: Verify elevated permissions before operations

## Architecture

### Service Interface

```go
type Service interface {
    Install() error
    Uninstall() error
    Start() error
    Stop() error
    Restart() error
    Status() (Status, error)
    Run(worker func(ctx context.Context) error) error
    Name() string
    DisplayName() string
}
```

### Platform Implementations

- **macOS (service_darwin.go)**: Uses launchd and launchctl
- **Windows (service_windows.go)**: Uses Windows Service Control Manager
- **Linux**: TODO - systemd support

## Usage

### Installation

```go
import "github.com/castleops/client/internal/service"

// Create service configuration
cfg := &service.Config{
    Name:             "com.castleops.client",
    DisplayName:      "CastleOps Client",
    Description:      "System monitoring and management agent",
    Executable:       "/usr/local/bin/castleops-client",
    Arguments:        []string{"-config", "/etc/castleops/config.yaml"},
    WorkingDirectory: "/var/lib/castleops",
    UserService:      false, // System-level service
    Logger:           logger,
}

// Create platform-specific service
svc, err := service.New(cfg)
if err != nil {
    log.Fatal(err)
}

// Install the service (requires elevated privileges)
if err := svc.Install(); err != nil {
    log.Fatal(err)
}
```

### Running as a Service

```go
// Define your service worker
worker := func(ctx context.Context) error {
    // Your service logic here
    // The context will be cancelled when the service should stop
    <-ctx.Done()
    return nil
}

// Run the service (blocking call)
if err := svc.Run(worker); err != nil {
    log.Fatal(err)
}
```

### Service Management

```go
// Start the service
if err := svc.Start(); err != nil {
    log.Fatal(err)
}

// Check status
status, err := svc.Status()
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Service status: %s\n", status)

// Stop the service
if err := svc.Stop(); err != nil {
    log.Fatal(err)
}

// Uninstall the service
if err := svc.Uninstall(); err != nil {
    log.Fatal(err)
}
```

## macOS Implementation

### Features

- **Launch Agents**: User-level services (~Library/LaunchAgents)
- **Launch Daemons**: System-level services (/Library/LaunchDaemons)
- **Auto-start**: RunAtLoad configuration
- **Keep-alive**: Automatic restart on crash
- **Logging**: stdout/stderr redirected to log files
- **Throttling**: 10-second throttle interval to prevent crash loops

### Plist Configuration

The service generates a plist file with:
- Label (service identifier)
- Program arguments
- Auto-start on boot (RunAtLoad)
- Keep-alive on crash
- Standard output/error logging
- Working directory
- Environment variables
- Throttle interval

### Commands

```bash
# Install service (creates plist and loads it)
sudo castleops-client -install

# Start service
launchctl start com.castleops.client

# Stop service
launchctl stop com.castleops.client

# Check status
launchctl list | grep castleops

# Uninstall service (unloads and removes plist)
sudo castleops-client -uninstall
```

## Windows Implementation

### Features

- **Windows Service**: Native Windows Service integration
- **Service Control Manager**: Full SCM integration
- **Event Log**: Windows Event Log integration
- **Auto-recovery**: Restart on failure (3 attempts with 60s delay)
- **LocalService account**: Runs as NT AUTHORITY\LocalService
- **Automatic startup**: Configured for automatic start

### Service Configuration

- **Start Type**: Automatic
- **Account**: NT AUTHORITY\LocalService
- **Recovery Actions**: Restart service after 60 seconds (3 times)
- **Reset Period**: 24 hours (86400 seconds)

### Commands

```powershell
# Install service (registers with SCM)
castleops-client.exe -install

# Start service
Start-Service CastleOpsClient

# Stop service
Stop-Service CastleOpsClient

# Check status
Get-Service CastleOpsClient

# Restart service
Restart-Service CastleOpsClient

# Uninstall service
castleops-client.exe -uninstall
```

## Privilege Requirements

All service installation/uninstallation operations require elevated privileges:

- **macOS**: root (use sudo)
- **Windows**: Administrator (run as administrator)
- **Linux**: root (use sudo)

The package provides helpers to check and enforce privilege requirements:

```go
// Check if running with elevated privileges
if service.IsElevated() {
    // Can perform service operations
}

// Require elevated privileges (returns error if not elevated)
if err := service.RequireElevated(); err != nil {
    log.Fatal(err)
}
```

## Error Handling

All service operations return descriptive errors:

```go
if err := svc.Install(); err != nil {
    switch {
    case strings.Contains(err.Error(), "already exists"):
        // Service is already installed
    case strings.Contains(err.Error(), "requires root"):
        // Need elevated privileges
    default:
        // Other error
    }
}
```

## Logging

The service uses zerolog for structured logging:

```go
cfg := &service.Config{
    Logger: logger.With().Str("component", "service").Logger(),
    // ... other config
}
```

Service logs include:
- Installation/uninstallation events
- Start/stop events
- Status checks
- Error conditions
- Platform-specific operations

## Best Practices

1. **Always check privileges**: Use `RequireElevated()` before install/uninstall
2. **Handle signals gracefully**: Implement proper shutdown in your worker function
3. **Use absolute paths**: For executable, config, and working directory
4. **Test status first**: Check if service is installed before operations
5. **Clean uninstall**: Always stop service before uninstalling
6. **Log extensively**: Use the provided logger for debugging

## Testing

The service package includes:
- Platform-specific implementations with proper build tags
- Stub implementations for unsupported platforms
- Comprehensive error handling
- Status checks before operations

## Future Enhancements

- [ ] Linux systemd support
- [ ] User-level services on Windows
- [ ] Service dependency management
- [ ] Custom recovery actions
- [ ] Service restart without reinstall
- [ ] Service configuration updates
- [ ] Health check integration

## Performance Considerations

- **Zero overhead**: Service abstraction has minimal performance impact
- **Non-blocking status checks**: Fast status queries
- **Efficient signal handling**: Clean shutdown coordination
- **Resource cleanup**: Proper cleanup on uninstall

## Platform Compatibility

| Platform | Support | Implementation | Notes |
|----------|---------|----------------|-------|
| macOS    | ✅      | launchd        | Launch Agents/Daemons |
| Windows  | ✅      | SCM            | Windows Services |
| Linux    | ❌      | systemd (TODO) | Planned for future |

## Related Files

- `service.go`: Interface and factory
- `service_darwin.go`: macOS implementation
- `service_windows.go`: Windows implementation
- `privilege_unix.go`: Unix privilege checking
- `privilege_windows.go`: Windows privilege checking
- `service_stub.go`: Stub for unsupported platforms
