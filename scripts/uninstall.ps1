#
# CastleOps Client Uninstallation Script for Windows
#
# This script uninstalls the CastleOps client Windows Service and removes all files.
#
# Run with Administrator privileges:
#   PowerShell -ExecutionPolicy Bypass -File uninstall.ps1
#

#Requires -RunAsAdministrator

# Configuration
$AppName = "castleops-client"
$ServiceName = "CastleOpsClient"

$InstallDir = "$env:ProgramFiles\CastleOps"
$ConfigDir = "$env:LOCALAPPDATA\CastleOps"
$LogDir = "$env:LOCALAPPDATA\CastleOps\Logs"
$DataDir = "$env:LOCALAPPDATA\CastleOps"
$BinaryPath = Join-Path $InstallDir "$AppName.exe"

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

# Uninstall Windows Service
function Uninstall-Service {
    Write-Info "Uninstalling Windows Service..."

    if (-not (Test-Path $BinaryPath)) {
        Write-Warning-Message "Binary not found, skipping service uninstall"
        return
    }

    $configFile = Join-Path $ConfigDir "config.yaml"

    # Use the binary's built-in uninstall command
    try {
        & $BinaryPath -config $configFile -uninstall
        Write-Success "Service uninstalled successfully"
    }
    catch {
        Write-Warning-Message "Service uninstall command failed (may not be installed)"
    }
}

# Remove binary and installation directory
function Remove-Binary {
    Write-Info "Removing binary and installation directory..."

    if (Test-Path $InstallDir) {
        Remove-Item -Path $InstallDir -Recurse -Force
        Write-Success "Binary removed"
    }
    else {
        Write-Warning-Message "Installation directory not found, skipping"
    }
}

# Remove configuration and data
function Remove-Configuration {
    Write-Info "Removing configuration and data..."

    if (Test-Path $ConfigDir) {
        Remove-Item -Path $ConfigDir -Recurse -Force
        Write-Success "Configuration removed"
    }
    else {
        Write-Warning-Message "Configuration directory not found, skipping"
    }
}

# Remove logs
function Remove-Logs {
    Write-Info "Removing logs..."

    if (Test-Path $LogDir) {
        Remove-Item -Path $LogDir -Recurse -Force
        Write-Success "Logs removed"
    }
    else {
        Write-Warning-Message "Log directory not found, skipping"
    }
}

# Main uninstallation flow
function Main {
    Write-Host ""
    Write-Host "========================================" -ForegroundColor Cyan
    Write-Host "  CastleOps Client Uninstallation" -ForegroundColor Cyan
    Write-Host "========================================" -ForegroundColor Cyan
    Write-Host ""

    if (-not (Test-Administrator)) {
        Write-Error-Message "This script must be run as Administrator"
        exit 1
    }

    # Confirm uninstallation
    $confirmation = Read-Host "Are you sure you want to uninstall CastleOps Client? (y/N)"
    if ($confirmation -ne 'y' -and $confirmation -ne 'Y') {
        Write-Host "Uninstallation cancelled"
        exit 0
    }

    try {
        Uninstall-Service
        Remove-Binary
        Remove-Configuration
        Remove-Logs

        Write-Host ""
        Write-Host "========================================" -ForegroundColor Green
        Write-Host "  Uninstallation Complete!" -ForegroundColor Green
        Write-Host "========================================" -ForegroundColor Green
        Write-Host ""
    }
    catch {
        Write-Error-Message "Uninstallation failed: $_"
        exit 1
    }
}

# Run main function
Main
