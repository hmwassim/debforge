package setup

import (
	"context"

	"github.com/hmwassim/debforge/internal/aptpty"
)

var extrepoConfigFiles = []ConfigFile{
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
	if r := checkStepPackages(ctx, cx, []string{"extrepo"}, "extrepo not installed"); r.Status != StatusSatisfied {
		return r
	}
	return checkConfigFiles(cx, extrepoConfigFiles)
}

func (s *ExtrepoStep) Apply(ctx context.Context, cx *Context, result CheckResult) error {
	initialDesc := "Configuring extrepo"
	if result.Status == StatusMissing {
		initialDesc = "Installing extrepo"
	}
	spinner := cx.UI.Spinner(ctx, initialDesc)
	defer spinner.Stop()

	if result.Status == StatusMissing {
		if err := aptpty.RunInstall(ctx, cx.Runner, []string{"extrepo"}, spinner); err != nil {
			return err
		}
		spinner.SetDesc("Configuring extrepo")
	}

	return processConfigFiles(cx, extrepoConfigFiles, result)
}
