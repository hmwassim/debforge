package coresetup

import "os"

type ConfigDef struct {
	Dest    string
	Content string
	Mode    os.FileMode
}

type GroupDef struct {
	Name        string
	Description string
	Packages    []string
	Backport    bool
	Configs     []ConfigDef
	Services    []string
	PostInstall string
}

var GroupDefs = []GroupDef{
	{
		Name:        "system-base",
		Description: "Base system packages (firmware, drivers, build tools)",
		Packages: []string{
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
			"cabextract", "extrepo", "zenity", "jq", "lm-sensors", "ddcutil", "hwloc",
			"hunspell-en-us", "hunspell-fr",
		},
	},
	{
		Name:        "kernel",
		Description: "Backported kernel",
		Packages:    []string{"linux-image-amd64", "linux-headers-amd64"},
		Backport:    true,
	},
	{
		Name:        "desktop-tools",
		Description: "Desktop tools (eza, starship, bat, ripgrep, etc.)",
		Packages:    []string{"eza", "starship", "papirus-icon-theme", "fastfetch", "bat", "ripgrep"},
	},
	{
		Name:        "system-fonts",
		Description: "System fonts (Liberation, Noto, Cantarell, Inter)",
		Packages: []string{
			"fonts-liberation", "fonts-liberation2", "fonts-croscore",
			"fonts-cantarell", "fonts-inter", "fonts-inter-variable",
			"fonts-noto", "fonts-noto-core", "fonts-noto-hinted",
			"fonts-noto-ui-core", "fonts-noto-unhinted", "fonts-noto-cjk",
			"fonts-noto-cjk-extra", "fonts-noto-color-emoji",
			"fonts-noto-extra", "fonts-noto-mono", "fonts-noto-ui-extra",
		},
		Configs: []ConfigDef{
			{Dest: "/etc/fonts/local.conf", Content: FontConfig, Mode: 0644},
		},
		PostInstall: "fonts",
	},
	{
		Name:        "system-services",
		Description: "System services (zram, resolved, timesyncd)",
		Packages:    []string{"systemd-zram-generator", "systemd-resolved", "systemd-timesyncd"},
		Configs: []ConfigDef{
			{Dest: "/etc/systemd/zram-generator.conf", Content: ZramConfig, Mode: 0644},
			{Dest: "/etc/systemd/resolved.conf.d/99-dot.conf", Content: ResolvedConfig, Mode: 0644},
			{Dest: "/etc/NetworkManager/conf.d/10-dns.conf", Content: NmDNSConfig, Mode: 0644},
			{Dest: "/etc/systemd/timesyncd.conf.d/10-timesyncd.conf", Content: TimesyncdConfig, Mode: 0644},
		},
		Services:    []string{"systemd-resolved", "systemd-timesyncd"},
		PostInstall: "resolved",
	},
	{
		Name:        "multimedia",
		Description: "Multimedia (PipeWire, GStreamer, codecs, gaming libs)",
		Packages: []string{
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
		Configs: []ConfigDef{
			{Dest: "/etc/security/limits.d/20-audio.conf", Content: AudioLimitsConfig, Mode: 0644},
		},
	},
	{
		Name:        "flatpak",
		Description: "Flatpak + Flathub",
		Packages:    []string{"flatpak"},
		PostInstall: "flathub",
	},
}

type Groups struct {
	items []GroupDef
}

func NewGroups() *Groups {
	items := make([]GroupDef, len(GroupDefs))
	for i, g := range GroupDefs {
		cp := GroupDef{
			Name:        g.Name,
			Description: g.Description,
			Backport:    g.Backport,
			PostInstall: g.PostInstall,
		}
		if g.Packages != nil {
			cp.Packages = make([]string, len(g.Packages))
			copy(cp.Packages, g.Packages)
		}
		if g.Configs != nil {
			cp.Configs = make([]ConfigDef, len(g.Configs))
			copy(cp.Configs, g.Configs)
		}
		if g.Services != nil {
			cp.Services = make([]string, len(g.Services))
			copy(cp.Services, g.Services)
		}
		items[i] = cp
	}
	return &Groups{items: items}
}

func (g *Groups) List() []GroupDef {
	result := make([]GroupDef, len(g.items))
	copy(result, g.items)
	return result
}
func (g *Groups) Lookup(name string) (GroupDef, bool) {
	for _, gr := range g.items {
		if gr.Name == name {
			return gr, true
		}
	}
	return GroupDef{}, false
}
