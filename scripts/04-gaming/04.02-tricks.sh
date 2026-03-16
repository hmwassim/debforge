#!/usr/bin/env bash
set -euo pipefail

echo "==> Downloading latest Winetricks and Protontricks..."

REMOTE_URL="https://raw.githubusercontent.com/Winetricks/winetricks/master/src/winetricks"
INSTALL_PATH="/usr/local/bin/winetricks"

echo "==> Downloading latest Winetricks..."
sudo curl -fsSL "$REMOTE_URL" -o "$INSTALL_PATH"
sudo chmod +x "$INSTALL_PATH"

DESKTOP_DIR="$HOME/.local/share/applications"
mkdir -p "$DESKTOP_DIR"

cat > "$DESKTOP_DIR/winetricks.desktop" << 'EOF'
[Desktop Entry]
Name=Winetricks
Comment=Work around problems in Wine
Exec=/usr/local/bin/winetricks --gui
Icon=winetricks
Type=Application
Terminal=false
Categories=Utility;Emulator;
StartupNotify=true
EOF

echo "==> Winetricks installed to $INSTALL_PATH"

echo "==> Installing pipx (if not already present)..."
sudo apt install -y pipx
pipx ensurepath

export PATH="$HOME/.local/bin:$PATH"

echo "==> Installing protontricks via pipx..."
pipx install --force protontricks

echo "==> Registering desktop integration..."
protontricks-desktop-install

echo "==> Protontricks installed. Open a new shell or run: source ~/.bashrc"