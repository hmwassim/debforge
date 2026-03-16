#!/usr/bin/env bash
set -euo pipefail

echo "==> Installing PipeWire audio stack..."
sudo apt install -t trixie-backports -y \
    pipewire \
    pipewire:i386 \
    pipewire-audio \
    pipewire-pulse \
    pipewire-alsa \
    pipewire-jack \
    libpipewire-0.3-0t64:i386 \
    libspa-0.2-modules:i386 \
    wireplumber \
    libspa-0.2-bluetooth \
    libspa-0.2-jack \
    libasound2-plugins \
    libasound2-plugins:i386 \
    rtkit

echo "==> Installing EasyEffects and LV2 plugins..."
sudo apt install -t trixie-backports -y \
    easyeffects \
    lsp-plugins-lv2 \
    calf-plugins \
    x42-plugins \
    zam-plugins

echo "==> Installing codec and multimedia stack..."
sudo apt install -t trixie-backports -y \
    ffmpeg \
    libavcodec-extra \
    libavcodec-extra:i386 \
    gstreamer1.0-libav \
    gstreamer1.0-libav:i386 \
    gstreamer1.0-plugins-good \
    gstreamer1.0-plugins-good:i386 \
    gstreamer1.0-plugins-bad \
    gstreamer1.0-plugins-bad:i386 \
    gstreamer1.0-plugins-ugly \
    gstreamer1.0-plugins-ugly:i386 \
    gstreamer1.0-vaapi \
    gstreamer1.0-alsa \
    gstreamer1.0-tools

echo "==> Installing media players and tools..."
sudo apt install -t trixie-backports -y \
    mpv \
    vlc \
    mpg123 \
    lame \
    x264 \
    x265 \
    opus-tools \
    flvmeta

echo "==> Installing runtime libraries..."
sudo apt install -t trixie-backports -y \
    libgif7           libgif7:i386 \
    libglfw3          libglfw3:i386 \
    libosmesa6        libosmesa6:i386 \
    libvulkan-dev     libvulkan-dev:i386

echo "==> Installing MIDI + DOS emulation..."
sudo apt install -t trixie-backports -y \
    timidity \
    fluidsynth \
    dosbox

mkdir -p \
    ~/.config/wireplumber/wireplumber.conf.d \
    ~/.config/pipewire/pipewire.conf.d \
    ~/.config/pipewire/pipewire-pulse.conf.d

cat > ~/.config/wireplumber/wireplumber.conf.d/51-disable-suspend.conf << 'EOF'
monitor.alsa.rules = [
  {
    matches = [
      { node.name = "~alsa_output.*" }
      { node.name = "~alsa_input.*"  }
    ]
    actions = {
      update-props = {
        session.suspend-timeout-seconds = 0
      }
    }
  }
]
EOF

cat > ~/.config/wireplumber/wireplumber.conf.d/52-deprioritize-hdmi.conf << 'EOF'
monitor.alsa.rules = [
  {
    matches = [{ node.name = "~alsa_output.*hdmi*" }]
    actions = {
      update-props = {
        priority.driver  = 100
        priority.session = 100
      }
    }
  }
]
EOF

cat > ~/.config/pipewire/pipewire.conf.d/10-clock.conf << 'EOF'
context.properties = {
    default.clock.rate           = 48000
    default.clock.allowed-rates  = [ 44100 48000 96000 ]
    default.clock.quantum        = 512
    default.clock.min-quantum    = 256
    default.clock.max-quantum    = 2048
}
EOF

cat > ~/.config/pipewire/pipewire-pulse.conf.d/10-pulse.conf << 'EOF'
pulse.properties = {
    pulse.min.req     = 256/48000
    pulse.default.req = 512/48000
    pulse.max.req     = 2048/48000
}
EOF

systemctl --user enable --now pipewire pipewire-pulse wireplumber
systemctl --user restart pipewire pipewire-pulse wireplumber