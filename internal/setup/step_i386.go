package setup

import (
	"context"
	"fmt"
	"strings"

	"github.com/hmwassim/debforge/internal/aptpty"
)

type I386Step struct{}

func (s *I386Step) Name() string {
	return "Enable i386 architecture"
}

func (s *I386Step) Check(ctx context.Context, cx *Context) CheckResult {
	out, _, err := cx.Runner.Run(ctx, "dpkg", "--print-foreign-architectures")
	if err != nil {
		return CheckResult{Status: StatusError, Summary: fmt.Sprintf("dpkg query failed: %s", err)}
	}

	for _, arch := range strings.Fields(string(out)) {
		if arch == "i386" {
			return CheckResult{Status: StatusSatisfied}
		}
	}
	return CheckResult{Status: StatusMissing, Summary: "i386 not enabled"}
}

func (s *I386Step) Apply(ctx context.Context, cx *Context, result CheckResult) error {
	if _, _, err := cx.Runner.Run(ctx, "dpkg", "--add-architecture", "i386"); err != nil {
		return fmt.Errorf("dpkg --add-architecture i386: %w", err)
	}

	cx.UI.Info("  refreshing apt cache...")
	if err := aptpty.RunUpdate(ctx, cx.Runner, cx.UI.Spinner(ctx, "apt update")); err != nil {
		return fmt.Errorf("apt-get update: %w", err)
	}

	return nil
}
