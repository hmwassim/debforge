#!/usr/bin/env bash
set -euo pipefail

sudo extrepo enable winehq
sudo apt update

echo "==> Installing Wine (WineHQ staging)..."
sudo apt install -y --install-recommends winehq-staging

echo "==> Installing Steam..."
sudo apt install -y \
    steam-installer \
    steam-devices \
    steam-libs \
    steam-libs-i386 \
    libgtk2.0-0:i386

echo "==> Installing MangoHud, GOverlay and Gamemode..."
sudo apt install -y \
    mangohud \
    mangohud:i386 \
    goverlay \
    gamemode \
    unrar