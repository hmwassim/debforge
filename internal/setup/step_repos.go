package setup

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/hmwassim/debforge/internal/aptpty"
	"github.com/hmwassim/debforge/internal/domain/installer"
)

type RepoSource struct {
	Path    string
	Content string
}

type ReposStep struct {
	Sources []RepoSource
}

func (s *ReposStep) Name() string {
	return "Configure Debian repositories"
}

func (s *ReposStep) Check(ctx context.Context, cx *Context) CheckResult {
	for _, src := range s.Sources {
		action := installer.DecideConfigAction(cx.Fsys, src.Path, src.Content, cx.ConfigHashes[src.Path], false)

		exists, _ := cx.Fsys.Exists(src.Path)
		switch {
		case action == installer.ConfigWrite && !exists:
			return CheckResult{Status: StatusMissing, Summary: fmt.Sprintf("%s does not exist", src.Path)}
		case action == installer.ConfigWrite && exists:
			continue
		case action == installer.ConfigSkip:
			return CheckResult{Status: StatusDrifted, Summary: fmt.Sprintf("%s modified by user", src.Path)}
		case action == installer.ConfigConflict:
			return CheckResult{Status: StatusConflict, Summary: fmt.Sprintf("%s: local changes conflict with new defaults", src.Path)}
		}
	}
	return CheckResult{Status: StatusSatisfied}
}

func (s *ReposStep) Apply(ctx context.Context, cx *Context, result CheckResult) error {
	needsUpdate := false

	for _, src := range s.Sources {
		force := cx.Force
		if result.Status == StatusDrifted {
			force = false
		}

		action := installer.DecideConfigAction(cx.Fsys, src.Path, src.Content, cx.ConfigHashes[src.Path], force)

		switch action {
		case installer.ConfigWrite:
			dir := filepath.Dir(src.Path)
			if err := cx.Fsys.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("create dir %s: %w", dir, err)
			}
			if err := cx.Fsys.WriteFile(src.Path, []byte(src.Content), 0644); err != nil {
				return fmt.Errorf("write %s: %w", src.Path, err)
			}
			cx.ConfigHashes[src.Path] = installer.Sha256Hex([]byte(src.Content))
			needsUpdate = true

		case installer.ConfigSkip:
			diskData, err := cx.Fsys.ReadFile(src.Path)
			if err == nil && diskData != nil {
				cx.ConfigHashes[src.Path] = installer.Sha256Hex(diskData)
			}

		case installer.ConfigConflict:
			sidecar := src.Path + ".debforge-new"
			if err := cx.Fsys.WriteFile(sidecar, []byte(src.Content), 0644); err != nil {
				return fmt.Errorf("write sidecar %s: %w", sidecar, err)
			}
			cx.UI.Warn("%s has local changes; new version saved as %s", src.Path, sidecar)
		}
	}

	if needsUpdate {
		cx.UI.Info("  refreshing apt cache...")
		if err := aptpty.RunUpdate(ctx, cx.Runner, cx.UI.Spinner(ctx, "apt update")); err != nil {
			return fmt.Errorf("apt-get update: %w", err)
		}
	}

	return nil
}
