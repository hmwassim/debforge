package setup

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/hmwassim/debforge/internal/aptpty"
	"github.com/hmwassim/debforge/internal/domain/installer"
)

type zramConfig struct {
	Path    string
	Content string
}

var zramPackages = []string{"systemd-zram-generator"}

var zramConfigFiles = []zramConfig{
	{
		Path: "/etc/systemd/zram-generator.conf",
		Content: `[zram0]
zram-size = min(ram / 2, 8192)
compression-algorithm = zstd
swap-priority = 100
fs-type = swap
`,
	},
}

type ZramStep struct{}

func (s *ZramStep) Name() string {
	return "Configured zram swap"
}

func (s *ZramStep) Check(ctx context.Context, cx *Context) CheckResult {
	ok, err := allInstalled(ctx, cx.Runner, zramPackages)
	if err != nil {
		return CheckResult{Status: StatusError, Summary: fmt.Sprintf("dpkg query failed: %s", err)}
	}
	if !ok {
		return CheckResult{Status: StatusMissing, Summary: "zram-generator not installed"}
	}

	for _, cfg := range zramConfigFiles {
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

func (s *ZramStep) Apply(ctx context.Context, cx *Context, result CheckResult) error {
	if result.Status == StatusMissing {
		spinner := cx.UI.Spinner(ctx, "Installing zram-generator")
		if err := aptpty.RunInstall(ctx, cx.Runner, zramPackages, spinner); err != nil {
			spinner.Stop()
			return err
		}
		spinner.Stop()
	}

	for _, cfg := range zramConfigFiles {
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

	cx.Runner.Run(ctx, "systemctl", "daemon-reload")
	cx.Runner.Run(ctx, "systemctl", "start", "systemd-zram-setup@zram0.service")
	return nil
}
