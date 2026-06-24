package installer

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/ports"
)

// RunScript executes a shell script via the runner, setting the spinner
// description to verb + name, and wraps errors with the same pattern.
func RunScript(ctx context.Context, runner ports.CommandRunner, spinner ports.Spinner, name, script, verb string) error {
	spinner.SetDesc(verb + " " + name)
	if _, _, err := runner.Run(ctx, "sh", "-c", script); err != nil {
		return fmt.Errorf("%s %s: %w", verb, name, err)
	}
	return nil
}
