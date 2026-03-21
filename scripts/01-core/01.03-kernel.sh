#!/usr/bin/env bash
set -euo pipefail

# ─────────────────────────────────────────────────────────────
# OS check
# ─────────────────────────────────────────────────────────────
if ! grep -q "^VERSION_CODENAME=trixie$" /etc/os-release; then
    echo "ERROR: This script targets Debian Trixie."
    exit 1
fi

# ─────────────────────────────────────────────────────────────
# Parse arguments
# ─────────────────────────────────────────────────────────────
KERNEL_CHOICE=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --kernel|-k)
            [[ $# -lt 2 ]] && { echo "ERROR: --kernel requires a value"; exit 1; }
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
        --help|-h)
            echo "Usage: $0 [--kernel backports|liquorix]"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Validate kernel choice
if [[ -n "$KERNEL_CHOICE" ]]; then
    case "$KERNEL_CHOICE" in
        backports|liquorix) ;;
        *)
            echo "ERROR: Invalid kernel: $KERNEL_CHOICE"
            exit 1
            ;;
    esac
fi

# ─────────────────────────────────────────────────────────────
# Backports helper
# ─────────────────────────────────────────────────────────────
ensure_backports() {
    if ! grep -rq "trixie-backports" /etc/apt/sources.list /etc/apt/sources.list.d/ 2>/dev/null; then
        echo "==> Adding trixie-backports..."
        echo "deb http://deb.debian.org/debian trixie-backports main contrib non-free non-free-firmware" | \
            sudo tee /etc/apt/sources.list.d/backports.list >/dev/null
    fi
}

# ─────────────────────────────────────────────────────────────
# Interactive mode
# ─────────────────────────────────────────────────────────────
if [[ -z "$KERNEL_CHOICE" ]]; then
    echo "Choose kernel:"
    echo "  1) Debian backports"
    echo "  2) Liquorix"

    read -rp "Choice [1/2]: " choice

    case "$choice" in
        1) KERNEL_CHOICE="backports" ;;
        2) KERNEL_CHOICE="liquorix" ;;
        *) echo "Invalid choice"; exit 1 ;;
    esac
fi

# ─────────────────────────────────────────────────────────────
# Install
# ─────────────────────────────────────────────────────────────
case "$KERNEL_CHOICE" in
    backports)
        ensure_backports
        sudo apt update
        echo "==> Installing backports kernel..."
        sudo apt install -y -t trixie-backports \
            linux-image-amd64 linux-headers-amd64
        ;;
    liquorix)
        echo "==> Installing Liquorix kernel..."
        sudo apt update
        sudo apt install -y gnupg
        sudo apt install -t trixie-backports -y libelf-dev
        curl -fsSL https://liquorix.net/install-liquorix.sh -o /tmp/liquorix.sh
        sudo bash /tmp/liquorix.sh
        ;;
esac

# ─────────────────────────────────────────────────────────────
# ntsync (safe)
# ─────────────────────────────────────────────────────────────
echo "==> Enabling ntsync module..."
echo ntsync | sudo tee /etc/modules-load.d/ntsync.conf

echo "==> Done: $KERNEL_CHOICE"
