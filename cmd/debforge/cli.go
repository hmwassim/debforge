package main

import (
	"fmt"
	"sort"
	"strings"
)

// flagSpec describes one flag.
type flagSpec struct {
	long  string
	short string
	value bool // takes a value
	help  string
}

// command describes one subcommand.
type command struct {
	name  string
	args  string
	help  string
	flags []string // long names of accepted flags
	run   func(a *App, inv *invocation) error
}

// invocation is a parsed command line.
type invocation struct {
	cmd   *command
	args  []string
	flags map[string][]string
}

func (i *invocation) has(f string) bool { _, ok := i.flags[f]; return ok }

func (i *invocation) values(f string) []string { return i.flags[f] }

var globalFlags = []flagSpec{
	{long: "yes", short: "y", help: "answer yes to confirmations"},
	{long: "help", short: "h", help: "show help"},
	{long: "no-color", help: "disable colours"},
	{long: "version", help: "print the debforge version"},
}

var allFlags = map[string]flagSpec{
	"force":     {long: "force", short: "f", help: "reinstall even if up to date; replace modified files (with backup)"},
	"dry-run":   {long: "dry-run", short: "n", help: "show the plan and stop"},
	"variant":   {long: "variant", value: true, help: "choose a package variant, e.g. --variant nvidia=open"},
	"all":       {long: "all", short: "a", help: "update apt packages and every debforge package"},
	"self":      {long: "self", help: "act on debforge itself"},
	"verbose":   {long: "verbose", short: "v", help: "show file contents and hooks"},
	"installed": {long: "installed", short: "i", help: "only installed packages"},
	"names":     {long: "names", help: "print package names only (for scripts)"},
}

func init() {
	for _, g := range globalFlags {
		allFlags[g.long] = g
	}
}

// parse parses argv (without the program name). Flags may appear anywhere;
// "--" ends flag parsing.
func parse(cmds []*command, argv []string) (*invocation, error) {
	inv := &invocation{flags: map[string][]string{}}
	var rest []string
	byShort := map[string]flagSpec{}
	for _, f := range allFlags {
		if f.short != "" {
			byShort[f.short] = f
		}
	}
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		switch {
		case a == "--":
			rest = append(rest, argv[i+1:]...)
			i = len(argv)
		case strings.HasPrefix(a, "--"):
			name, val, hasVal := strings.Cut(a[2:], "=")
			f, ok := allFlags[name]
			if !ok {
				return nil, fmt.Errorf("unknown flag --%s", name)
			}
			if f.value && !hasVal {
				if i+1 >= len(argv) {
					return nil, fmt.Errorf("--%s needs a value", name)
				}
				i++
				val = argv[i]
			} else if !f.value && hasVal {
				return nil, fmt.Errorf("--%s takes no value", name)
			}
			inv.flags[name] = append(inv.flags[name], val)
		case strings.HasPrefix(a, "-") && len(a) > 1:
			for _, c := range a[1:] {
				f, ok := byShort[string(c)]
				if !ok {
					return nil, fmt.Errorf("unknown flag -%c", c)
				}
				if f.value {
					return nil, fmt.Errorf("-%c needs a value; use --%s=VALUE", c, f.long)
				}
				inv.flags[f.long] = append(inv.flags[f.long], "")
			}
		default:
			rest = append(rest, a)
		}
	}
	if len(rest) == 0 {
		return inv, nil
	}
	for _, c := range cmds {
		if c.name == rest[0] {
			inv.cmd = c
		}
	}
	if inv.cmd == nil {
		return nil, fmt.Errorf("unknown command %q (see 'debforge --help')", rest[0])
	}
	inv.args = rest[1:]
	allowed := map[string]bool{}
	for _, g := range globalFlags {
		allowed[g.long] = true
	}
	for _, f := range inv.cmd.flags {
		allowed[f] = true
	}
	for f := range inv.flags {
		if !allowed[f] {
			return nil, fmt.Errorf("%s does not accept --%s", inv.cmd.name, f)
		}
	}
	return inv, nil
}

func usage(cmds []*command) string {
	var b strings.Builder
	b.WriteString("debforge - opinionated Debian Trixie provisioner and package manager\n\n")
	b.WriteString("Usage: debforge <command> [flags] [args]\n\nCommands:\n")
	for _, c := range cmds {
		fmt.Fprintf(&b, "  %-28s %s\n", strings.TrimSpace(c.name+" "+c.args), c.help)
	}
	b.WriteString("\nFlags:\n")
	var names []string
	for n := range allFlags {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		f := allFlags[n]
		s := "    "
		if f.short != "" {
			s = "-" + f.short + ", "
		}
		l := "--" + f.long
		if f.value {
			l += "=VALUE"
		}
		fmt.Fprintf(&b, "  %s%-22s %s\n", s, l, f.help)
	}
	b.WriteString("\nPackages can be given by name, glob (\"fonts-nerd-*\") or category (\"@gaming\").\n")
	return b.String()
}
