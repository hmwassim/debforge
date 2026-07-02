package setup

import (
	"context"

	"github.com/hmwassim/debforge/internal/aptpty"
)

type UpgradeStep struct{}

func (s *UpgradeStep) Name() string {
	return "Upgraded system packages"
}

func (s *UpgradeStep) Check(ctx context.Context, cx *Context) CheckResult {
	return CheckResult{Status: StatusMissing}
}

func (s *UpgradeStep) Apply(ctx context.Context, cx *Context, result CheckResult) error {
	spinner := cx.UI.Spinner(ctx, "Refreshing repositories")
	defer spinner.Stop()
	if err := aptpty.RunUpdate(ctx, cx.Runner, spinner); err != nil {
		return err
	}
	spinner.SetDesc("Upgrading packages")
	return aptpty.RunUpgrade(ctx, cx.Runner, spinner)
}
