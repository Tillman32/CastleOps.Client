#!/usr/bin/env bash
#
# CastleOps Client Installation Script for macOS
#
# This script installs the CastleOps client as a launchd service on macOS.
# It handles binary installation, configuration setup, and service registration.
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
readonly PLIST_DIR="${HOME}/Library/LaunchAgents"
readonly PLIST_NAME="com.castleops.client.plist"

# Check if running on macOS
check_platform() {
    if [[ "$(uname -s)" != "Darwin" ]]; then
        echo -e "${RED}Error: This script is for macOS only${NC}"
        exit 1
    fi
}

# Check if running with appropriate permissions
check_permissions() {
    if [[ $EUID -eq 0 ]]; then
        echo -e "${YELLOW}Warning: This script should not be run as root${NC}"
        echo -e "${YELLOW}It will install to user directories and prompt for sudo when needed${NC}"
        read -p "Continue anyway? (y/N) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            exit 1
        fi
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

# Create necessary directories
create_directories() {
    print_info "Creating directories..."

    mkdir -p "${CONFIG_DIR}"
    mkdir -p "${LOG_DIR}"
    mkdir -p "${PLIST_DIR}"

    print_success "Directories created"
}

# Install binary
install_binary() {
    local binary_path="$1"

    print_info "Installing binary to ${INSTALL_DIR}..."

    if [[ ! -f "${binary_path}" ]]; then
        print_error "Binary not found at: ${binary_path}"
        exit 1
    fi

    # Copy binary to install directory (requires sudo)
    sudo cp "${binary_path}" "${INSTALL_DIR}/${APP_NAME}"
    sudo chmod +x "${INSTALL_DIR}/${APP_NAME}"

    print_success "Binary installed"
}

# Create default configuration if it doesn't exist
create_config() {
    local config_file="${CONFIG_DIR}/config.yaml"

    if [[ -f "${config_file}" ]]; then
        print_warning "Configuration file already exists, skipping"
        return
    fi

    print_info "Creating default configuration..."

    cat > "${config_file}" <<EOF
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
  path: "${CONFIG_DIR}/data.db"

logging:
  level: "info"
  path: "${LOG_DIR}/client.log"
  format: "json"

package_managers:
  preferred: "auto"
EOF

    print_success "Configuration created at: ${config_file}"
}

# Install service using the binary's built-in service installer
install_service() {
    print_info "Installing service..."

    # Use sudo to install the service (requires elevated privileges)
    if sudo "${INSTALL_DIR}/${APP_NAME}" -config "${CONFIG_DIR}/config.yaml" -install; then
        print_success "Service installed successfully"
    else
        print_error "Failed to install service"
        return 1
    fi
}

# Start the installed service
start_service() {
    print_info "Starting service..."

    # On macOS, the service auto-starts on installation
    # Just verify it's running
    sleep 2
    if launchctl list | grep -q "com.castleops.client"; then
        print_success "Service is running"
    else
        print_warning "Service may not be running, check logs"
    fi
}

# Check service status
check_service_status() {
    print_info "Checking service status..."

    if launchctl list | grep -q "com.castleops.client"; then
        print_success "Service is running"
    else
        print_warning "Service is not running"
    fi
}

# Main installation flow
main() {
    echo -e "${CYAN}"
    echo "========================================"
    echo "  CastleOps Client Installation"
    echo "========================================"
    echo -e "${NC}"

    check_platform
    check_permissions

    # Get binary path from argument or use default
    local binary_path="${1:-./build/${APP_NAME}}"

    create_directories
    install_binary "${binary_path}"
    create_config
    install_service
    start_service
    check_service_status

    echo ""
    echo -e "${GREEN}========================================"
    echo "  Installation Complete!"
    echo "========================================${NC}"
    echo ""
    echo "Configuration: ${CONFIG_DIR}/config.yaml"
    echo "Logs:          ${LOG_DIR}/"
    echo ""
    echo "Service commands:"
    echo "  Start:     launchctl start com.castleops.client"
    echo "  Stop:      launchctl stop com.castleops.client"
    echo "  Status:    launchctl list | grep castleops"
    echo "  Uninstall: sudo ${INSTALL_DIR}/${APP_NAME} -uninstall"
    echo ""
    echo "View logs:"
    echo "  tail -f ${LOG_DIR}/client.log"
    echo "  tail -f ${LOG_DIR}/stdout.log"
    echo "  tail -f ${LOG_DIR}/stderr.log"
    echo ""
}

# Run main function
main "$@"
