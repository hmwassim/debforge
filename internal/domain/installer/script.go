package installer

import (
	"context"
	"fmt"
	"strings"

	"github.com/hmwassim/debforge/internal/ports"
)

// RunScript executes a shell script via the runner, setting the spinner
// description to verb + name, and wraps errors with the same pattern.
func RunScript(ctx context.Context, runner ports.CommandRunner, spinner ports.Spinner, name, script, verb string) error {
	spinner.SetDesc(verb + " " + name)
	if _, stderr, err := runner.Run(ctx, "sh", "-c", script); err != nil {
		return fmt.Errorf("%s %s: %w%s", verb, name, err, trimErr(stderr))
	}
	return nil
}

// RunScriptInDir is like RunScript but runs the script in the given directory.
func RunScriptInDir(ctx context.Context, runner ports.CommandRunner, spinner ports.Spinner, name, script, verb, dir string) error {
	spinner.SetDesc(verb + " " + name)
	if _, stderr, err := runner.RunWithOptions(ctx, ports.RunOptions{Dir: dir}, "sh", "-c", script); err != nil {
		return fmt.Errorf("%s %s: %w%s", verb, name, err, trimErr(stderr))
	}
	return nil
}

func trimErr(stderr []byte) string {
	out := strings.TrimSpace(string(stderr))
	if out == "" {
		return ""
	}
	if len(out) > 500 {
		out = out[:500] + "..."
	}
	return ": " + out
}
