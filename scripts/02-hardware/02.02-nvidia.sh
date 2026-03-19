#!/usr/bin/env bash
set -euo pipefail

# ─────────────────────────────────────────────────────────────
# Requirements check
# ─────────────────────────────────────────────────────────────
for cmd in extrepo git; do
    if ! command -v "$cmd" &>/dev/null; then
        echo "ERROR: $cmd not found. Install it first."
        exit 1
    fi
done

# ─────────────────────────────────────────────────────────────
# Parse arguments
# ─────────────────────────────────────────────────────────────
NVIDIA_VARIANT=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --variant|-v)
            [[ $# -lt 2 ]] && { echo "ERROR: --variant requires a value"; exit 1; }
            NVIDIA_VARIANT="$2"
            shift 2
            ;;
        --open|-o)
            NVIDIA_VARIANT="nvidia-open"
            shift
            ;;
        --proprietary|-p)
            NVIDIA_VARIANT="cuda-drivers"
            shift
            ;;
        --help|-h)
            echo "Usage: $0 [--variant nvidia-open|cuda-drivers]"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Validate variant
if [[ -n "$NVIDIA_VARIANT" ]]; then
    case "$NVIDIA_VARIANT" in
        nvidia-open|cuda-drivers) ;;
        *)
            echo "ERROR: Invalid variant: $NVIDIA_VARIANT"
            exit 1
            ;;
    esac
fi

# ─────────────────────────────────────────────────────────────
# Enable repo
# ─────────────────────────────────────────────────────────────
echo "==> Enabling NVIDIA CUDA repository..."
sudo extrepo enable nvidia-cuda
sudo apt update
sudo apt upgrade -y

# ─────────────────────────────────────────────────────────────
# Interactive selection
# ─────────────────────────────────────────────────────────────
if [[ -z "$NVIDIA_VARIANT" ]]; then
    echo
    echo "Choose NVIDIA driver variant:"
    echo "  1) nvidia-open   (recommended for RTX 3060)"
    echo "  2) cuda-drivers  (full proprietary stack)"
    echo

    read -rp "Choice [1/2]: " choice

    case "$choice" in
        1) NVIDIA_VARIANT="nvidia-open" ;;
        2) NVIDIA_VARIANT="cuda-drivers" ;;
        *) echo "Invalid choice"; exit 1 ;;
    esac
fi

# ─────────────────────────────────────────────────────────────
# Install packages
# ─────────────────────────────────────────────────────────────
echo "==> Installing $NVIDIA_VARIANT + nvtop..."
sudo apt install -y "$NVIDIA_VARIANT" nvtop

# ─────────────────────────────────────────────────────────────
# Kernel parameter
# ─────────────────────────────────────────────────────────────
echo
echo "==> Configuring nvidia-drm.modeset=1..."

add_modeset_grub() {
    local cfg=/etc/default/grub

    if grep -q "nvidia-drm.modeset=1" "$cfg"; then
        echo "    Already set (GRUB)"
        return
    fi

    sudo sed -i \
        's/^GRUB_CMDLINE_LINUX_DEFAULT="\([^"]*\)"/GRUB_CMDLINE_LINUX_DEFAULT="\1 nvidia-drm.modeset=1"/' \
        "$cfg"

    sudo update-grub
    echo "    Applied to GRUB"
}

add_modeset_systemd_boot() {
    local entries
    entries=$(find /boot /efi /boot/efi -type f -path "*/loader/entries/*.conf" 2>/dev/null || true)

    if [[ -z "$entries" ]]; then
        echo "    WARNING: No systemd-boot entries found"
        return
    fi

    for entry in $entries; do
        if grep -q "nvidia-drm.modeset=1" "$entry"; then
            echo "    Already set in $entry"
            continue
        fi

        sudo sed -i '/^options / s/$/ nvidia-drm.modeset=1/' "$entry"
        echo "    Patched $entry"
    done
}

if [[ -d /boot/loader ]]; then
    add_modeset_systemd_boot
elif [[ -f /etc/default/grub ]]; then
    add_modeset_grub
else
    echo "WARNING: Bootloader not detected"
fi

# ─────────────────────────────────────────────────────────────
# NVFlux
# ─────────────────────────────────────────────────────────────
echo
echo "==> Installing NVFlux..."

NVFLUX_TEMP_DIR=$(mktemp -d)
trap 'rm -rf "$NVFLUX_TEMP_DIR"' EXIT

if git clone --quiet https://github.com/hmwassim/NvFlux.git "$NVFLUX_TEMP_DIR"; then
    if (cd "$NVFLUX_TEMP_DIR" && sudo ./install.sh); then
        echo "    NVFlux installed"
    else
        echo "    WARNING: NVFlux install failed"
    fi
else
    echo "    WARNING: Clone failed"
fi

echo
echo "==> Done: $NVIDIA_VARIANT"