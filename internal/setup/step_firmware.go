package setup

import (
	"context"
	"fmt"

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
	ok, err := allInstalled(ctx, cx.Runner, firmwarePackages)
	if err != nil {
		return CheckResult{Status: StatusError, Summary: fmt.Sprintf("dpkg query failed: %s", err)}
	}
	if !ok {
		return CheckResult{Status: StatusMissing, Summary: "firmware packages not installed"}
	}
	return CheckResult{Status: StatusSatisfied}
}

func (s *FirmwareStep) Apply(ctx context.Context, cx *Context, result CheckResult) error {
	spinner := cx.UI.Spinner(ctx, "Installing firmware")
	defer spinner.Stop()
	return aptpty.RunInstall(ctx, cx.Runner, firmwarePackages, spinner)
}
