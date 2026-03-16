#!/usr/bin/env bash
set -euo pipefail

if ! grep -rq "contrib" /etc/apt/sources.list /etc/apt/sources.list.d/ 2>/dev/null; then
    echo "==> Enabling contrib component..."
    sudo sed -i 's/main$/main contrib/' /etc/apt/sources.list
    sudo apt update
fi

echo "==> Installing system fonts..."
sudo apt install -y \
    fonts-liberation \
    fonts-liberation2 \
    fonts-croscore \
    fonts-cantarell \
    fonts-inter \
    fonts-inter-variable

sudo apt install -y \
    fonts-noto \
    fonts-noto-core \
    fonts-noto-hinted \
    fonts-noto-ui-core \
    fonts-noto-unhinted \
    fonts-noto-cjk \
    fonts-noto-cjk-extra \
    fonts-noto-color-emoji \
    fonts-noto-extra \
    fonts-noto-mono \
    fonts-noto-ui-extra

echo "==> Installing Fonts from Codeberg..."

MY_FONTS_DIR="/usr/local/share/fonts"
MY_TEMP="$(mktemp -d)"

trap 'rm -rf "$MY_TEMP"' EXIT

curl -fL "https://codeberg.org/hmwassim/fonts/raw/branch/main/fonts.tar.gz" \
    -o "$MY_TEMP/fonts.tar.gz"

sudo install -d -m 0755 "$MY_FONTS_DIR"
sudo tar -xzf "$MY_TEMP/fonts.tar.gz" -C "$MY_FONTS_DIR"
trap - EXIT
rm -rf "$MY_TEMP"

sudo tee /etc/fonts/local.conf > /dev/null << 'EOF'
<?xml version='1.0'?>
<!DOCTYPE fontconfig SYSTEM 'fonts.dtd'>
<fontconfig>
  <!-- Set preferred serif, sans serif, and monospace fonts. -->
  <alias>
   <family>sans-serif</family>
   <prefer>
    <family>Arimo</family>
    <family>Noto Sans Arabic</family>
   </prefer>
  </alias>

  <alias>
   <family>serif</family>
   <prefer>
    <family>Tinos</family>
    <family>Noto Sans Arabic</family>
   </prefer>
  </alias>

  <alias>
   <family>Sans</family>
   <prefer>
    <family>Arimo</family>
    <family>Noto Sans Arabic</family>
   </prefer>
  </alias>

  <alias>
   <family>monospace</family>
   <prefer>
    <family>Cousine</family>
    <family>Noto Sans Arabic</family>
   </prefer>
  </alias>
  <!-- Aliases for commonly used MS fonts. -->
  <match>
    <test name="family"><string>Arial</string></test>
    <edit name="family" mode="assign" binding="strong">
      <string>Arimo</string>
    </edit>
  </match>
  <match>
    <test name="family"><string>Helvetica</string></test>
    <edit name="family" mode="assign" binding="strong">
      <string>Arimo</string>
    </edit>
  </match>
  <match>
    <test name="family"><string>Verdana</string></test>
    <edit name="family" mode="assign" binding="strong">
      <string>Arimo</string>
    </edit>
  </match>
  <match>
    <test name="family"><string>Tahoma</string></test>
    <edit name="family" mode="assign" binding="strong">
      <string>Arimo</string>
    </edit>
  </match>
  <match>
    <!-- Insert joke here -->
    <test name="family"><string>Comic Sans MS</string></test>
    <edit name="family" mode="assign" binding="strong">
      <string>Arimo</string>
    </edit>
  </match>
  <match>
    <test name="family"><string>Times New Roman</string></test>
    <edit name="family" mode="assign" binding="strong">
      <string>Tinos</string>
    </edit>
  </match>
  <match>
    <test name="family"><string>Times</string></test>
    <edit name="family" mode="assign" binding="strong">
      <string>Tinos</string>
    </edit>
  </match>
  <match>
    <test name="family"><string>Courier New</string></test>
    <edit name="family" mode="assign" binding="strong">
      <string>Cousine</string>
    </edit>
  </match>
</fontconfig>
EOF

echo "==> Rebuilding font cache..."
sudo fc-cache -f -v
echo "==> Fonts installed."
