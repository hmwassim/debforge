package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
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
)

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
}

func newHandler(cfg *self.Config, fsys ports.FileSystem, runner ports.CommandRunner, locker ports.Locker, ui ports.UI) (*commandHandler, error) {
	reg := pkg.NewRegistry()
	instReg := installer.NewRegistry()

	instReg.Register(pkg.TypeApt, aptInst.NewInstaller(runner, fsys, ui))
	instReg.Register(pkg.TypeDeb, debInst.NewInstaller(runner, fsys, ui))
	instReg.Register(pkg.TypeSource, sourceInst.NewInstaller(runner, fsys, ui))
	instReg.Register(pkg.TypeConfig, configInst.NewInstaller(runner, fsys, ui))

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
		locker: locker, cfg: cfg, runner: runner, fsys: fsys,
	}, nil
}

func (h *commandHandler) install(ctx context.Context, u ports.UI, names []string, forceMode bool) int {
	svc := service.NewInstallService(h.reg, h.instReg, service.NewResolver(h.reg), h.stateSvc, h.locker, h.cfg.LockPath, h.runner, h.fsys)

	for _, name := range names {
		p, ok := h.reg.Lookup(name)
		if !ok || p.Apt == nil {
			continue
		}
		if strings.ToLower(p.Name) != "nvidia" {
			continue
		}
		spinner := u.Spinner(ctx, "checking gpu...")
		if err := aptInst.CheckGPU(ctx, h.runner, p.Name); err != nil {
			spinner.DoneWarn()
			u.Warn("%s", err)
			return 1
		}
		spinner.Done()
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
	svc := service.NewRemoveService(h.reg, h.instReg, h.stateSvc, h.locker, h.cfg.LockPath, h.runner, h.fsys)
	return withConfirm(ctx, u, func(spinner ports.Spinner) error {
		return svc.Run(ctx, names, spinner)
	})
}

func (h *commandHandler) update(ctx context.Context, u ports.UI, names []string, forceMode, allMode bool) int {
	svc := service.NewInstallService(h.reg, h.instReg, service.NewResolver(h.reg), h.stateSvc, h.locker, h.cfg.LockPath, h.runner, h.fsys)
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

func (h *commandHandler) search(ctx context.Context, u ports.UI, patterns []string) int {
	st, err := h.stateSvc.Load()
	if err != nil {
		u.Error("load state: %s", err)
		return 1
	}

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	green, grey, reset := "\033[32m", "\033[90m", "\033[0m"
	isTerm := term.IsTerminal(int(os.Stdout.Fd()))

	pat := ""
	if len(patterns) > 0 {
		pat = strings.ToLower(strings.Join(patterns, " "))
	}

	var names []string
	h.reg.Range(func(name string, p *pkg.Package) bool {
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

	for _, name := range names {
		p, _ := h.reg.Lookup(name)
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

	out := buf.String()
	if out == "" {
		if len(patterns) > 0 {
			u.Info("no packages found matching %q", strings.Join(patterns, " "))
		}
		return 0
	}

	if !isTerm {
		fmt.Print(out)
		return 0
	}

	pagerCmd := os.Getenv("PAGER")
	if pagerCmd == "" {
		if p, err := exec.LookPath("less"); err == nil {
			pagerCmd = p + " -FRS"
		}
	}
	if pagerCmd == "" {
		fmt.Print(out)
		return 0
	}

	cmd := exec.Command("sh", "-c", pagerCmd)
	cmd.Stdin = strings.NewReader(out)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Print(out)
	}
	return 0
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
	spinner := u.Spinner(ctx, "Working")
	if err := fn(spinner); err != nil {
		if !errors.Is(err, service.ErrNotInstalled) {
			u.Error("%s", err)
		}
		return 1
	}
	return 0
}
