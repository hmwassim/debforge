package setup

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/aptpty"
)

var zramPackages = []string{"systemd-zram-generator"}

var zramConfigFiles = []ConfigFile{
	{
		Path: "/etc/systemd/zram-generator.conf",
		Content: `[zram0]
zram-size = min(ram / 2, 8192)
compression-algorithm = zstd
swap-priority = 100
fs-type = swap
`,
	},
}

type ZramStep struct{}

func (s *ZramStep) Name() string {
	return "Configured zram swap"
}

func (s *ZramStep) Check(ctx context.Context, cx *Context) CheckResult {
	if r := checkStepPackages(ctx, cx, zramPackages, "zram-generator not installed"); r.Status != StatusSatisfied {
		return r
	}
	return checkConfigFiles(cx, zramConfigFiles)
}

func (s *ZramStep) Apply(ctx context.Context, cx *Context, result CheckResult) error {
	initialDesc := "Configuring zram-generator"
	if result.Status == StatusMissing {
		initialDesc = "Installing zram-generator"
	}
	spinner := cx.UI.Spinner(ctx, initialDesc)
	defer spinner.Stop()

	if result.Status == StatusMissing {
		if err := aptpty.RunInstall(ctx, cx.Runner, zramPackages, spinner); err != nil {
			return err
		}
		spinner.SetDesc("Configuring zram-generator")
	}

	if err := processConfigFiles(cx, zramConfigFiles, result); err != nil {
		return err
	}

	if _, _, err := cx.Runner.Run(ctx, "systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("daemon-reload: %w", err)
	}
	if _, _, err := cx.Runner.Run(ctx, "systemctl", "start", "systemd-zram-setup@zram0.service"); err != nil {
		return fmt.Errorf("start zram: %w", err)
	}
	return nil
}
