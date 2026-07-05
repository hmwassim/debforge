package setup

import (
	"context"

	"github.com/hmwassim/debforge/internal/aptpty"
)

var firmwarePackages = []string{
	"firmware-linux",
	"firmware-linux-nonfree",
	"firmware-misc-nonfree",
	"firmware-iwlwifi",
	"firmware-sof-signed",
	"firmware-realtek",
	"intel-microcode",
}

type FirmwareStep struct{}

func (s *FirmwareStep) Name() string {
	return "Installed firmware"
}

func (s *FirmwareStep) Check(ctx context.Context, cx *Context) CheckResult {
	return checkStepPackages(ctx, cx, firmwarePackages, "firmware packages not installed")
}

func (s *FirmwareStep) Apply(ctx context.Context, cx *Context, result CheckResult) error {
	spinner := cx.UI.Spinner(ctx, "Installing firmware")
	defer spinner.Stop()
	return aptpty.RunInstall(ctx, cx.Runner, firmwarePackages, spinner)
}
