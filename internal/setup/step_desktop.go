package setup

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hmwassim/debforge/internal/aptpty"
	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/ports"
)

const (
	bashrcDStartMarker = "# >>> debforge bashrc.d loader >>>\n"
	bashrcDEndMarker   = "# <<< debforge bashrc.d loader <<<\n"
)

var bashrcDBlock = []byte(bashrcDStartMarker + `if [ -d "$HOME/.config/bashrc.d" ]; then
    for file in "$HOME/.config/bashrc.d"/*.sh; do
        [ -f "$file" ] && . "$file"
    done
fi
` + bashrcDEndMarker)

var baseDesktopPackages = []string{
	"eza", "starship", "papirus-icon-theme", "fastfetch", "bat", "ripgrep",
	"flatpak", "xdg-desktop-portal", "fzf",
}

type DesktopStep struct{}

func (s *DesktopStep) Name() string {
	return "Installed desktop tools"
}

func desktopPackages(sys ports.System) []string {
	pkgs := make([]string, len(baseDesktopPackages))
	copy(pkgs, baseDesktopPackages)
	de := sys.Getenv("XDG_CURRENT_DESKTOP")
	switch {
	case strings.Contains(strings.ToLower(de), "kde"),
		strings.Contains(strings.ToLower(de), "plasma"):
		pkgs = append(pkgs, "plasma-discover-backend-flatpak", "xdg-desktop-portal-kde")
	case strings.Contains(strings.ToLower(de), "gnome"):
		pkgs = append(pkgs, "gnome-software-plugin-flatpak", "xdg-desktop-portal-gnome")
	}
	return pkgs
}

func (s *DesktopStep) Check(ctx context.Context, cx *Context) CheckResult {
	ok, err := allInstalled(ctx, cx.Runner, desktopPackages(cx.Sys))
	if err != nil {
		return CheckResult{Status: StatusError, Summary: fmt.Sprintf("dpkg query failed: %s", err)}
	}
	if !ok {
		return CheckResult{Status: StatusMissing, Summary: "desktop packages not installed"}
	}

	homeDir, err := installer.UserHomeDir(cx.Sys)
	if err != nil {
		return CheckResult{Status: StatusError, Summary: fmt.Sprintf("home dir: %s", err)}
	}
	bashrcDDir := filepath.Join(homeDir, ".config", "bashrc.d")
	bashrc := filepath.Join(homeDir, ".bashrc")

	exists, _ := cx.Fsys.Exists(bashrcDDir)
	if !exists {
		return CheckResult{Status: StatusMissing, Summary: "bashrc.d directory does not exist"}
	}

	data, err := cx.Fsys.ReadFile(bashrc)
	if err != nil || !bytes.Contains(data, bashrcDBlock) {
		return CheckResult{Status: StatusMissing, Summary: "bashrc.d loader not found in .bashrc"}
	}

	return CheckResult{Status: StatusSatisfied}
}

func (s *DesktopStep) Apply(ctx context.Context, cx *Context, result CheckResult) error {
	initialDesc := "Configuring desktop"
	if result.Status == StatusMissing {
		initialDesc = "Installing desktop tools"
	}
	spinner := cx.UI.Spinner(ctx, initialDesc)
	defer spinner.Stop()

	if result.Status == StatusMissing {
		if err := aptpty.RunInstall(ctx, cx.Runner, desktopPackages(cx.Sys), spinner); err != nil {
			return err
		}
		spinner.SetDesc("Configuring desktop")
	}

	homeDir, err := installer.UserHomeDir(cx.Sys)
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	bashrcDDir := filepath.Join(homeDir, ".config", "bashrc.d")
	bashrc := filepath.Join(homeDir, ".bashrc")

	if err := cx.Fsys.MkdirAll(bashrcDDir, 0755); err != nil {
		return fmt.Errorf("create bashrc.d: %w", err)
	}

	data, err := cx.Fsys.ReadFile(bashrc)
	if err != nil {
		data = nil
	}

	start := bytes.Index(data, []byte(bashrcDStartMarker))
	end := bytes.Index(data, []byte(bashrcDEndMarker))

	var updated []byte
	if start != -1 && end != -1 {
		end += len(bashrcDEndMarker)
		updated = make([]byte, 0, len(data)+len(bashrcDBlock))
		updated = append(updated, data[:start]...)
		updated = append(updated, bashrcDBlock...)
		updated = append(updated, data[end:]...)
	} else {
		updated = make([]byte, 0, len(data)+len(bashrcDBlock)+1)
		updated = append(updated, data...)
		updated = append(updated, '\n')
		updated = append(updated, bashrcDBlock...)
	}

	cx.Runner.Run(ctx, "flatpak", "remote-add", "--if-not-exists",
		"flathub", "https://flathub.org/repo/flathub.flatpakrepo")

	return cx.Fsys.WriteFile(bashrc, updated, 0644)
}
