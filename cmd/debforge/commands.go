package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"sort"
	"strings"

	"golang.org/x/term"

	"github.com/hmwassim/debforge/internal/adapters/store"
	"github.com/hmwassim/debforge/internal/aptpty"
	"github.com/hmwassim/debforge/internal/definition"
	"github.com/hmwassim/debforge/internal/domain/installer"
	aptInst "github.com/hmwassim/debforge/internal/domain/installer/apt"
	configInst "github.com/hmwassim/debforge/internal/domain/installer/config"
	debInst "github.com/hmwassim/debforge/internal/domain/installer/deb"
	sourceInst "github.com/hmwassim/debforge/internal/domain/installer/source"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/self"
	"github.com/hmwassim/debforge/internal/service"
	"github.com/hmwassim/debforge/internal/setup"
)

// Package-level test hooks (overridable in tests).
var isTerminal = term.IsTerminal
var lookPath = exec.LookPath

// commandHandler bundles the dependencies shared by install/remove/update
// so they are wired once instead of repeated in every handler.
type commandHandler struct {
	reg      *pkg.Registry
	instReg  *installer.Registry
	stateSvc *service.StateManager
	locker   ports.Locker
	cfg      *self.Config
	runner   ports.CommandRunner
	fsys     ports.FileSystem
	sys      ports.System
}

func newHandler(cfg *self.Config, fsys ports.FileSystem, runner ports.CommandRunner, locker ports.Locker, ui ports.UI, sys ports.System) (*commandHandler, error) {
	reg := pkg.NewRegistry()
	instReg := installer.NewRegistry()

	instReg.Register(pkg.TypeApt, aptInst.NewInstaller(runner, fsys, ui))
	instReg.Register(pkg.TypeDeb, debInst.NewInstaller(runner, fsys, ui, sys))
	instReg.Register(pkg.TypeSource, sourceInst.NewInstaller(runner, fsys, ui))
	instReg.Register(pkg.TypeConfig, configInst.NewInstaller(runner, fsys, ui, sys))

	if err := definition.LoadAll(cfg.PkgsDir, fsys, reg); err != nil {
		return nil, fmt.Errorf("load definitions: %w", err)
	}

	st := store.NewStore[service.State](fsys, cfg.StatePath)
	stateSvc := service.NewStateManager(st)
	if _, err := stateSvc.Load(); err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}

	return &commandHandler{
		reg: reg, instReg: instReg, stateSvc: stateSvc,
		locker: locker, cfg: cfg, runner: runner, fsys: fsys, sys: sys,
	}, nil
}

func (h *commandHandler) install(ctx context.Context, u ports.UI, names []string, forceMode bool) int {
	names = expandGlobs(h.reg, names)
	svc := service.NewInstallService(h.reg, h.instReg, service.NewResolver(h.reg), h.stateSvc, h.locker, h.cfg.LockPath, h.runner, h.fsys, h.sys)

	resolver := service.NewResolver(h.reg)
	for _, name := range names {
		p, ok := h.reg.Lookup(name)
		if !ok {
			continue
		}
		deps, err := resolver.Resolve(p)
		if err != nil {
			u.Error("resolve deps: %s", err)
			return 1
		}
		for _, dep := range deps {
			if strings.ToLower(dep.Name) == "nvidia" {
				spinner := u.Spinner(ctx, "checking gpu...")
				if err := aptInst.CheckGPU(ctx, h.runner, dep.Name); err != nil {
					spinner.DoneWarn()
					u.Warn("%s", err)
					return 1
				}
				spinner.Done()
			}
		}
	}

	var conflicts []string
	for _, name := range names {
		p, ok := h.reg.Lookup(name)
		if !ok {
			continue
		}
		if p.Apt != nil {
			conflicts = append(conflicts, aptpty.FindInstalledConflicts(ctx, h.runner, p.Apt.Conflicts)...)
		}
	}
	if len(conflicts) > 0 {
		u.Info("Conflicting package(s) installed: %s", strings.Join(conflicts, ", "))
	}

	if err := svc.SelectVariants(ctx, names, forceMode); err != nil {
		u.Error("%s", err)
		return 1
	}

	return withConfirm(ctx, u, func(spinner ports.Spinner) error {
		return svc.Run(ctx, names, forceMode, spinner)
	})
}

func (h *commandHandler) remove(ctx context.Context, u ports.UI, names []string) int {
	names = expandGlobs(h.reg, names)
	svc := service.NewRemoveService(h.reg, h.instReg, h.stateSvc, h.locker, h.cfg.LockPath, h.runner, h.fsys, h.sys)

	st, err := h.stateSvc.Load()
	if err == nil {
		if deps := svc.AffectedDependents(st, names); len(deps) > 0 {
			u.Info("Also removing: %s", strings.Join(deps, ", "))
		}
	}

	return withConfirm(ctx, u, func(spinner ports.Spinner) error {
		return svc.Run(ctx, names, spinner)
	})
}

func (h *commandHandler) update(ctx context.Context, u ports.UI, names []string, forceMode, allMode bool) int {
	names = expandGlobs(h.reg, names)
	svc := service.NewInstallService(h.reg, h.instReg, service.NewResolver(h.reg), h.stateSvc, h.locker, h.cfg.LockPath, h.runner, h.fsys, h.sys)
	return withConfirm(ctx, u, func(spinner ports.Spinner) error {
		if err := aptpty.RunUpdate(ctx, h.runner, spinner); err != nil {
			return err
		}
		if allMode {
			if err := aptpty.RunUpgrade(ctx, h.runner, spinner); err != nil {
				return err
			}
		}
		return svc.Update(ctx, names, forceMode, allMode, spinner)
	})
}

func (h *commandHandler) setup(ctx context.Context, u ports.UI, force bool) int {
	if !h.sys.IsPrivileged() {
		u.Error("setup must be run as root")
		return 1
	}

	u.Info("Provision system with recommended configuration")
	if !u.Prompt("Continue?") {
		u.Info("Cancelled")
		return 0
	}

	st, err := setup.LoadState(h.fsys, h.cfg.SetupStatePath)
	if err != nil {
		u.Error("load setup state: %s", err)
		return 1
	}

	cx := &setup.Context{
		Runner:       h.runner,
		Fsys:         h.fsys,
		Sys:          h.sys,
		UI:           u,
		Force:        force,
		ConfigHashes: st.ConfigHashes,
	}

	runner := setup.NewRunner(setup.DefaultSteps()...)

	if err := runner.Run(ctx, cx); err != nil {
		u.Error("%s", err)
		return 1
	}

	st.ConfigHashes = cx.ConfigHashes
	if err := setup.SaveState(h.fsys, h.cfg.SetupStatePath, st); err != nil {
		u.Error("save setup state: %s", err)
		return 1
	}

	u.Success("System provisioning complete")
	return 0
}

func (h *commandHandler) search(ctx context.Context, u ports.UI, patterns []string) int {
	st, err := h.stateSvc.Load()
	if err != nil {
		u.Error("load state: %s", err)
		return 1
	}

	out := formatSearchOutput(h.reg, st, patterns)
	if out == "" {
		if len(patterns) > 0 {
			u.Info("no packages found matching %q", strings.Join(patterns, " "))
		}
		return 0
	}

	if !isTerminal(int(os.Stdout.Fd())) {
		fmt.Print(out)
		return 0
	}

	pagerCmd, pagerArgs := selectPager()
	if pagerCmd == "" {
		fmt.Print(out)
		return 0
	}

	cmd := exec.Command(pagerCmd, pagerArgs...)
	cmd.Stdin = strings.NewReader(out)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Print(out)
	}
	return 0
}

// selectPager returns the pager command and arguments to use for displaying
// output. It checks the PAGER environment variable first, then falls back to
// less. Returns ("", nil) when no suitable pager is found.
func selectPager() (cmd string, args []string) {
	envPager := os.Getenv("PAGER")
	if envPager != "" {
		parts := strings.Fields(envPager)
		cmd = parts[0]
		if len(parts) > 1 {
			args = parts[1:]
		}
		return
	}
	if p, err := lookPath("less"); err == nil {
		return p, []string{"-FRS"}
	}
	return "", nil
}

// formatSearchOutput formats the package listing. When isTerm is true the
// output includes ANSI colour codes suitable for a terminal.
func formatSearchOutput(reg *pkg.Registry, st *service.State, patterns []string) string {
	green, grey, reset := "\033[32m", "\033[90m", "\033[0m"

	pat := ""
	if len(patterns) > 0 {
		pat = strings.ToLower(strings.Join(patterns, " "))
	}

	var names []string
	reg.Range(func(name string, p *pkg.Package) bool {
		if pat != "" {
			n := strings.ToLower(name)
			d := strings.ToLower(p.Description)
			if !strings.Contains(n, pat) && !strings.Contains(d, pat) {
				return true
			}
		}
		names = append(names, name)
		return true
	})
	sort.Strings(names)

	maxLen := 0
	for _, name := range names {
		if len(name) > maxLen {
			maxLen = len(name)
		}
	}
	pad := maxLen + 2

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	for _, name := range names {
		p, _ := reg.Lookup(name)
		_, installed := st.Packages[name]
		if installed {
			fmt.Fprintf(w, "%s[*]%s %-*s", green, reset, pad, name)
			if p.Description != "" {
				fmt.Fprintf(w, "%s%s%s", grey, p.Description, reset)
			}
			fmt.Fprintln(w)
		} else {
			fmt.Fprintf(w, "%s[-]%s %s%-*s%s", grey, reset, grey, pad, name, reset)
			if p.Description != "" {
				fmt.Fprintf(w, "%s%s%s", grey, p.Description, reset)
			}
			fmt.Fprintln(w)
		}
	}
	w.Flush()
	return buf.String()
}

func extractFlags(ss []string, yes, force, all *bool) []string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		switch s {
		case "-y", "--yes":
			*yes = true
		case "-f", "--force":
			*force = true
		case "-a", "--all":
			*all = true
		default:
			out = append(out, s)
		}
	}
	return out
}

func loadYAMLDefinitions(reg *pkg.Registry, names []string, fsys ports.FileSystem) error {
	for i, n := range names {
		if !strings.HasSuffix(n, ".yaml") {
			continue
		}
		p, err := definition.Parse(n, fsys)
		if err != nil {
			return fmt.Errorf("load %s: %w", n, err)
		}
		reg.Register(p)
		names[i] = p.Name
	}
	return nil
}

func loadDefs(reg *pkg.Registry, names []string, fsys ports.FileSystem, u ports.UI) bool {
	if err := loadYAMLDefinitions(reg, names, fsys); err != nil {
		u.Error("%s", err)
		return false
	}
	return true
}

func withConfirm(ctx context.Context, u ports.UI, fn func(ports.Spinner) error) int {
	if !u.Prompt("Continue?") {
		u.Info("Cancelled")
		return 0
	}
	spinner := u.Spinner(ctx, "Processing")
	if err := fn(spinner); err != nil {
		if !errors.Is(err, service.ErrNotInstalled) {
			u.Error("%s", err)
		}
		return 1
	}
	return 0
}

// expandGlobs expands glob patterns in names against the registry.
// Names without glob characters are kept as-is. Globs with fewer than
// three literal characters before the first wildcard are treated as
// literals (preventing accidental matches from single-char prefixes).
// Duplicates are removed.
func expandGlobs(reg *pkg.Registry, names []string) []string {
	var out []string
	seen := make(map[string]bool)
	for _, name := range names {
		if !containsGlob(name) || globPrefixLen(name) < 3 {
			if !seen[name] {
				out = append(out, name)
				seen[name] = true
			}
			continue
		}
		reg.Range(func(key string, _ *pkg.Package) bool {
			if ok, _ := path.Match(name, key); ok && !seen[key] {
				out = append(out, key)
				seen[key] = true
			}
			return true
		})
	}
	return out
}

func containsGlob(s string) bool {
	return strings.ContainsAny(s, "*?[")
}

func globPrefixLen(s string) int {
	for i, r := range s {
		if r == '*' || r == '?' || r == '[' {
			return i
		}
	}
	return len(s)
}
