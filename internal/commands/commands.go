package commands

import (
	"context"
	"fmt"
	"sort"

	"github.com/hmwassim/debforge/internal/ports"
)

func withSpinner(ctx context.Context, ui ports.UI, desc string, fn func() error) error {
	s := ui.Spinner(ctx, desc)
	if err := fn(); err != nil {
		s.Fail()
		return err
	}
	s.Done()
	return nil
}

var Version = "dev"

type Command interface {
	Name() string
	Usage() string
	Run(ctx context.Context, args []string) error
}

type Registry struct {
	entries map[string]Command
}

func NewRegistry() *Registry {
	return &Registry{entries: make(map[string]Command)}
}

func (r *Registry) Register(cmd Command) {
	r.entries[cmd.Name()] = cmd
}

func (r *Registry) Lookup(name string) (Command, bool) {
	cmd, ok := r.entries[name]
	return cmd, ok
}

func (r *Registry) List() []Command {
	cmds := make([]Command, 0, len(r.entries))
	for _, cmd := range r.entries {
		cmds = append(cmds, cmd)
	}
	sort.Slice(cmds, func(i, j int) bool { return cmds[i].Name() < cmds[j].Name() })
	return cmds
}

func (r *Registry) Help() string {
	out := "debforge -- Manage packages outside Debian repositories\n\nUsage:\n  debforge <command> [flags]\n\nCommands:\n"
	for _, cmd := range r.List() {
		out += fmt.Sprintf("  %-26s %s\n", cmd.Name(), cmd.Usage())
	}
	out += "\nFlags:\n"
	out += "  --version, -V            Show version\n"
	out += "  --help, -h               Show this help message\n"
	return out
}

func PromptVariant(ui ports.UI, variants map[string]string) string {
	keys := make([]string, 0, len(variants))
	for k := range variants {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	ui.Info("Select variant:")
	for i, k := range keys {
		ui.Info("  %d) %s — %s", i+1, k, variants[k])
	}

	input := ui.PromptInput("Enter number [1-%d] or 0 to cancel:", len(keys))
	var n int
	if _, err := fmt.Sscanf(input, "%d", &n); err != nil || n < 0 || n > len(keys) {
		ui.Warn("Invalid selection")
		return ""
	}
	if n == 0 {
		ui.Info("Cancelled")
		return ""
	}
	return keys[n-1]
}
