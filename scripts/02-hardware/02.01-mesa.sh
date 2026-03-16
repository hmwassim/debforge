#!/usr/bin/env bash
set -euo pipefail

sudo apt install -t trixie-backports -y \
    intel-media-va-driver-non-free \
    intel-media-va-driver-non-free:i386 \
    mesa-va-drivers \
    mesa-va-drivers:i386 \
    mesa-vulkan-drivers \
    mesa-vulkan-drivers:i386 \
    libva2 \
    libva2:i386 \
    libvulkan1 \
    libvulkan1:i386 \
    libglx-mesa0:i386 \
    libgl1-mesa-dri:i386 \
    vulkan-tools \
    vulkan-validationlayers \
    vainfo \
    vdpauinfo
