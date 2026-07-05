package setup

import (
	"context"

	"github.com/hmwassim/debforge/internal/aptpty"
)

var devtoolsPackages = []string{
	"git", "curl", "wget", "unzip", "p7zip-full", "gzip", "gnupg",
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
	return checkStepPackages(ctx, cx, devtoolsPackages, "dev tools not installed")
}

func (s *DevtoolsStep) Apply(ctx context.Context, cx *Context, result CheckResult) error {
	spinner := cx.UI.Spinner(ctx, "Installing core development tools")
	defer spinner.Stop()
	return aptpty.RunInstall(ctx, cx.Runner, devtoolsPackages, spinner)
}
