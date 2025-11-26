# CastleOps Client - Service Installation and Usage Guide

This guide covers installation, configuration, and management of the CastleOps Client as a system service on macOS and Windows.

## Quick Start

### macOS Installation

```bash
# Build the binary
make build-darwin

# Install as system service (requires sudo)
sudo ./build/castleops-client -install

# Or use the installation script
./scripts/install.sh ./build/castleops-client
```

### Windows Installation

```powershell
# Build the binary
make build-windows

# Install as Windows Service (requires Administrator)
.\build\castleops-client.exe -install

# Or use the installation script
.\scripts\install.ps1 .\build\castleops-client.exe
```

## Command-Line Flags

```
-config string
    Path to configuration file
    Default: Platform-specific (see Configuration section)

-version
    Print version and exit

-install
    Install as system service (requires elevated privileges)

-uninstall
    Uninstall system service (requires elevated privileges)

-service
    Run in service mode (used internally by service manager)
```

## Installation

### macOS (launchd)

#### Manual Installation

```bash
# Install the service
sudo castleops-client -config /path/to/config.yaml -install

# The service will:
# - Create a plist file in /Library/LaunchDaemons/com.castleops.client.plist
# - Load the service using launchctl
# - Auto-start the service
# - Configure auto-start on boot
```

#### Script Installation

```bash
# Run the installation script
./scripts/install.sh ./build/castleops-client

# This will:
# - Copy binary to /usr/local/bin/
# - Create config directory: ~/Library/Application Support/CastleOps/
# - Create log directory: ~/Library/Logs/CastleOps/
# - Create default configuration
# - Install and start the service
```

#### Installation Locations

```
Binary:        /usr/local/bin/castleops-client
Config:        ~/Library/Application Support/CastleOps/config.yaml
Data/Cache:    ~/Library/Application Support/CastleOps/data.db
Logs:          ~/Library/Logs/CastleOps/
Plist:         ~/Library/LaunchAgents/com.castleops.client.plist (user)
               /Library/LaunchDaemons/com.castleops.client.plist (system)
```

### Windows (Windows Service)

#### Manual Installation

```powershell
# Install the service (run as Administrator)
.\castleops-client.exe -config C:\path\to\config.yaml -install

# The service will:
# - Register with Service Control Manager
# - Configure for automatic startup
# - Set recovery actions (restart on failure)
# - Run as NT AUTHORITY\LocalService
```

#### Script Installation

```powershell
# Run the installation script (as Administrator)
.\scripts\install.ps1 .\build\castleops-client.exe

# This will:
# - Copy binary to C:\Program Files\CastleOps\
# - Create config directory: %LOCALAPPDATA%\CastleOps\
# - Create log directory: %LOCALAPPDATA%\CastleOps\Logs\
# - Create default configuration
# - Install and start the service
```

#### Installation Locations

```
Binary:     C:\Program Files\CastleOps\castleops-client.exe
Config:     %LOCALAPPDATA%\CastleOps\config.yaml
Data/Cache: %LOCALAPPDATA%\CastleOps\data.db
Logs:       %LOCALAPPDATA%\CastleOps\Logs\
```

## Configuration

### Default Configuration Locations

- **macOS**: `~/Library/Application Support/CastleOps/config.yaml`
- **Windows**: `%LOCALAPPDATA%\CastleOps\config.yaml`
- **Linux**: `/etc/castleops/config.yaml`

### Sample Configuration

```yaml
server:
  url: "https://api.castleops.com"
  tls_verify: true
  timeout: 30s

client:
  id: ""  # Auto-generated on first registration
  token: ""  # Obtained from server on registration

heartbeat:
  interval: 30s
  retry_attempts: 3

metrics:
  collection_interval: 60s
  batch_size: 100
  retention_days: 7

cache:
  type: "sqlite"
  path: "~/Library/Application Support/CastleOps/data.db"  # macOS
  # path: "%LOCALAPPDATA%\CastleOps\data.db"  # Windows

logging:
  level: "info"  # debug, info, warn, error
  format: "json"  # json or console
  path: "~/Library/Logs/CastleOps/client.log"  # macOS
  # path: "%LOCALAPPDATA%\CastleOps\Logs\client.log"  # Windows

package_managers:
  preferred: "auto"  # auto, homebrew (macOS), chocolatey (Windows)
```

## Service Management

### macOS (launchd)

```bash
# Start service
launchctl start com.castleops.client

# Stop service
launchctl stop com.castleops.client

# Check status
launchctl list | grep castleops

# View detailed status
launchctl list com.castleops.client

# Restart service
launchctl stop com.castleops.client && launchctl start com.castleops.client

# Uninstall service
sudo castleops-client -uninstall

# Or use the uninstall script
./scripts/uninstall.sh
```

### Windows (Service Control Manager)

```powershell
# Start service
Start-Service CastleOpsClient

# Stop service
Stop-Service CastleOpsClient

# Check status
Get-Service CastleOpsClient

# Restart service
Restart-Service CastleOpsClient

# View service details
Get-Service CastleOpsClient | Format-List *

# Uninstall service (as Administrator)
.\castleops-client.exe -uninstall

# Or use the uninstall script
.\scripts\uninstall.ps1
```

## Running in Foreground (Development/Testing)

For development or testing, you can run the client in foreground mode without installing as a service:

```bash
# macOS/Linux
./castleops-client -config config.yaml

# Windows
.\castleops-client.exe -config config.yaml
```

Press Ctrl+C to gracefully shutdown.

## Logging

### macOS

Service logs are written to multiple locations:

```bash
# Application logs (structured logging)
tail -f ~/Library/Logs/CastleOps/client.log

# Service stdout
tail -f ~/Library/Logs/CastleOps/stdout.log

# Service stderr
tail -f ~/Library/Logs/CastleOps/stderr.log

# System log (for launchd events)
log stream --predicate 'subsystem == "com.castleops.client"'
```

### Windows

Service logs are written to:

```powershell
# Application logs (structured logging)
Get-Content "$env:LOCALAPPDATA\CastleOps\Logs\client.log" -Tail 50 -Wait

# Windows Event Log
Get-EventLog -LogName Application -Source CastleOpsClient -Newest 50

# Or use Event Viewer
eventvwr.msc
# Navigate to: Windows Logs > Application > Filter by CastleOpsClient
```

## Troubleshooting

### Service Won't Start (macOS)

```bash
# Check if plist file exists
ls -la ~/Library/LaunchAgents/com.castleops.client.plist

# Check plist syntax
plutil -lint ~/Library/LaunchAgents/com.castleops.client.plist

# Check service status
launchctl list com.castleops.client

# View recent errors
tail -100 ~/Library/Logs/CastleOps/stderr.log

# Unload and reload
launchctl unload ~/Library/LaunchAgents/com.castleops.client.plist
launchctl load ~/Library/LaunchAgents/com.castleops.client.plist
```

### Service Won't Start (Windows)

```powershell
# Check service status
Get-Service CastleOpsClient | Format-List *

# View recent errors
Get-EventLog -LogName Application -Source CastleOpsClient -Newest 10

# Check if binary exists
Test-Path "C:\Program Files\CastleOps\castleops-client.exe"

# Try starting manually to see errors
& "C:\Program Files\CastleOps\castleops-client.exe" -config "$env:LOCALAPPDATA\CastleOps\config.yaml"
```

### Permission Errors

```bash
# macOS: Ensure running with sudo for service operations
sudo castleops-client -install

# Windows: Ensure running as Administrator
# Right-click PowerShell > Run as Administrator
```

### Configuration Issues

```bash
# Validate configuration file exists
ls -la ~/Library/Application\ Support/CastleOps/config.yaml  # macOS
dir %LOCALAPPDATA%\CastleOps\config.yaml  # Windows

# Test configuration in foreground
./castleops-client -config /path/to/config.yaml  # Test before installing
```

## Uninstallation

### macOS

```bash
# Method 1: Use the binary
sudo castleops-client -uninstall

# Method 2: Use the uninstall script (recommended)
./scripts/uninstall.sh

# This will:
# - Stop the service
# - Unload the service
# - Remove the plist file
# - Remove the binary
# - Remove configuration and data
# - Remove logs
```

### Windows

```powershell
# Method 1: Use the binary (run as Administrator)
.\castleops-client.exe -uninstall

# Method 2: Use the uninstall script (recommended)
.\scripts\uninstall.ps1

# This will:
# - Stop the service
# - Unregister from Service Control Manager
# - Remove event log source
# - Remove the binary
# - Remove configuration and data
# - Remove logs
```

## Advanced Usage

### Running as User-Level Service (macOS)

By default, the service installs as a system-level daemon. To install as a user-level agent:

```bash
# Modify the service configuration in code:
# UserService: true

# This will:
# - Install to ~/Library/LaunchAgents/ instead of /Library/LaunchDaemons/
# - Run under your user account
# - Start when you log in (not at boot)
```

### Custom Recovery Actions (Windows)

The Windows service is configured with automatic recovery:

```
Failure 1: Restart service after 60 seconds
Failure 2: Restart service after 60 seconds
Failure 3: Restart service after 60 seconds
Reset failure count: After 24 hours
```

To customize, use sc.exe:

```powershell
# View current failure actions
sc.exe qfailure CastleOpsClient

# Modify failure actions
sc.exe failure CastleOpsClient reset= 86400 actions= restart/30000/restart/60000/restart/120000
```

### Monitoring Service Health

```bash
# macOS: Check if service is running
if launchctl list | grep -q "com.castleops.client"; then
    echo "Service is running"
else
    echo "Service is not running"
fi

# Windows: Check if service is running
$service = Get-Service CastleOpsClient
if ($service.Status -eq 'Running') {
    Write-Host "Service is running"
} else {
    Write-Host "Service is not running"
}
```

## Security Considerations

1. **Configuration File**: Contains sensitive data (API tokens)
   - macOS: Set permissions to 600 (owner read/write only)
   - Windows: Use NTFS permissions to restrict access

2. **Service Account**:
   - macOS: Runs as root (system daemon) or user (user agent)
   - Windows: Runs as NT AUTHORITY\LocalService (limited privileges)

3. **TLS Verification**: Always keep `tls_verify: true` in production

4. **Logging**: Logs may contain sensitive information
   - Review log retention policies
   - Ensure proper log file permissions

## Performance Tuning

### Resource Limits

```yaml
# Adjust collection interval for performance
metrics:
  collection_interval: 120s  # Increase to reduce CPU usage

# Reduce batch size to lower memory usage
metrics:
  batch_size: 50  # Smaller batches = more frequent uploads

# Adjust heartbeat interval
heartbeat:
  interval: 60s  # Increase to reduce network traffic
```

### Log Rotation

```bash
# macOS: Use newsyslog
# Add to /etc/newsyslog.conf:
# ~/Library/Logs/CastleOps/*.log 644 7 1000 * J

# Windows: Use built-in log rotation or external tools
```

## Integration with CI/CD

### GitHub Actions Example

```yaml
name: Install CastleOps Client

on: [push]

jobs:
  install-macos:
    runs-on: macos-latest
    steps:
      - uses: actions/checkout@v2
      - name: Install service
        run: |
          sudo ./scripts/install.sh ./castleops-client
```

## Support

For issues, questions, or feature requests:
- GitHub Issues: https://github.com/castleops/client/issues
- Documentation: https://docs.castleops.com
- Email: support@castleops.com
