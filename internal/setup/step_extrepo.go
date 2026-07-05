package setup

import (
	"context"
	"fmt"

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
	ok, err := allInstalled(ctx, cx.Runner, []string{"extrepo"})
	if err != nil {
		return CheckResult{Status: StatusError, Summary: fmt.Sprintf("dpkg query failed: %s", err)}
	}
	if !ok {
		return CheckResult{Status: StatusMissing, Summary: "extrepo not installed"}
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
