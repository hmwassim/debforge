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
	spinnerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	spinner := cx.UI.Spinner(spinnerCtx, "Refreshing repositories")
	if err := aptpty.RunUpdate(ctx, cx.Runner, spinner); err != nil {
		return err
	}
	spinner.SetDesc("Upgrading packages")
	if err := aptpty.RunUpgrade(ctx, cx.Runner, spinner); err != nil {
		return err
	}
	return nil
}
