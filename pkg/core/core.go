package core

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/hmwassim/debforge/pkg/executil"
	"github.com/hmwassim/debforge/pkg/packages"
	"github.com/hmwassim/debforge/pkg/text"
)

func init() {
	packages.Register(Handler{})
}

type Handler struct{}

func (Handler) Type() string { return "core" }

func (Handler) Install(string) error {
	return fmt.Errorf("core packages are managed via 'debforge core update' or 'debforge core repair'")
}

func (Handler) Remove(string) error {
	return fmt.Errorf("core packages cannot be removed")
}

type configFile struct {
	dest    string
	content string
	mode    os.FileMode
}

type group struct {
	name     string
	packages []string
	backport bool
	configs  []configFile
	services []string
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
		services: []string{"pipewire", "pipewire-pulse", "wireplumber"},
	},
}

func Repair(log *text.Logger) error {
	log.Info("Repairing core system...")

	if err := ensureSourcesList(); err != nil {
		return fmt.Errorf("sources.list: %w", err)
	}
	if err := enablei386(); err != nil {
		return fmt.Errorf("i386: %w", err)
	}

	log.Info("Updating package lists...")
	if err := executil.Run(exec.Command("apt", "update")); err != nil {
		return fmt.Errorf("apt update: %w", err)
	}

	for _, g := range groups {
		log.Info("Installing %s...", g.name)

		args := []string{"install", "-y"}
		if g.backport {
			args = append(args, "-t", "trixie-backports")
		}
		args = append(args, g.packages...)

		if err := executil.Run(exec.Command("apt", args...)); err != nil {
			return fmt.Errorf("installing %s: %w", g.name, err)
		}

		for _, cf := range g.configs {
			if err := deployConfig(cf); err != nil {
				return fmt.Errorf("deploying %s: %w", cf.dest, err)
			}
		}

		for _, svc := range g.services {
			if err := executil.Run(exec.Command("systemctl", "enable", "--now", svc)); err != nil {
				log.Warn("Failed to start %s: %s", svc, err)
			}
		}
	}

	log.Success("Core repair complete")
	return nil
}

func Update(log *text.Logger) error {
	log.Info("Updating core packages...")

	if err := executil.Run(exec.Command("apt", "update")); err != nil {
		return fmt.Errorf("apt update: %w", err)
	}

	var allPkgs []string
	for _, g := range groups {
		allPkgs = append(allPkgs, g.packages...)
	}

	args := append([]string{"install", "-y"}, allPkgs...)
	if err := executil.Run(exec.Command("apt", args...)); err != nil {
		return fmt.Errorf("upgrading core: %w", err)
	}

	log.Success("Core packages up to date")
	return nil
}

func List(log *text.Logger) {
	log.Info("Core packages:")

	for _, g := range groups {
		var missing []string
		for _, pkg := range g.packages {
			cmd := exec.Command("dpkg", "-s", pkg)
			cmd.Stdout = io.Discard
			if err := executil.Run(cmd); err != nil {
				missing = append(missing, pkg)
			}
		}
		if len(missing) == 0 {
			log.Success("  %s — installed", g.name)
		} else {
			log.Warn("  %s — missing: %s", g.name, strings.Join(missing, ", "))
		}
	}
}

func ensureSourcesList() error {
	const path = "/etc/apt/sources.list"
	data, err := os.ReadFile(path)
	if err == nil && strings.Contains(string(data), "trixie") {
		return nil
	}
	return os.WriteFile(path, []byte(sourcesList), 0644)
}

func enablei386() error {
	out, err := exec.Command("dpkg", "--print-foreign-architectures").Output()
	if err == nil && strings.Contains(string(out), "i386") {
		return nil
	}
	return executil.Run(exec.Command("dpkg", "--add-architecture", "i386"))
}

func deployConfig(cf configFile) error {
	dir := cf.dest[:strings.LastIndex(cf.dest, "/")]
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(cf.dest, []byte(cf.content), cf.mode)
}

const sourcesList = `deb http://deb.debian.org/debian trixie main contrib non-free non-free-firmware
deb http://deb.debian.org/debian trixie-updates main contrib non-free non-free-firmware
deb http://security.debian.org/debian-security/ trixie-security main contrib non-free non-free-firmware
deb http://deb.debian.org/debian trixie-backports main contrib non-free non-free-firmware
`

const zramConfig = `[zram0]
zram-size = min(ram / 2, 8192)
compression-algorithm = zstd
swap-priority = 100
fs-type = swap
`

const resolvedConfig = `[Resolve]
DNS=1.1.1.2#security.cloudflare-dns.com 1.0.0.2#security.cloudflare-dns.com 2606:4700:4700::1112#security.cloudflare-dns.com 2606:4700:4700::1002#security.cloudflare-dns.com
FallbackDNS=9.9.9.9#dns.quad9.net 149.112.112.112#dns.quad9.net 2620:fe::fe#dns.quad9.net
DNSOverTLS=yes
DNSSEC=yes
DNSStubListener=yes
MulticastDNS=no
Cache=yes
Domains=~.
`

const nmDNSConfig = `[main]
dns=systemd-resolved
`

const timesyncdConfig = `[Time]
NTP=time.cloudflare.com
FallbackNTP=time.google.com 0.debian.pool.ntp.org 1.debian.pool.ntp.org 2.debian.pool.ntp.org 3.debian.pool.ntp.org
`

const audioLimitsConfig = `@audio - rtprio 99
`
