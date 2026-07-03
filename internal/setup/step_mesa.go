package setup

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/aptpty"
)

var mesaPackages = []string{
	"intel-media-va-driver-non-free", "intel-media-va-driver-non-free:i386",
	"mesa-va-drivers", "mesa-va-drivers:i386",
	"mesa-vulkan-drivers", "mesa-vulkan-drivers:i386",
	"libva2", "libva2:i386", "libvulkan1", "libvulkan1:i386",
	"libglx-mesa0:i386", "libgl1-mesa-dri:i386",
	"vulkan-tools", "vulkan-validationlayers", "vainfo", "vdpauinfo",
}

type MesaStep struct{}

func (s *MesaStep) Name() string {
	return "Installed Mesa GPU drivers"
}

func (s *MesaStep) Check(ctx context.Context, cx *Context) CheckResult {
	ok, err := allInstalled(ctx, cx.Runner, mesaPackages)
	if err != nil {
		return CheckResult{Status: StatusError, Summary: fmt.Sprintf("dpkg query failed: %s", err)}
	}
	if !ok {
		return CheckResult{Status: StatusMissing, Summary: "Mesa packages not installed"}
	}
	return CheckResult{Status: StatusSatisfied}
}

func (s *MesaStep) Apply(ctx context.Context, cx *Context, result CheckResult) error {
	spinner := cx.UI.Spinner(ctx, "Installing Mesa GPU drivers")
	defer spinner.Stop()
	return aptpty.RunInstall(ctx, cx.Runner, mesaPackages, spinner)
}
