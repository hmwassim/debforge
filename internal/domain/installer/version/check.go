package version

import (
	"context"
	"fmt"
	"strings"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

// GatherVersion returns the latest version string from VersionCmd or
// Repo-based tag lookup. Returns ("", nil) when no version source is
// configured. Returns ("", err) when a configured source fails.
func GatherVersion(ctx context.Context, runner ports.CommandRunner, p *pkg.Package) (string, error) {
	if p.VersionCmd != "" {
		out, _, err := runner.Run(ctx, "sh", "-c", p.VersionCmd)
		if err != nil {
			return "", fmt.Errorf("version check %s: %w", p.Name, err)
		}
		return strings.TrimSpace(string(out)), nil
	}

	if repo := RepoFromPkg(p); repo != "" {
		return LatestTag(ctx, runner, repo, p.TagPrefix)
	}

	return "", nil
}

// ApplyVersionUpdate compares latest against p.Version and either
// short-circuits (returns false, nil when already up to date) or
// updates p.Version (returns true, nil when a newer version was found).
// Returns an error when latest is empty.
func ApplyVersionUpdate(spinner ports.Spinner, p *pkg.Package, latest string) (bool, error) {
	if latest == "" {
		return false, fmt.Errorf("version check %s: empty output", p.Name)
	}
	if latest == p.Version {
		spinner.SetDesc(p.Name + " already up to date")
		return false, nil
	}
	p.Version = latest
	return true, nil
}
