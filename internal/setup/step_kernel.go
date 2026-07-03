package setup

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/aptpty"
)

var kernelPackages = []string{"linux-image-amd64", "linux-headers-amd64"}

type KernelStep struct{}

func (s *KernelStep) Name() string {
	return "Installed backported kernel"
}

func (s *KernelStep) Check(ctx context.Context, cx *Context) CheckResult {
	ok, err := allInstalled(ctx, cx.Runner, kernelPackages)
	if err != nil {
		return CheckResult{Status: StatusError, Summary: fmt.Sprintf("dpkg query failed: %s", err)}
	}
	if !ok {
		return CheckResult{Status: StatusMissing, Summary: "backported kernel not installed"}
	}
	return CheckResult{Status: StatusSatisfied}
}

func (s *KernelStep) Apply(ctx context.Context, cx *Context, result CheckResult) error {
	spinner := cx.UI.Spinner(ctx, "Installing backported kernel")
	defer spinner.Stop()
	return aptpty.RunInstallBackports(ctx, cx.Runner, kernelPackages, "trixie-backports", spinner)
}
