// Package systemtest provides a scripted fake system.Runner for tests.
// Any command without a registered handler fails, so tests can never reach
// the real apt-get or dpkg by accident.
package systemtest

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/hmwassim/debforge/internal/system"
)

// Handler produces the result for a matched command.
type Handler func(c system.Cmd) (system.Result, error)

type rule struct {
	prefix  string
	handler Handler
}

// Runner is a fake system.Runner.
type Runner struct {
	mu    sync.Mutex
	rules []rule
	Calls []system.Cmd
}

// On registers a handler for commands whose "name args..." string starts
// with prefix. Later registrations take precedence.
func (r *Runner) On(prefix string, h Handler) *Runner {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rules = append(r.rules, rule{prefix: prefix, handler: h})
	return r
}

// OK registers a handler that succeeds with the given stdout.
func (r *Runner) OK(prefix, stdout string) *Runner {
	return r.On(prefix, func(system.Cmd) (system.Result, error) {
		return system.Result{Stdout: []byte(stdout)}, nil
	})
}

// Fail registers a handler that exits with code and stderr.
func (r *Runner) Fail(prefix string, code int, stderr string) *Runner {
	return r.On(prefix, func(c system.Cmd) (system.Result, error) {
		return system.Result{Stderr: []byte(stderr), ExitCode: code},
			&system.ExitError{Cmd: c.Name, Code: code, Stderr: stderr}
	})
}

// Run implements system.Runner.
func (r *Runner) Run(_ context.Context, c system.Cmd) (system.Result, error) {
	r.mu.Lock()
	r.Calls = append(r.Calls, c)
	line := c.String()
	var h Handler
	for i := len(r.rules) - 1; i >= 0; i-- {
		if strings.HasPrefix(line, r.rules[i].prefix) {
			h = r.rules[i].handler
			break
		}
	}
	r.mu.Unlock()
	if h == nil {
		return system.Result{ExitCode: 127}, fmt.Errorf("systemtest: unexpected command %q", line)
	}
	return h(c)
}

// Commands returns the string form of every call so far.
func (r *Runner) Commands() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.Calls))
	for i, c := range r.Calls {
		out[i] = c.String()
	}
	return out
}

// Ran reports whether any call starts with prefix.
func (r *Runner) Ran(prefix string) bool {
	for _, c := range r.Commands() {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}
