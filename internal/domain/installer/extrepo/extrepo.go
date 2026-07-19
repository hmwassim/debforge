// Package extrepo provides a shared adapter for managing extrepo sources.
// It centralises the logic for checking whether an extrepo is already
// enabled and enabling it — previously duplicated between the service
// layer and the apt installer.
package extrepo

import (
	"context"
	"fmt"
	"strings"

	"github.com/hmwassim/debforge/internal/ports"
)

// NeedsEnable reports whether the given extrepo needs to be enabled.
// Returns false for repo names containing "/" or ".." to prevent path
// traversal, and for repos whose sources file already has Enabled: yes
// (or has no Enabled line at all, which defaults to enabled).
func NeedsEnable(ctx context.Context, repo string, fs ports.FileSystem) (bool, error) {
	if strings.Contains(repo, "/") || strings.Contains(repo, "..") {
		return false, nil
	}
	path := "/etc/apt/sources.list.d/extrepo_" + repo + ".sources"
	exists, err := fs.Exists(path)
	if err != nil {
		return false, err
	}
	if !exists {
		return true, nil
	}
	data, err := fs.ReadFile(path)
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Enabled:") {
			val := strings.TrimSpace(line[8:])
			return val == "no", nil
		}
	}
	return false, nil
}

// Enable runs "extrepo enable" for the given repo. The caller is
// responsible for running "apt-get update" after all repos are enabled.
func Enable(ctx context.Context, repo string, runner ports.CommandRunner, spinner ports.Spinner) error {
	spinner.SetDesc("enabling extrepo " + repo)
	if _, _, err := runner.Run(ctx, "extrepo", "enable", repo); err != nil {
		return fmt.Errorf("enable extrepo %q: %w", repo, err)
	}
	return nil
}
