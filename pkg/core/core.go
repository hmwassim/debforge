package core

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/hmwassim/debforge/pkg/executil"
	"github.com/hmwassim/debforge/pkg/packages"
	"github.com/hmwassim/debforge/pkg/settings"
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
	name        string
	packages    []string
	backport    bool
	configs     []configFile
	services    []string
	postInstall func(*text.Logger) error
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
		services: []string{}, // user services (pipewire, wireplumber) handled per-user
	},
	{
		name: "flatpak",
		packages: []string{
			"flatpak",
		},
		postInstall: installFlathub,
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

		if g.postInstall != nil {
			if err := g.postInstall(log); err != nil {
				return fmt.Errorf("post-install %s: %w", g.name, err)
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

	var defaultPkgs, backportPkgs []string
	for _, g := range groups {
		if g.backport {
			backportPkgs = append(backportPkgs, g.packages...)
		} else {
			defaultPkgs = append(defaultPkgs, g.packages...)
		}
	}

	if len(defaultPkgs) > 0 {
		args := append([]string{"install", "-y"}, defaultPkgs...)
		if err := executil.Run(exec.Command("apt", args...)); err != nil {
			return fmt.Errorf("upgrading core: %w", err)
		}
	}
	if len(backportPkgs) > 0 {
		args := append([]string{"install", "-y", "-t", "trixie-backports"}, backportPkgs...)
		if err := executil.Run(exec.Command("apt", args...)); err != nil {
			return fmt.Errorf("upgrading backports: %w", err)
		}
	}

	log.Success("Core packages up to date")
	return nil
}

func List(log *text.Logger) {
	log.Info("Core packages:")

	for _, g := range groups {
		var missing []string
		for _, pkg := range g.packages {
			if !isInstalled(pkg) {
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

func isInstalled(pkg string) bool {
	out, err := exec.Command("dpkg", "--get-selections", pkg).Output()
	return err == nil && strings.Contains(string(out), "\tinstall")
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

func installCodebergFonts(log *text.Logger) error {
	cachePath := settings.Default.CacheDir() + "/fonts.tar.gz"
	fontDir := "/usr/local/share/fonts"

	if _, err := os.Stat(cachePath); err == nil {
		log.Info("Using cached fonts...")
		if err := extractFonts(cachePath, fontDir); err == nil {
			return nil
		}
		log.Warn("Cached fonts are corrupt, re-downloading...")
		os.Remove(cachePath)
	}

	log.Info("Downloading custom fonts...")
	if err := os.MkdirAll(settings.Default.CacheDir(), 0755); err != nil {
		return err
	}

	if err := downloadFile(cachePath, "https://codeberg.org/hmwassim/fonts/raw/branch/main/fonts.tar.gz"); err != nil {
		return fmt.Errorf("downloading fonts: %w", err)
	}

	return extractFonts(cachePath, fontDir)
}

func downloadFile(path, url string) error {
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP status: %s", resp.Status)
	}

	total := resp.ContentLength
	if total <= 0 {
		_, err = io.Copy(out, resp.Body)
		return err
	}

	start := time.Now()
	pb := &progressWriter{total: total, start: start}
	if _, err := io.Copy(out, io.TeeReader(resp.Body, pb)); err != nil {
		return err
	}
	pb.done()
	fmt.Fprintln(os.Stderr)
	return nil
}

type progressWriter struct {
	total     int64
	current   int64
	start     time.Time
	lastPrint time.Time
}

func (w *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	w.current += int64(n)
	if time.Since(w.lastPrint) < 100*time.Millisecond {
		return n, nil
	}
	w.lastPrint = time.Now()
	w.print()
	return n, nil
}

func (w *progressWriter) done() {
	w.current = w.total
	w.print()
}

func (w *progressWriter) print() {
	pct := float64(w.current) / float64(w.total) * 100
	barWidth := 40
	filled := int(float64(barWidth) * float64(w.current) / float64(w.total))
	bar := strings.Repeat("=", filled) + strings.Repeat("-", barWidth-filled)
	if filled < barWidth {
		bar = bar[:filled] + ">" + bar[filled+1:]
	}
	elapsed := time.Since(w.start)
	rate := float64(w.current) / elapsed.Seconds()
	var etaStr string
	if rate > 0 {
		remaining := time.Duration(float64(w.total-w.current)/rate) * time.Second
		etaStr = remaining.Truncate(time.Second).String()
	} else {
		etaStr = "?"
	}
	fmt.Fprintf(os.Stderr, "\033[2K\r  [%s] %3.0f%%  ETA %s", bar, pct, etaStr)
}

func extractFonts(path, fontDir string) error {
	if err := os.MkdirAll(fontDir, 0755); err != nil {
		return err
	}
	extract := exec.Command("tar", "-xzf", path, "-C", fontDir)
	extract.Stdout = nil
	if err := executil.Run(extract); err != nil {
		return fmt.Errorf("extracting fonts: %w", err)
	}
	return executil.Run(exec.Command("fc-cache", "-f", "-v"))
}

func deployConfig(cf configFile) error {
	dir := cf.dest[:strings.LastIndex(cf.dest, "/")]
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(cf.dest, []byte(cf.content), cf.mode)
}

func installFlathub(log *text.Logger) error {
	log.Info("Adding Flathub remote...")
	return executil.Run(exec.Command("flatpak", "remote-add", "--if-not-exists", "flathub", "https://flathub.org/repo/flathub.flatpakrepo"))
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

const fontConfig = `<?xml version="1.0"?>
<!DOCTYPE fontconfig SYSTEM "urn:fontconfig:fonts.dtd">
<fontconfig>

  <selectfont>
    <rejectfont>
      <glob>*NotoNastaliq*</glob>
    </rejectfont>
  </selectfont>

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

  <match target="pattern">
    <test name="lang" compare="contains"><string>ar</string></test>
    <edit name="family" mode="prepend" binding="strong">
      <string>Noto Sans Arabic</string>
    </edit>
  </match>

  <match target="pattern">
    <test name="lang" compare="contains"><string>ar</string></test>
    <test name="spacing" compare="eq"><int>100</int></test>
    <edit name="family" mode="prepend" binding="strong">
      <string>Noto Sans Arabic UI</string>
    </edit>
  </match>

  <match target="pattern">
    <test name="family"><string>emoji</string></test>
    <edit name="family" mode="prepend" binding="strong">
      <string>Noto Color Emoji</string>
    </edit>
  </match>

  <match target="pattern">
    <test name="lang" compare="contains"><string>zh</string></test>
    <edit name="family" mode="append" binding="weak">
      <string>Noto Sans CJK SC</string>
    </edit>
  </match>
  <match target="pattern">
    <test name="lang" compare="contains"><string>ja</string></test>
    <edit name="family" mode="append" binding="weak">
      <string>Noto Sans CJK JP</string>
    </edit>
  </match>
  <match target="pattern">
    <test name="lang" compare="contains"><string>ko</string></test>
    <edit name="family" mode="append" binding="weak">
      <string>Noto Sans CJK KR</string>
    </edit>
  </match>

</fontconfig>
`
