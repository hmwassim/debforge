package setup

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/hmwassim/debforge/internal/aptpty"
	"github.com/hmwassim/debforge/internal/domain/installer"
)

type extrepoConfig struct {
	Path    string
	Content string
}

var extrepoConfigFiles = []extrepoConfig{
	{
		Path: "/etc/extrepo/config.yaml",
		Content: `url: https://extrepo-team.pages.debian.net/extrepo-data
dist: debian
version: trixie
enabled_policies:
- main
- contrib
- non-free
`,
	},
}

type ExtrepoStep struct{}

func (s *ExtrepoStep) Name() string {
	return "Configured extrepo"
}

func (s *ExtrepoStep) Check(ctx context.Context, cx *Context) CheckResult {
	ok, err := allInstalled(ctx, cx.Runner, []string{"extrepo"})
	if err != nil {
		return CheckResult{Status: StatusError, Summary: fmt.Sprintf("dpkg query failed: %s", err)}
	}
	if !ok {
		return CheckResult{Status: StatusMissing, Summary: "extrepo not installed"}
	}

	for _, cfg := range extrepoConfigFiles {
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

func (s *ExtrepoStep) Apply(ctx context.Context, cx *Context, result CheckResult) error {
	if result.Status == StatusMissing {
		spinner := cx.UI.Spinner(ctx, "Installing extrepo")
		if err := aptpty.RunInstall(ctx, cx.Runner, []string{"extrepo"}, spinner); err != nil {
			spinner.Stop()
			return err
		}
		spinner.Stop()
	}

	for _, cfg := range extrepoConfigFiles {
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

	return nil
}
