package core

import (
	"os"

	"github.com/hmwassim/debforge/pkg/text"
)

type configFile struct {
	dest    string
	content string
	mode    os.FileMode
}

type group struct {
	name        string
	packages    []string
	backport    bool
	configs     []configFile
	services    []string
	postInstall func(*text.Logger, *text.Spinner, bool) error
}

var groups = []group{
	{
		name: "system-base",
		packages: []string{
			"firmware-linux", "firmware-linux-nonfree", "firmware-misc-nonfree",
			"firmware-iwlwifi", "firmware-sof-signed", "firmware-realtek",
			"intel-microcode",
			"intel-media-va-driver-non-free", "intel-media-va-driver-non-free:i386",
			"mesa-va-drivers", "mesa-va-drivers:i386",
			"mesa-vulkan-drivers", "mesa-vulkan-drivers:i386",
			"libva2", "libva2:i386", "libvulkan1", "libvulkan1:i386",
			"libglx-mesa0:i386", "libgl1-mesa-dri:i386",
			"vulkan-tools", "vulkan-validationlayers", "vainfo", "vdpauinfo",
			"build-essential", "git", "curl", "wget", "unzip", "p7zip-full", "gzip",
			"pkg-config", "cmake", "nvme-cli", "smartmontools", "pciutils", "usbutils",
			"cabextract", "zenity", "jq", "lm-sensors", "ddcutil", "hwloc",
			"hunspell-en-us", "hunspell-fr",
		},
	},
	{
		name: "kernel",
		packages: []string{
			"linux-image-amd64", "linux-headers-amd64",
		},
		backport: true,
	},
	{
		name: "desktop-tools",
		packages: []string{
			"eza", "starship", "papirus-icon-theme", "fastfetch", "bat", "ripgrep",
		},
	},
	{
		name: "system-fonts",
		packages: []string{
			"fonts-liberation", "fonts-liberation2", "fonts-croscore",
			"fonts-cantarell", "fonts-inter", "fonts-inter-variable",
			"fonts-noto", "fonts-noto-core", "fonts-noto-hinted",
			"fonts-noto-ui-core", "fonts-noto-unhinted", "fonts-noto-cjk",
			"fonts-noto-cjk-extra", "fonts-noto-color-emoji",
			"fonts-noto-extra", "fonts-noto-mono", "fonts-noto-ui-extra",
		},
		configs: []configFile{
			{"/etc/fonts/local.conf", fontConfig, 0644},
		},
		postInstall: installCodebergFonts,
	},
	{
		name: "system-services",
		packages: []string{
			"systemd-zram-generator", "systemd-resolved", "systemd-timesyncd",
		},
		configs: []configFile{
			{"/etc/systemd/zram-generator.conf", zramConfig, 0644},
			{"/etc/systemd/resolved.conf.d/99-dot.conf", resolvedConfig, 0644},
			{"/etc/NetworkManager/conf.d/10-dns.conf", nmDNSConfig, 0644},
			{"/etc/systemd/timesyncd.conf.d/10-timesyncd.conf", timesyncdConfig, 0644},
		},
		services: []string{"systemd-resolved", "systemd-timesyncd"},
	},
	{
		name: "multimedia",
		packages: []string{
			"pipewire", "pipewire:i386", "pipewire-audio", "pipewire-pulse",
			"pipewire-alsa", "pipewire-jack", "pipewire-bin",
			"wireplumber", "rtkit",
			"alsa-utils", "alsa-tools", "alsa-firmware-loaders",
			"libasound2-plugins", "libasound2-plugins:i386",
			"libcanberra-pulse",
			"easyeffects", "lsp-plugins-lv2", "calf-plugins", "x42-plugins", "zam-plugins",
			"ffmpeg",
			"gstreamer1.0-libav", "gstreamer1.0-libav:i386",
			"gstreamer1.0-plugins-good", "gstreamer1.0-plugins-good:i386",
			"gstreamer1.0-plugins-bad", "gstreamer1.0-plugins-bad:i386",
			"gstreamer1.0-plugins-ugly", "gstreamer1.0-plugins-ugly:i386",
			"gstreamer1.0-vaapi", "gstreamer1.0-alsa", "gstreamer1.0-tools",
			"mpv", "vlc", "mpg123", "lame", "x264", "x265", "opus-tools", "flvmeta",
			"libgif7", "libgif7:i386", "giflib-tools",
			"libglfw3", "libglfw3:i386",
			"libosmesa6", "libosmesa6:i386",
			"libvulkan-dev", "libvulkan-dev:i386",
			"libgtk-3-0t64", "libgtk-3-0t64:i386",
			"libopenal1", "libopenal1:i386",
			"libturbojpeg0", "libturbojpeg0:i386",
			"libjpeg62-turbo", "libjpeg62-turbo:i386",
			"ocl-icd-libopencl1", "ocl-icd-libopencl1:i386",
			"libxslt1-dev", "libxml2-dev",
			"timidity", "fluidsynth", "dosbox",
		},
		configs: []configFile{
			{"/etc/security/limits.d/20-audio.conf", audioLimitsConfig, 0644},
		},
	},
	{
		name: "flatpak",
		packages: []string{
			"flatpak",
		},
		postInstall: installFlathub,
	},
}
