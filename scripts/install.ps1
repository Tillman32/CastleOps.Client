#
# CastleOps Client Installation Script for Windows
#
# This script installs the CastleOps client as a Windows Service.
# It handles binary installation, configuration setup, and service registration.
#
# Run with Administrator privileges:
#   PowerShell -ExecutionPolicy Bypass -File install.ps1
#

#Requires -RunAsAdministrator

# Configuration
$AppName = "castleops-client"
$ServiceName = "CastleOpsClient"
$ServiceDisplayName = "CastleOps Client"
$ServiceDescription = "CastleOps desktop agent for system monitoring and management"

$InstallDir = "$env:ProgramFiles\CastleOps"
$ConfigDir = "$env:LOCALAPPDATA\CastleOps"
$LogDir = "$env:LOCALAPPDATA\CastleOps\Logs"
$DataDir = "$env:LOCALAPPDATA\CastleOps"

# Color output functions
function Write-Info {
    param([string]$Message)
    Write-Host "[INFO] $Message" -ForegroundColor Cyan
}

function Write-Success {
    param([string]$Message)
    Write-Host "[SUCCESS] $Message" -ForegroundColor Green
}

function Write-Error-Message {
    param([string]$Message)
    Write-Host "[ERROR] $Message" -ForegroundColor Red
}

function Write-Warning-Message {
    param([string]$Message)
    Write-Host "[WARNING] $Message" -ForegroundColor Yellow
}

# Check if running as Administrator
function Test-Administrator {
    $currentUser = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($currentUser)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

# Create necessary directories
function New-Directories {
    Write-Info "Creating directories..."

    $directories = @($InstallDir, $ConfigDir, $LogDir, $DataDir)
    foreach ($dir in $directories) {
        if (-not (Test-Path $dir)) {
            New-Item -ItemType Directory -Path $dir -Force | Out-Null
            Write-Info "Created directory: $dir"
        }
    }

    Write-Success "Directories created"
}

# Install binary
function Install-Binary {
    param([string]$BinaryPath)

    Write-Info "Installing binary to $InstallDir..."

    if (-not (Test-Path $BinaryPath)) {
        Write-Error-Message "Binary not found at: $BinaryPath"
        exit 1
    }

    $destinationPath = Join-Path $InstallDir "$AppName.exe"
    Copy-Item -Path $BinaryPath -Destination $destinationPath -Force

    Write-Success "Binary installed to: $destinationPath"
    return $destinationPath
}

# Create default configuration
function New-Configuration {
    $configFile = Join-Path $ConfigDir "config.yaml"

    if (Test-Path $configFile) {
        Write-Warning-Message "Configuration file already exists, skipping"
        return
    }

    Write-Info "Creating default configuration..."

    $configContent = @"
# CastleOps Client Configuration
server:
  url: "https://api.castleops.com"
  tls_verify: true
  timeout: 30s

client:
  id: ""
  token: ""

heartbeat:
  interval: 30s
  retry_attempts: 3

metrics:
  collection_interval: 60s
  batch_size: 100
  retention_days: 7

cache:
  type: "sqlite"
  path: "$($DataDir -replace '\\', '\\')\data.db"

logging:
  level: "info"
  path: "$($LogDir -replace '\\', '\\')\client.log"
  format: "json"

package_managers:
  preferred: "auto"
"@

    Set-Content -Path $configFile -Value $configContent -Encoding UTF8

    Write-Success "Configuration created at: $configFile"
}

# Uninstall existing service if present
function Uninstall-ExistingService {
    param([string]$BinaryPath)

    Write-Info "Checking for existing service..."

    $service = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
    if ($service) {
        Write-Info "Uninstalling existing service..."

        $configFile = Join-Path $ConfigDir "config.yaml"

        # Use the binary's built-in uninstall command
        & $BinaryPath -config $configFile -uninstall

        # Wait for service to be fully removed
        Start-Sleep -Seconds 2
        Write-Success "Existing service uninstalled"
    }
}

# Install Windows Service using the binary's built-in installer
function Install-Service {
    param([string]$BinaryPath)

    Write-Info "Installing Windows Service..."

    $configFile = Join-Path $ConfigDir "config.yaml"

    # Use the binary's built-in install command
    # This must be run as administrator
    & $BinaryPath -config $configFile -install

    if ($LASTEXITCODE -ne 0) {
        Write-Error-Message "Failed to install service"
        exit 1
    }

    Write-Success "Service installed"
}

# Start the service
function Start-ServiceWrapper {
    Write-Info "Starting service..."

    Start-Service -Name $ServiceName -ErrorAction Stop

    # Wait a moment and check status
    Start-Sleep -Seconds 2
    $service = Get-Service -Name $ServiceName

    if ($service.Status -eq 'Running') {
        Write-Success "Service started successfully"
    } else {
        Write-Warning-Message "Service status: $($service.Status)"
    }
}

# Check service status
function Get-ServiceStatus {
    Write-Info "Checking service status..."

    $service = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
    if ($service) {
        Write-Success "Service Status: $($service.Status)"
    } else {
        Write-Warning-Message "Service not found"
    }
}

# Main installation flow
function Main {
    param([string]$BinaryPath)

    Write-Host ""
    Write-Host "========================================" -ForegroundColor Cyan
    Write-Host "  CastleOps Client Installation" -ForegroundColor Cyan
    Write-Host "========================================" -ForegroundColor Cyan
    Write-Host ""

    if (-not (Test-Administrator)) {
        Write-Error-Message "This script must be run as Administrator"
        exit 1
    }

    # Default binary path if not provided
    if (-not $BinaryPath) {
        $BinaryPath = ".\build\$AppName.exe"
    }

    try {
        New-Directories
        $installedBinaryPath = Install-Binary -BinaryPath $BinaryPath
        New-Configuration
        Uninstall-ExistingService -BinaryPath $installedBinaryPath
        Install-Service -BinaryPath $installedBinaryPath
        Start-ServiceWrapper
        Get-ServiceStatus

        Write-Host ""
        Write-Host "========================================" -ForegroundColor Green
        Write-Host "  Installation Complete!" -ForegroundColor Green
        Write-Host "========================================" -ForegroundColor Green
        Write-Host ""
        Write-Host "Configuration: $(Join-Path $ConfigDir 'config.yaml')"
        Write-Host "Logs:          $LogDir"
        Write-Host ""
        Write-Host "Service commands:"
        Write-Host "  Start:     Start-Service $ServiceName"
        Write-Host "  Stop:      Stop-Service $ServiceName"
        Write-Host "  Status:    Get-Service $ServiceName"
        Write-Host "  Restart:   Restart-Service $ServiceName"
        Write-Host "  Uninstall: & '$installedBinaryPath' -config '$(Join-Path $ConfigDir 'config.yaml')' -uninstall"
        Write-Host ""
        Write-Host "View logs with:"
        Write-Host "  Get-Content '$LogDir\client.log' -Tail 50 -Wait"
        Write-Host ""
    }
    catch {
        Write-Error-Message "Installation failed: $_"
        exit 1
    }
}

# Run main function with command-line arguments
$binaryArg = $args[0]
Main -BinaryPath $binaryArg
