#!/usr/bin/env bash
set -euo pipefail

echo "==> Setting up Flatpak for KDE Plasma..."
sudo apt install -y \
    flatpak \
    plasma-discover-backend-flatpak \
    xdg-desktop-portal \
    xdg-desktop-portal-kde

flatpak remote-add --if-not-exists flathub \
    https://dl.flathub.org/repo/flathub.flatpakrepo

# Disable Baloo file indexing
echo "==> Disabling Baloo file indexing..."
if command -v kwriteconfig6 &>/dev/null; then
    kwriteconfig6 --file baloofilerc --group "Basic Settings" --key "Indexing-Enabled" "false"
fi
if command -v balooctl6 &>/dev/null; then
    balooctl6 disable 2>/dev/null || true
fi

# Faster animations
echo "==> Setting animation duration factor to 0.5..."
if command -v kwriteconfig6 &>/dev/null; then
    kwriteconfig6 --file kdeglobals --group "KDE" --key "AnimationDurationFactor" "0.5"
fi

# SDDM Wayland greeter
echo "==> Deploying SDDM Wayland config..."
sudo mkdir -p /usr/lib/sddm/sddm.conf.d
sudo touch /usr/lib/sddm/sddm.conf.d/zz-wayland.conf
sudo tee /usr/lib/sddm/sddm.conf.d/zz-wayland.conf > /dev/null << SDDMCONF
[General]
DisplayServer=wayland
GreeterEnvironment=QT_WAYLAND_SHELL_INTEGRATION=layer-shell

[Wayland]
CompositorCommand=kwin_wayland --drm --no-lockscreen --no-global-shortcuts --locale1
SDDMCONF