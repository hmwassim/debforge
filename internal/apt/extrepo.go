package apt

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/hmwassim/debforge/internal/system"
)

// Extrepo manages extrepo-provided sources.
type Extrepo struct {
	R system.Runner
	// SourcesDir is /etc/apt/sources.list.d (overridable in tests).
	SourcesDir string
}

func (e *Extrepo) file(name string) string {
	dir := e.SourcesDir
	if dir == "" {
		dir = "/etc/apt/sources.list.d"
	}
	return filepath.Join(dir, "extrepo_"+name+".sources")
}

// Enabled reports whether the repo's sources file exists and is not
// disabled.
func (e *Extrepo) Enabled(name string) bool {
	b, err := os.ReadFile(e.file(name))
	if err != nil {
		return false
	}
	for _, l := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(l, ":")
		if ok && strings.EqualFold(strings.TrimSpace(k), "Enabled") && strings.EqualFold(strings.TrimSpace(v), "no") {
			return false
		}
	}
	return true
}

// Enable runs extrepo enable.
func (e *Extrepo) Enable(ctx context.Context, name string) error {
	_, err := e.R.Run(ctx, system.Cmd{Name: "extrepo", Args: []string{"enable", name}})
	return err
}

// Disable runs extrepo disable.
func (e *Extrepo) Disable(ctx context.Context, name string) error {
	_, err := e.R.Run(ctx, system.Cmd{Name: "extrepo", Args: []string{"disable", name}})
	return err
}
