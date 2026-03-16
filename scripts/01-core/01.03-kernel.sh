#!/usr/bin/env bash
set -euo pipefail

# kernel choice
if ! grep -q "^VERSION_CODENAME=trixie$" /etc/os-release; then
    echo "ERROR: This script targets Debian Trixie. Exiting."
    exit 1
fi

# Parse arguments
KERNEL_CHOICE=""
SKIP_INTERACTIVE=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --kernel)
            KERNEL_CHOICE="$2"
            shift 2
            ;;
        --backports)
            KERNEL_CHOICE="backports"
            shift
            ;;
        --liquorix)
            KERNEL_CHOICE="liquorix"
            shift
            ;;
        --interactive)
            SKIP_INTERACTIVE=false
            shift
            ;;
        --help|-h)
            echo "Usage: $0 [--kernel backports|liquorix] [--interactive]"
            echo ""
            echo "Options:"
            echo "  --kernel, -k backports|liquorix  Choose kernel non-interactively"
            echo "  --backports                      Use Debian backported kernel"
            echo "  --liquorix                       Use Liquorix kernel"
            echo "  --interactive                    Force interactive mode (default)"
            echo "  --help, -h                       Show this help"
            echo ""
            echo "Examples:"
            echo "  $0 --kernel backports    # Non-interactive, backports kernel"
            echo "  $0 --liquorix            # Non-interactive, Liquorix kernel"
            echo "  $0                       # Interactive mode"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

ensure_backports() {
    if ! grep -rq "trixie-backports" /etc/apt/sources.list /etc/apt/sources.list.d/ 2>/dev/null; then
        echo "==> Adding trixie-backports..."
        echo "deb http://deb.debian.org/debian trixie-backports main contrib non-free non-free-firmware" | \
            sudo tee /etc/apt/sources.list.d/backports.list >/dev/null
        sudo apt update
    else
        echo "Backports already enabled."
    fi
}

# If no kernel choice provided, use interactive mode
if [[ -z "$KERNEL_CHOICE" ]]; then
    echo "Choose kernel to install:"
    echo "  1) Debian backported kernel"
    echo "  2) Liquorix kernel"
    read -r -p "Enter choice [1-2]: " kernel_choice

    case "${kernel_choice}" in
        1) KERNEL_CHOICE="backports" ;;
        2) KERNEL_CHOICE="liquorix" ;;
        *)
            echo "ERROR: Invalid choice. Exiting."
            exit 1
            ;;
    esac
fi

# Install selected kernel
case "${KERNEL_CHOICE}" in
    backports)
        ensure_backports
        echo "==> Installing backported kernel (amd64)..."
        sudo apt install -t trixie-backports -y \
            linux-headers-amd64 \
            linux-image-amd64
        ;;
    liquorix)
        echo "==> Installing Liquorix kernel..."
        sudo apt install -y curl
        sudo apt install -t trixie-backports -y libelf-dev
        curl -fsSL https://liquorix.net/install-liquorix.sh | sudo bash
        ;;
    *)
        echo "ERROR: Invalid kernel choice '$KERNEL_CHOICE'. Use 'backports' or 'liquorix'."
        exit 1
        ;;
esac

# apply ntsync
echo "==> Enabling ntsync module..."
echo ntsync | sudo tee /etc/modules-load.d/ntsync.conf

echo "==> Kernel installation complete: $KERNEL_CHOICE"
