#!/usr/bin/env bash
set -euo pipefail

if ! command -v extrepo &>/dev/null; then
    echo "ERROR: extrepo not found. Run base/system.sh first."
    exit 1
fi

# Parse arguments
NVIDIA_VARIANT=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --variant)
            NVIDIA_VARIANT="$2"
            shift 2
            ;;
        --open)
            NVIDIA_VARIANT="nvidia-open"
            shift
            ;;
        --proprietary)
            NVIDIA_VARIANT="cuda-drivers"
            shift
            ;;
        --help|-h)
            echo "Usage: $0 [--variant nvidia-open|cuda-drivers]"
            echo ""
            echo "Options:"
            echo "  --variant, -v nvidia-open|cuda-drivers  Choose driver variant"
            echo "  --open, -o                              Use nvidia-open (recommended for RTX 3060)"
            echo "  --proprietary, -p                       Use cuda-drivers (proprietary, full CUDA)"
            echo "  --help, -h                              Show this help"
            echo ""
            echo "Examples:"
            echo "  $0 --variant nvidia-open    # Non-interactive, open drivers"
            echo "  $0 --open                   # Non-interactive, open drivers"
            echo "  $0 --proprietary            # Non-interactive, proprietary drivers"
            echo "  $0                          # Interactive mode"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

echo "==> Enabling NVIDIA CUDA repository via extrepo..."
sudo extrepo enable nvidia-cuda
sudo apt update
sudo apt upgrade -y

# If no variant provided, use interactive mode
if [[ -z "$NVIDIA_VARIANT" ]]; then
    echo
    echo "Choose NVIDIA driver variant:"
    echo "  1) nvidia-open   — open kernel modules (recommended for RTX 3060)"
    echo "  2) cuda-drivers  — proprietary, full CUDA stack"
    echo
    read -rp "Choice [1/2]: " choice

    case "$choice" in
        1) NVIDIA_VARIANT="nvidia-open" ;;
        2) NVIDIA_VARIANT="cuda-drivers" ;;
        *)
            echo "Invalid choice. Exiting."
            exit 1
            ;;
    esac
fi

echo "==> Installing $NVIDIA_VARIANT + nvtop..."
sudo apt install -y "$NVIDIA_VARIANT" nvtop

echo
echo "==> Adding nvidia-drm.modeset=1 kernel parameter..."

add_modeset_grub() {
    local cfg=/etc/default/grub
    if grep -q "nvidia-drm.modeset=1" "$cfg"; then
        echo "    Already present in $cfg, skipping."
        return
    fi
    sudo sed -i '/^GRUB_CMDLINE_LINUX_DEFAULT=/ s/"$/ nvidia-drm.modeset=1"/' "$cfg"
    sudo update-grub
    echo "    Written to $cfg and GRUB updated."
}

add_modeset_systemd_boot() {
    local entry
    for mp in /efi /boot/efi /boot; do
        entry=$(sudo find "$mp/loader/entries" -maxdepth 1 -name "*.conf" 2>/dev/null | sort | head -1)
        [[ -n "$entry" ]] && break
    done
    if [[ -z "$entry" ]]; then
        echo "    WARNING: No systemd-boot entry found. Add nvidia-drm.modeset=1 manually."
        return
    fi
    if grep -q "nvidia-drm.modeset=1" "$entry"; then
        echo "    Already present in $entry, skipping."
        return
    fi
    sudo sed -i '/^options / s/$/ nvidia-drm.modeset=1/' "$entry"
    echo "    Written to $entry."
}

if sudo bootctl is-installed 2>/dev/null; then
    add_modeset_systemd_boot
elif [[ -f /etc/default/grub ]]; then
    add_modeset_grub
else
    echo "    WARNING: Could not detect bootloader. Add nvidia-drm.modeset=1 manually."
fi

echo "==> NVIDIA driver installation complete: $NVIDIA_VARIANT"
