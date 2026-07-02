package setup

import (
	"context"
	"fmt"
	"strings"
)

type I386Step struct{}

func (s *I386Step) Name() string {
	return "Enabled i386 architecture"
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
	return nil
}
