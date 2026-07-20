// Package extrepo provides an adapter that satisfies the ports.ExtrepoManager
// interface by delegating to the extrepo CLI.
package extrepo

import (
	"context"
	"fmt"
	"strings"

	"github.com/hmwassim/debforge/internal/ports"
)

// Manager satisfies ports.ExtrepoManager by running extrepo commands
// and reading source files.
type Manager struct {
	Runner ports.CommandRunner
	Fs     ports.FileSystem
}

// NeedsEnable reports whether the given extrepo source file needs to be enabled.
// Returns false for repo names containing "/" or ".." to prevent path
// traversal, and for repos whose sources file already has Enabled: yes
// (or has no Enabled line at all, which defaults to enabled).
func (m *Manager) NeedsEnable(ctx context.Context, repo string) (bool, error) {
	if strings.Contains(repo, "/") || strings.Contains(repo, "..") {
		return false, nil
	}
	path := "/etc/apt/sources.list.d/extrepo_" + repo + ".sources"
	exists, err := m.Fs.Exists(path)
	if err != nil {
		return false, err
	}
	if !exists {
		return true, nil
	}
	data, err := m.Fs.ReadFile(path)
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
func (m *Manager) Enable(ctx context.Context, repo string, spinner ports.Spinner) error {
	spinner.SetDesc("enabling extrepo " + repo)
	if _, _, err := m.Runner.Run(ctx, "extrepo", "enable", repo); err != nil {
		return fmt.Errorf("enable extrepo %q: %w", repo, err)
	}
	return nil
}
