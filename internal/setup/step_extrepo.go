package setup

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/aptpty"
)

type ExtrepoStep struct{}

func (s *ExtrepoStep) Name() string {
	return "Installed extrepo"
}

func (s *ExtrepoStep) Check(ctx context.Context, cx *Context) CheckResult {
	ok, err := allInstalled(ctx, cx.Runner, []string{"extrepo"})
	if err != nil {
		return CheckResult{Status: StatusError, Summary: fmt.Sprintf("dpkg query failed: %s", err)}
	}
	if !ok {
		return CheckResult{Status: StatusMissing, Summary: "extrepo not installed"}
	}
	return CheckResult{Status: StatusSatisfied}
}

func (s *ExtrepoStep) Apply(ctx context.Context, cx *Context, result CheckResult) error {
	spinner := cx.UI.Spinner(ctx, "Installing extrepo")
	defer spinner.Stop()
	return aptpty.RunInstall(ctx, cx.Runner, []string{"extrepo"}, spinner)
}
