package setup

import (
	"context"
	"fmt"
	"strings"

	"github.com/hmwassim/debforge/internal/aptpty"
	"github.com/hmwassim/debforge/internal/ports"
)

var baseDesktopPackages = []string{
	"eza", "starship", "papirus-icon-theme", "fastfetch", "bat", "ripgrep",
	"flatpak", "xdg-desktop-portal",
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
	return CheckResult{Status: StatusSatisfied}
}

func (s *DesktopStep) Apply(ctx context.Context, cx *Context, result CheckResult) error {
	spinner := cx.UI.Spinner(ctx, "Installing desktop tools")
	defer spinner.Stop()
	return aptpty.RunInstall(ctx, cx.Runner, desktopPackages(cx.Sys), spinner)
}
