package setup

import (
	"context"
	"errors"
	"strings"

	"github.com/hmwassim/debforge/internal/ports"
)

func allInstalled(ctx context.Context, runner ports.CommandRunner, names []string) (bool, error) {
	if len(names) == 0 {
		return true, nil
	}
	args := []string{"-W", "-f=${db:Status-Status}\n"}
	args = append(args, names...)
	out, _, err := runner.Run(ctx, "dpkg-query", args...)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return false, err
		}
		return false, nil
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "installed" {
			return false, nil
		}
	}
	return true, nil
}
