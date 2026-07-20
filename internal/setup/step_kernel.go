package setup

import (
	"context"

	"github.com/hmwassim/debforge/internal/aptpty"
	"github.com/hmwassim/debforge/internal/domain/installer/apt"
)

var kernelPackages = []string{"linux-image-amd64", "linux-headers-amd64"}

type KernelStep struct{}

func (s *KernelStep) Name() string {
	return "Installed backported kernel"
}

func (s *KernelStep) Check(ctx context.Context, cx *Context) CheckResult {
	return checkStepPackages(ctx, cx, kernelPackages, "backported kernel not installed")
}

func (s *KernelStep) Apply(ctx context.Context, cx *Context, result CheckResult) error {
	spinner := cx.UI.Spinner(ctx, "Installing backported kernel")
	defer spinner.Stop()
	return aptpty.RunInstallBackports(ctx, cx.Runner, kernelPackages, apt.DefaultBackportSuite, spinner)
}
