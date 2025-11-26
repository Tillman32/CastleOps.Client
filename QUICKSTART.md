# CastleOps Client - Quick Start Guide

## Installation

### macOS

```bash
# 1. Build the binary
make build-darwin
# or
go build -o castleops-client ./cmd/castleops-client

# 2. Install as service
./scripts/install.sh ./castleops-client

# 3. Verify it's running
launchctl list | grep castleops
```

### Windows

```powershell
# 1. Build the binary
make build-windows
# or
go build -o castleops-client.exe ./cmd/castleops-client

# 2. Install as service (run PowerShell as Administrator)
.\scripts\install.ps1 .\castleops-client.exe

# 3. Verify it's running
Get-Service CastleOpsClient
```

## Service Management

### macOS

```bash
# Start
launchctl start com.castleops.client

# Stop
launchctl stop com.castleops.client

# Status
launchctl list com.castleops.client

# View logs
tail -f ~/Library/Logs/CastleOps/client.log

# Uninstall
./scripts/uninstall.sh
```

### Windows

```powershell
# Start
Start-Service CastleOpsClient

# Stop
Stop-Service CastleOpsClient

# Status
Get-Service CastleOpsClient

# View logs
Get-Content "$env:LOCALAPPDATA\CastleOps\Logs\client.log" -Tail 50 -Wait

# Uninstall
.\scripts\uninstall.ps1
```

## Configuration

### macOS
Edit: `~/Library/Application Support/CastleOps/config.yaml`

### Windows
Edit: `%LOCALAPPDATA%\CastleOps\config.yaml`

### Key Settings

```yaml
server:
  url: "https://api.castleops.com"

heartbeat:
  interval: 30s

metrics:
  collection_interval: 60s

logging:
  level: "info"  # debug, info, warn, error
```

## Development Mode

Run without installing as service:

```bash
# macOS/Linux
./castleops-client -config config.yaml

# Windows
.\castleops-client.exe -config config.yaml
```

Press Ctrl+C to stop.

## Troubleshooting

### Service won't start?

**macOS:**
```bash
# Check logs
tail -100 ~/Library/Logs/CastleOps/stderr.log

# Reload service
launchctl unload ~/Library/LaunchAgents/com.castleops.client.plist
launchctl load ~/Library/LaunchAgents/com.castleops.client.plist
```

**Windows:**
```powershell
# Check event log
Get-EventLog -LogName Application -Source CastleOpsClient -Newest 10

# Try manual start to see errors
& "C:\Program Files\CastleOps\castleops-client.exe" -config "$env:LOCALAPPDATA\CastleOps\config.yaml"
```

### Permission denied?

**macOS:** Use `sudo` for install/uninstall
```bash
sudo ./scripts/install.sh ./castleops-client
```

**Windows:** Run PowerShell as Administrator
```
Right-click PowerShell → Run as Administrator
```

## Next Steps

- Read [USAGE.md](USAGE.md) for detailed documentation
- Configure your server URL in config.yaml
- Monitor logs for successful registration
- Verify metrics collection is working

## Support

- Documentation: [USAGE.md](USAGE.md)
- Service Details: [internal/service/README.md](internal/service/README.md)
- Implementation Summary: [SERVICE_INTEGRATION_SUMMARY.md](SERVICE_INTEGRATION_SUMMARY.md)
