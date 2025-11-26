#!/usr/bin/env bash
#
# CastleOps Client Uninstallation Script for macOS
#
# This script uninstalls the CastleOps client service and removes all files.
#

set -e  # Exit on error
set -u  # Exit on undefined variable

# Color codes for output
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[0;33m'
readonly CYAN='\033[0;36m'
readonly NC='\033[0m' # No Color

# Installation paths
readonly APP_NAME="castleops-client"
readonly INSTALL_DIR="/usr/local/bin"
readonly CONFIG_DIR="${HOME}/Library/Application Support/CastleOps"
readonly LOG_DIR="${HOME}/Library/Logs/CastleOps"
readonly BINARY_PATH="${INSTALL_DIR}/${APP_NAME}"

# Check if running on macOS
check_platform() {
    if [[ "$(uname -s)" != "Darwin" ]]; then
        echo -e "${RED}Error: This script is for macOS only${NC}"
        exit 1
    fi
}

# Print colored message
print_info() {
    echo -e "${CYAN}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# Uninstall service using the binary
uninstall_service() {
    print_info "Uninstalling service..."

    if [[ ! -f "${BINARY_PATH}" ]]; then
        print_warning "Binary not found, skipping service uninstall"
        return
    fi

    # Try to uninstall using the binary (requires sudo)
    if sudo "${BINARY_PATH}" -config "${CONFIG_DIR}/config.yaml" -uninstall 2>/dev/null; then
        print_success "Service uninstalled successfully"
    else
        print_warning "Service uninstall command failed (may not be installed)"
    fi
}

# Remove binary
remove_binary() {
    print_info "Removing binary..."

    if [[ -f "${BINARY_PATH}" ]]; then
        sudo rm -f "${BINARY_PATH}"
        print_success "Binary removed"
    else
        print_warning "Binary not found, skipping"
    fi
}

# Remove configuration and data
remove_config() {
    print_info "Removing configuration and data..."

    if [[ -d "${CONFIG_DIR}" ]]; then
        rm -rf "${CONFIG_DIR}"
        print_success "Configuration removed"
    else
        print_warning "Configuration directory not found, skipping"
    fi
}

# Remove logs
remove_logs() {
    print_info "Removing logs..."

    if [[ -d "${LOG_DIR}" ]]; then
        rm -rf "${LOG_DIR}"
        print_success "Logs removed"
    else
        print_warning "Log directory not found, skipping"
    fi
}

# Main uninstallation flow
main() {
    echo -e "${CYAN}"
    echo "========================================"
    echo "  CastleOps Client Uninstallation"
    echo "========================================"
    echo -e "${NC}"

    check_platform

    # Confirm uninstallation
    read -p "Are you sure you want to uninstall CastleOps Client? (y/N) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Uninstallation cancelled"
        exit 0
    fi

    uninstall_service
    remove_binary
    remove_config
    remove_logs

    echo ""
    echo -e "${GREEN}========================================"
    echo "  Uninstallation Complete!"
    echo "========================================${NC}"
    echo ""
}

# Run main function
main "$@"
