package dpkg

import (
	"context"
	"strings"
	"sync"

	"github.com/hmwassim/debforge/internal/ports"
)

// CachedRunner wraps a CommandRunner and caches the full dpkg installed-package
// set after the first intercepted IsInstalled-style call, serving subsequent
// lookups from the cache without spawning additional dpkg-query processes.
type CachedRunner struct {
	inner ports.CommandRunner
	mu    sync.Mutex
	set   map[string]bool
}

// NewCachedRunner returns a CachedRunner wrapping inner.
func NewCachedRunner(inner ports.CommandRunner) *CachedRunner {
	return &CachedRunner{inner: inner}
}

func (c *CachedRunner) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	if !isInstalledQuery(name, args) {
		return c.inner.Run(ctx, name, args...)
	}

	c.mu.Lock()
	if c.set == nil {
		installed, err := ListInstalled(ctx, c.inner)
		if err != nil {
			c.mu.Unlock()
			return nil, nil, err
		}
		c.set = installed
	}
	c.mu.Unlock()

	pkgName := args[len(args)-1]
	if c.set[pkgName] {
		return []byte("installed\n"), nil, nil
	}
	return []byte("not-installed\n"), nil, nil
}

func (c *CachedRunner) RunWithOptions(ctx context.Context, opts ports.RunOptions, name string, args ...string) ([]byte, []byte, error) {
	if !isInstalledQuery(name, args) {
		return c.inner.RunWithOptions(ctx, opts, name, args...)
	}
	return c.Run(ctx, name, args...)
}

// isInstalledQuery matches dpkg-query -W -f=${db:Status-Status}\n <name>.
func isInstalledQuery(name string, args []string) bool {
	if name != "dpkg-query" || len(args) != 3 {
		return false
	}
	return args[0] == "-W" && strings.HasPrefix(args[1], "-f=${db:Status-Status}")
}
