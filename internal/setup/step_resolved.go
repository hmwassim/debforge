package setup

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/hmwassim/debforge/internal/aptpty"
	"github.com/hmwassim/debforge/internal/domain/installer"
)

type resolvedConfig struct {
	Path    string
	Content string
}

var resolvedPackages = []string{"systemd-resolved"}

var resolvedConfigFiles = []resolvedConfig{
	{
		Path: "/etc/systemd/resolved.conf.d/99-dot.conf",
		Content: `[Resolve]
DNS=1.1.1.1 1.0.0.1
DNSOverTLS=yes
DNSSEC=allow-downgrade
`,
	},
	{
		Path: "/etc/NetworkManager/conf.d/10-dns.conf",
		Content: `[main]
dns=systemd-resolved
`,
	},
}

type ResolvedStep struct{}

func (s *ResolvedStep) Name() string {
	return "Configured DNS-over-TLS"
}

func (s *ResolvedStep) Check(ctx context.Context, cx *Context) CheckResult {
	ok, err := allInstalled(ctx, cx.Runner, resolvedPackages)
	if err != nil {
		return CheckResult{Status: StatusError, Summary: fmt.Sprintf("dpkg query failed: %s", err)}
	}
	if !ok {
		return CheckResult{Status: StatusMissing, Summary: "systemd-resolved not installed"}
	}

	for _, cfg := range resolvedConfigFiles {
		action := installer.DecideConfigAction(cx.Fsys, cfg.Path, cfg.Content, cx.ConfigHashes[cfg.Path], false)
		exists, _ := cx.Fsys.Exists(cfg.Path)
		switch {
		case action == installer.ConfigWrite && !exists:
			return CheckResult{Status: StatusMissing, Summary: fmt.Sprintf("%s does not exist", cfg.Path)}
		case action == installer.ConfigWrite && exists:
			continue
		case action == installer.ConfigSkip:
			return CheckResult{Status: StatusDrifted, Summary: fmt.Sprintf("%s modified by user", cfg.Path)}
		case action == installer.ConfigConflict:
			return CheckResult{Status: StatusConflict, Summary: fmt.Sprintf("%s: local changes conflict with new defaults", cfg.Path)}
		}
	}

	return CheckResult{Status: StatusSatisfied}
}

func (s *ResolvedStep) Apply(ctx context.Context, cx *Context, result CheckResult) error {
	if result.Status == StatusMissing {
		spinner := cx.UI.Spinner(ctx, "Installing systemd-resolved")
		if err := aptpty.RunInstall(ctx, cx.Runner, resolvedPackages, spinner); err != nil {
			spinner.Stop()
			return err
		}
		spinner.Stop()
	}

	for _, cfg := range resolvedConfigFiles {
		force := cx.Force
		if result.Status == StatusDrifted {
			force = false
		}

		action := installer.DecideConfigAction(cx.Fsys, cfg.Path, cfg.Content, cx.ConfigHashes[cfg.Path], force)

		switch action {
		case installer.ConfigWrite:
			dir := filepath.Dir(cfg.Path)
			if err := cx.Fsys.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("create dir %s: %w", dir, err)
			}
			if err := cx.Fsys.WriteFile(cfg.Path, []byte(cfg.Content), 0644); err != nil {
				return fmt.Errorf("write %s: %w", cfg.Path, err)
			}
			cx.ConfigHashes[cfg.Path] = installer.Sha256Hex([]byte(cfg.Content))

		case installer.ConfigSkip:
			diskData, err := cx.Fsys.ReadFile(cfg.Path)
			if err == nil && diskData != nil {
				cx.ConfigHashes[cfg.Path] = installer.Sha256Hex(diskData)
			}

		case installer.ConfigConflict:
			sidecar := cfg.Path + ".debforge-new"
			if err := cx.Fsys.WriteFile(sidecar, []byte(cfg.Content), 0644); err != nil {
				return fmt.Errorf("write sidecar %s: %w", sidecar, err)
			}
			cx.UI.Warn("%s has local changes; new version saved as %s", cfg.Path, sidecar)
		}
	}

	cx.Runner.Run(ctx, "ln", "-sf", "/run/systemd/resolve/stub-resolv.conf", "/etc/resolv.conf")
	cx.Runner.Run(ctx, "systemctl", "enable", "--now", "systemd-resolved")
	cx.Runner.Run(ctx, "nmcli", "general", "reload")
	cx.Runner.Run(ctx, "systemctl", "restart", "systemd-resolved")

	for i := 0; i < 15; i++ {
		_, _, err := cx.Runner.Run(ctx, "resolvectl", "query", "debian.org")
		if err == nil {
			break
		}
		time.Sleep(2 * time.Second)
	}

	return nil
}
