package setup

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/aptpty"
)

var devtoolsPackages = []string{
	"git", "curl", "wget", "unzip", "p7zip-full", "gzip",
	"build-essential", "pkg-config", "cmake",
	"nvme-cli", "smartmontools", "pciutils", "usbutils",
	"cabextract", "zenity", "jq", "lm-sensors", "ddcutil",
	"hunspell-en-us", "hunspell-fr",
	"hwloc",
}

type DevtoolsStep struct{}

func (s *DevtoolsStep) Name() string {
	return "Installed core development tools"
}

func (s *DevtoolsStep) Check(ctx context.Context, cx *Context) CheckResult {
	ok, err := allInstalled(ctx, cx.Runner, devtoolsPackages)
	if err != nil {
		return CheckResult{Status: StatusError, Summary: fmt.Sprintf("dpkg query failed: %s", err)}
	}
	if !ok {
		return CheckResult{Status: StatusMissing, Summary: "dev tools not installed"}
	}
	return CheckResult{Status: StatusSatisfied}
}

func (s *DevtoolsStep) Apply(ctx context.Context, cx *Context, result CheckResult) error {
	spinner := cx.UI.Spinner(ctx, "Installing core development tools")
	defer spinner.Stop()
	return aptpty.RunInstall(ctx, cx.Runner, devtoolsPackages, spinner)
}
