#!/bin/bash
#
# MySoc Updater Installation Script
# Usage: curl -sSL https://updates.mysoc.ai/install.sh | sudo bash
#

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Configuration
UPDATER_URL="${UPDATER_URL:-https://updates.mysoc.ai}"
INSTALL_DIR="/usr/local/bin"
UPDATER_BINARY="mysoc-updater"

# Functions
print_banner() {
    echo -e "${CYAN}"
    echo "╔════════════════════════════════════════════════════════════╗"
    echo "║           MySoc Updater Installation                       ║"
    echo "╚════════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
}

log_info() {
    echo -e "${GREEN}→${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}⚠${NC} $1"
}

log_error() {
    echo -e "${RED}✗${NC} $1"
}

log_success() {
    echo -e "${GREEN}✓${NC} $1"
}

# Check if running as root
check_root() {
    if [ "$(id -u)" != "0" ]; then
        log_error "This script must be run as root"
        echo "Usage: curl -sSL ${UPDATER_URL}/install.sh | sudo bash"
        exit 1
    fi
}

# Detect OS and architecture
detect_system() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)
    
    case $ARCH in
        x86_64)
            ARCH="amd64"
            ;;
        aarch64|arm64)
            ARCH="arm64"
            ;;
        *)
            log_error "Unsupported architecture: $ARCH"
            exit 1
            ;;
    esac
    
    case $OS in
        linux)
            OS="linux"
            ;;
        darwin)
            OS="darwin"
            ;;
        *)
            log_error "Unsupported operating system: $OS"
            exit 1
            ;;
    esac
    
    log_info "Detected: ${OS}/${ARCH}"
}

# Check for required commands
check_dependencies() {
    for cmd in curl sha256sum systemctl; do
        if ! command -v $cmd &> /dev/null; then
            log_warn "Command '$cmd' not found. Some features may not work."
        fi
    done
}

# Download the updater binary
download_updater() {
    local download_url="${UPDATER_URL}/releases/mysoc-updater/latest/mysoc-updater-${OS}-${ARCH}"
    local checksum_url="${UPDATER_URL}/releases/mysoc-updater/latest/mysoc-updater-${OS}-${ARCH}.sha256"
    
    log_info "Downloading mysoc-updater..."
    
    # Download binary
    if ! curl -sSL -o /tmp/${UPDATER_BINARY} "${download_url}"; then
        log_error "Failed to download mysoc-updater"
        exit 1
    fi
    
    # Download and verify checksum (if available)
    if curl -sSL -o /tmp/${UPDATER_BINARY}.sha256 "${checksum_url}" 2>/dev/null; then
        log_info "Verifying checksum..."
        cd /tmp
        if ! sha256sum -c ${UPDATER_BINARY}.sha256 &>/dev/null; then
            log_error "Checksum verification failed"
            exit 1
        fi
        log_success "Checksum verified"
    else
        log_warn "Checksum not available, skipping verification"
    fi
}

# Install the binary
install_binary() {
    log_info "Installing to ${INSTALL_DIR}/${UPDATER_BINARY}..."
    
    # Make executable
    chmod +x /tmp/${UPDATER_BINARY}
    
    # Move to install directory
    mv /tmp/${UPDATER_BINARY} ${INSTALL_DIR}/${UPDATER_BINARY}
    
    # Verify installation
    if ! ${INSTALL_DIR}/${UPDATER_BINARY} version &>/dev/null; then
        log_error "Installation verification failed"
        exit 1
    fi
    
    local version=$(${INSTALL_DIR}/${UPDATER_BINARY} version 2>&1 | head -1)
    log_success "Installed: $version"
}

# Create log directory
create_directories() {
    log_info "Creating directories..."
    mkdir -p /var/log/mysoc-updater
    log_success "Directories created"
}

# Print next steps
print_next_steps() {
    echo ""
    echo -e "${GREEN}╔════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║  ✓ Installation complete!                                   ║${NC}"
    echo -e "${GREEN}╚════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo "Next steps:"
    echo ""
    echo -e "  ${CYAN}1.${NC} Initialize with your license key:"
    echo ""
    echo -e "     ${YELLOW}sudo mysoc-updater init --license YOUR-LICENSE-KEY${NC}"
    echo ""
    echo -e "  ${CYAN}2.${NC} Check status:"
    echo ""
    echo -e "     ${YELLOW}mysoc-updater status${NC}"
    echo ""
    echo -e "  ${CYAN}3.${NC} View logs:"
    echo ""
    echo -e "     ${YELLOW}journalctl -u mysoc-updater -f${NC}"
    echo ""
    echo "For more information, visit: https://docs.mysoc.ai/updater"
    echo ""
}

# Main installation flow
main() {
    print_banner
    check_root
    detect_system
    check_dependencies
    download_updater
    install_binary
    create_directories
    print_next_steps
}

# Run main
main

