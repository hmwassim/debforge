#!/usr/bin/env bash
set -euo pipefail

if ! grep -q "VERSION_CODENAME=trixie" /etc/os-release; then
    echo "ERROR: This script targets Debian Trixie. Exiting."
    exit 1
fi

echo "==> Updating system..."
sudo apt update
sudo apt full-upgrade -y

echo "==> Enabling i386 architecture..."
sudo dpkg --add-architecture i386
sudo apt update

if ! grep -rq "trixie-backports" /etc/apt/sources.list /etc/apt/sources.list.d/ 2>/dev/null; then
    echo "==> Adding trixie-backports..."
    echo "deb http://deb.debian.org/debian trixie-backports main contrib non-free non-free-firmware" | \
        sudo tee /etc/apt/sources.list.d/backports.list
    sudo apt update
fi

echo "==> Installing firmware (from backports)..."
sudo apt install -t trixie-backports -y \
    firmware-linux \
    firmware-linux-nonfree \
    firmware-misc-nonfree \
    firmware-iwlwifi \
    firmware-sof-signed \
    intel-microcode \
    firmware-realtek

echo "==> Installing core tools..."
sudo apt install -t trixie-backports -y \
    git \
    curl \
    wget \
    unzip \
    p7zip-full \
    gzip \
    build-essential \
    pkg-config \
    cmake \
    nvme-cli \
    smartmontools \
    pciutils \
    usbutils \
    cabextract \
    zenity \
    extrepo \
    jq \
    lm-sensors \
    hunspell-en-us \
    hunspell-fr \
    ddcutil \
    earlyoom \
    systemd-zram-generator

echo "==> Adding $USER to hardware groups..."
sudo usermod -aG audio,video,render,i2c "$USER"

# DNS-over-TLS
echo "==> Configuring systemd-resolved with DNS-over-TLS..."
sudo apt install -y systemd-resolved

sudo ln -sf /run/systemd/resolve/stub-resolv.conf /etc/resolv.conf

# The actual DoT config is in configs/systemd/resolved.conf.d/99-dot.conf
# and gets deployed by configs/configs_apply.sh.
sudo systemctl enable --now systemd-resolved

# NetworkManager -> resolved integration
sudo mkdir -p /usr/lib/NetworkManager/conf.d
REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
sudo install -m 0644 "$REPO_DIR/configs/NetworkManager/dns.conf" \
    /usr/lib/NetworkManager/conf.d/dns.conf

sudo nmcli general reload 2>/dev/null || sudo systemctl reload NetworkManager 2>/dev/null || true
sudo systemctl restart systemd-resolved

echo "    Waiting for network connectivity..."
for i in $(seq 1 15); do
    if resolvectl query debian.org &>/dev/null; then
        echo "    Network OK."
        break
    fi
    sleep 2
done

if ! resolvectl query debian.org &>/dev/null; then
    echo "WARNING: DNS resolution check failed — verify resolved config after optimize/apply.sh"
fi

sudo apt autoremove -y
echo "==> Base system setup complete. A reboot is recommended."
