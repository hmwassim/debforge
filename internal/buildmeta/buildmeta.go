package buildmeta

import (
	"context"
	"strings"

	"github.com/hmwassim/debforge/internal/ports"
)

const DefaultVersion = "0.1.0-dev"

func DeriveVersion(ctx context.Context, runner ports.CommandRunner, sourceDir string) string {
	v, _, err := runner.Run(ctx, "git", "-C", sourceDir, "describe", "--tags", "--always")
	if err != nil {
		return DefaultVersion
	}
	if s := strings.TrimSpace(string(v)); s != "" {
		return s
	}
	return DefaultVersion
}

func Ldflags(version string) string {
	return "-X main.version=" + version
}
