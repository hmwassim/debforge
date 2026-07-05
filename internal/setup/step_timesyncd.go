package setup

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/aptpty"
)

var timesyncdPackages = []string{"systemd-timesyncd"}

var timesyncdConfigFiles = []ConfigFile{
	{
		Path: "/etc/systemd/timesyncd.conf.d/10-timesyncd.conf",
		Content: `[Time]
NTP=time.cloudflare.com
FallbackNTP=time.google.com 0.debian.pool.ntp.org 1.debian.pool.ntp.org 2.debian.pool.ntp.org 3.debian.pool.ntp.org
`,
	},
}

type TimesyncdStep struct{}

func (s *TimesyncdStep) Name() string {
	return "Configured NTP time sync"
}

func (s *TimesyncdStep) Check(ctx context.Context, cx *Context) CheckResult {
	if r := checkStepPackages(ctx, cx, timesyncdPackages, "systemd-timesyncd not installed"); r.Status != StatusSatisfied {
		return r
	}
	return checkConfigFiles(cx, timesyncdConfigFiles)
}

func (s *TimesyncdStep) Apply(ctx context.Context, cx *Context, result CheckResult) error {
	initialDesc := "Configuring systemd-timesyncd"
	if result.Status == StatusMissing {
		initialDesc = "Installing systemd-timesyncd"
	}
	spinner := cx.UI.Spinner(ctx, initialDesc)
	defer spinner.Stop()

	if result.Status == StatusMissing {
		if err := aptpty.RunInstall(ctx, cx.Runner, timesyncdPackages, spinner); err != nil {
			return err
		}
		spinner.SetDesc("Configuring systemd-timesyncd")
	}

	if err := processConfigFiles(cx, timesyncdConfigFiles, result); err != nil {
		return err
	}

	if _, _, err := cx.Runner.Run(ctx, "systemctl", "enable", "--now", "systemd-timesyncd"); err != nil {
		return fmt.Errorf("enable timesyncd: %w", err)
	}
	if _, _, err := cx.Runner.Run(ctx, "timedatectl", "set-ntp", "true"); err != nil {
		return fmt.Errorf("set-ntp: %w", err)
	}
	return nil
}
