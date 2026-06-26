// Package buildmeta derives version metadata from git and generates
// linker flags for embedding the version into the binary.
package buildmeta

import (
	"context"
	"strings"

	"github.com/hmwassim/debforge/internal/ports"
)

// DefaultVersion is the fallback version when git describe fails.
const DefaultVersion = "0.1.0-dev"

// DeriveVersion returns the git describe output for sourceDir, falling
// back to DefaultVersion on any error.
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

// Ldflags returns -X linker flags for the given version.
func Ldflags(version string) string {
	return "-X main.version=" + version
}
