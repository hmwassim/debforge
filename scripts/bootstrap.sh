#!/usr/bin/env bash
# bootstrap.sh - Download and install DebForge from GitHub
# This script downloads the latest release and sets up DebForge
#
#
# Usage: curl -fsSL https://github.com/.../bootstrap.sh | bash
# Or:    wget -O- https://github.com/.../bootstrap.sh | bash

set -euo pipefail

# Configuration
GITHUB_REPO="${GITHUB_REPO:-hmwassim/debforge}"
INSTALL_DIR="${INSTALL_DIR:-/opt/debforge}"
BIN_LINK="/usr/local/bin/debforge"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}ℹ $*${NC}"
}

log_success() {
    echo -e "${GREEN}✓ $*${NC}"
}

log_warn() {
    echo -e "${YELLOW}⚠ $*${NC}"
}

log_error() {
    echo -e "${RED}✗ $*${NC}" >&2
}

# Check if running as root (we need sudo for system-wide install)
check_root() {
    if [[ $EUID -eq 0 ]]; then
        log_error "This script should NOT be run as root"
        log_info "The script will use sudo internally where needed"
        exit 1
    fi
}

# Check requirements
check_requirements() {
    log_info "Checking requirements..."

    local missing=()

    if ! command -v curl &>/dev/null && ! command -v wget &>/dev/null; then
        missing+=("curl or wget")
    fi

    if ! command -v jq &>/dev/null; then
        missing+=("jq")
    fi

    if ! command -v sudo &>/dev/null; then
        missing+=("sudo")
    fi

    if [[ ${#missing[@]} -gt 0 ]]; then
        log_error "Missing required tools: ${missing[*]}"
        log_info "Install with: sudo apt install ${missing[*]}"
        exit 1
    fi

    log_success "All requirements met"
}

# Get latest release info from GitHub
get_latest_release() {
    log_info "Fetching latest release from GitHub..."

    local api_url="https://api.github.com/repos/$GITHUB_REPO/releases/latest"
    local release_info

    if command -v curl &>/dev/null; then
        release_info=$(curl -fsSL "$api_url" 2>/dev/null || echo "")
    else
        release_info=$(wget -qO- "$api_url" 2>/dev/null || echo "")
    fi

    if [[ -z "$release_info" ]]; then
        log_error "Failed to fetch release info"
        exit 1
    fi

    LATEST_VERSION=$(echo "$release_info" | jq -r '.tag_name')
    
    # Try to get source tarball URL first (more reliable than tarball_url)
    LATEST_TARBALL=$(echo "$release_info" | jq -r '.tarball_url')
    
    # Fallback to zipball if tarball fails
    if [[ "$LATEST_TARBALL" == "null" ]] || [[ -z "$LATEST_TARBALL" ]]; then
        LATEST_TARBALL=$(echo "$release_info" | jq -r '.zipball_url')
    fi

    if [[ "$LATEST_VERSION" == "null" ]] || [[ -z "$LATEST_VERSION" ]]; then
        log_error "No releases found"
        exit 1
    fi

    log_success "Latest release: $LATEST_VERSION"
}

# Download and extract release
download_release() {
    log_info "Downloading $LATEST_VERSION..."

    # Create temp directory
    local temp_dir
    temp_dir=$(mktemp -d)
    trap "rm -rf '$temp_dir'" EXIT

    # Download tarball
    local tarball="$temp_dir/debforge.tar.gz"

    if command -v curl &>/dev/null; then
        curl -fsSL "$LATEST_TARBALL" -o "$tarball"
    else
        wget -qO "$tarball" "$LATEST_TARBALL"
    fi

    log_success "Downloaded to $tarball"

    # Extract to install directory
    log_info "Installing to $INSTALL_DIR..."

    # Create install directory
    sudo mkdir -p "$INSTALL_DIR"
    
    # Extract and strip the first directory level (repo name-commit hash)
    # GitHub tarballs have format: repo-name-commit/
    sudo tar -xzf "$tarball" -C "$INSTALL_DIR" --strip-components=1

    log_success "Extracted to $INSTALL_DIR"
}

# Create symlink
create_symlink() {
    log_info "Creating symlink: $BIN_LINK..."

    # Remove existing symlink/file if it exists
    if [[ -L "$BIN_LINK" ]]; then
        sudo rm -f "$BIN_LINK"
    elif [[ -f "$BIN_LINK" ]]; then
        # Backup existing file if it's not a symlink
        log_warn "Existing file at $BIN_LINK, backing up..."
        sudo mv "$BIN_LINK" "${BIN_LINK}.bak.$(date +%Y%m%d-%H%M%S)"
    fi

    # Create new symlink
    if sudo ln -s "$INSTALL_DIR/scripts/debforge" "$BIN_LINK"; then
        log_success "Created symlink: $BIN_LINK"
    else
        log_error "Failed to create symlink"
        return 1
    fi
}

# Set permissions
set_permissions() {
    log_info "Setting permissions..."

    # Make all scripts executable recursively
    sudo find "$INSTALL_DIR/scripts" -name "*.sh" -exec chmod +x {} \; 2>/dev/null || true
    sudo find "$INSTALL_DIR/scripts" -name "debforge" -exec chmod +x {} \; 2>/dev/null || true
    sudo chmod +x "$INSTALL_DIR/bin/"* 2>/dev/null || true

    log_success "Permissions set"
}

# Verify installation
verify_installation() {
    log_info "Verifying installation..."

    local errors=0

    # Check install directory
    if [[ ! -d "$INSTALL_DIR" ]]; then
        log_error "Install directory missing: $INSTALL_DIR"
        ((errors++))
    fi

    # Check key files
    if [[ ! -f "$INSTALL_DIR/scripts/install.sh" ]]; then
        log_error "Setup script missing"
        ((errors++))
    fi

    if [[ ! -f "$INSTALL_DIR/scripts/debforge" ]]; then
        log_error "CLI wrapper missing"
        ((errors++))
    fi

    # Check symlink
    if [[ ! -L "$BIN_LINK" ]]; then
        log_error "Symlink not created: $BIN_LINK"
        ((errors++))
    fi

    # Test CLI wrapper
    if ! command -v debforge &>/dev/null; then
        log_error "debforge command not found"
        ((errors++))
    fi

    if [[ $errors -gt 0 ]]; then
        log_error "Verification failed with $errors error(s)"
        return 1
    fi

    log_success "Installation verified successfully"
    return 0
}

# Print success message
print_success() {
    echo ""
    echo -e "${GREEN}${BOLD}DebForge installed successfully!${NC}"
    echo ""
    echo "What's next:"
    echo "  1. Run: debforge install    # Setup and install configurations"
    echo "  2. Run: debforge status   # Check installation status"
    echo "  3. Run: debforge help     # Show all commands"
    echo ""
    echo "To uninstall:"
    echo "  debforge uninstall"
    echo ""
    echo "Documentation: https://github.com/$GITHUB_REPO"
    echo ""
}

# Main
main() {
    echo ""
    echo -e "${CYAN}${BOLD}DebForge Bootstrap Installer${NC}"
    echo ""

    check_root
    check_requirements
    get_latest_release
    download_release
    create_symlink
    set_permissions
    verify_installation

    print_success
}

main "$@"
