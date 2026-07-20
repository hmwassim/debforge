package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/hmwassim/debforge/internal/aptpty"
	"github.com/hmwassim/debforge/internal/format"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/self"
	"github.com/hmwassim/debforge/internal/setup"
)

func (h *commandHandler) selfRemove(ctx context.Context, u ports.UI) int {
	remover := self.NewRemover(h.cfg, u, h.factory, h.pkgList)
	if err := remover.Remove(ctx); err != nil {
		u.Error("self-remove failed: %s", err)
		return 1
	}
	return 0
}

func (h *commandHandler) selfUpdate(ctx context.Context, u ports.UI, forceMode bool) int {
	updater := self.NewUpdater(h.cfg, h.runner, h.fsys, u, h.locker, h.sys, forceMode)
	if err := updater.Update(ctx); err != nil {
		u.Error("self-update failed: %s", err)
		return 1
	}
	return 0
}

func (h *commandHandler) install(ctx context.Context, u ports.UI, names []string, forceMode bool) int {
	if !h.checkGPUPreconditions(ctx, u, names) {
		return 1
	}
	if conflicts, err := h.checkConflicts(ctx, u, names); err != nil {
		u.Error("conflict check: %s", err)
		return 1
	} else if len(conflicts) > 0 {
		u.Info("Conflicting package(s) installed: %s", strings.Join(conflicts, ", "))
	}

	svc := h.factory.Install()
	if err := svc.SelectVariants(ctx, names, forceMode); err != nil {
		u.Error("variant selection failed: %s", err)
		return 1
	}

	return withConfirm(ctx, u, func(spinner ports.Spinner) error {
		return svc.Run(ctx, names, forceMode, spinner)
	})
}

func (h *commandHandler) remove(ctx context.Context, u ports.UI, names []string) int {
	svc := h.factory.Remove(h.pkgList)

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
	svc := h.factory.Install()
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

func (h *commandHandler) doctor(ctx context.Context, u ports.UI) int {
	if !h.sys.IsPrivileged() {
		u.Error("doctor must be run as root")
		return 1
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
		ConfigHashes: st.ConfigHashes,
	}

	steps := setup.DefaultSteps()
	results := setup.NewRunner(steps...).CheckAll(ctx, cx)

	allOk := true
	for i, r := range results {
		if _, ok := steps[i].(*setup.UpgradeStep); ok {
			continue
		}
		name := steps[i].Name()
		switch r.Status {
		case setup.StatusSatisfied:
			u.Success("%s", name)
		case setup.StatusMissing:
			u.Info("%s (not configured)", name)
			allOk = false
		case setup.StatusDrifted:
			u.Warn("%s (modified by user): %s", name, r.Summary)
			allOk = false
		case setup.StatusConflict:
			u.Warn("%s (conflict): %s", name, r.Summary)
			allOk = false
		case setup.StatusError:
			u.Error("%s: %s", name, r.Summary)
			allOk = false
		}
	}

	if allOk {
		u.Success("All systems ready")
		return 0
	}
	return 1
}

func (h *commandHandler) diff(ctx context.Context, u ports.UI, args []string) int {
	targets := make([]string, 0, len(args))

	if len(args) > 0 {
		for _, arg := range args {
			sidecar := arg + ".debforge-new"
			exists, err := h.fsys.Exists(sidecar)
			if err != nil {
				u.Error("check %s: %s", sidecar, err)
				return 1
			}
			if !exists {
				u.Warn("no sidecar found for %s", arg)
				continue
			}
			targets = append(targets, arg)
		}
	} else {
		if err := h.fsys.Walk("/", func(path string, info ports.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if strings.HasSuffix(path, ".debforge-new") {
				targets = append(targets, strings.TrimSuffix(path, ".debforge-new"))
			}
			return nil
		}); err != nil {
			u.Error("scan: %s", err)
			return 1
		}
		if len(targets) == 0 {
			u.Info("no config sidecars found")
			return 0
		}
	}

	if len(targets) == 0 {
		return 0
	}

	for i, orig := range targets {
		if i > 0 {
			fmt.Println()
		}
		sidecar := orig + ".debforge-new"
		stdout, stderr, err := h.runner.Run(ctx, "diff", "-u", orig, sidecar)
		if len(stderr) > 0 {
			u.Error("diff: %s", string(stderr))
			return 1
		}
		if err != nil && len(stdout) == 0 {
			u.Error("diff: %s", err)
			return 1
		}
		fmt.Print(string(stdout))
	}
	return 0
}

func (h *commandHandler) search(ctx context.Context, u ports.UI, patterns []string) int {
	st, err := h.stateSvc.Load()
	if err != nil {
		u.Error("load state: %s", err)
		return 1
	}

	out := format.FormatSearchOutput(h.reg, format.NewStateView(st), patterns)
	if out == "" {
		if len(patterns) > 0 {
			u.Info("no packages found matching %q", strings.Join(patterns, " "))
		}
		return 0
	}

	return displayWithPager(out)
}

func (h *commandHandler) list(ctx context.Context, u ports.UI, category string, showPackages bool) int {
	st, err := h.stateSvc.Load()
	if err != nil {
		u.Error("load state: %s", err)
		return 1
	}

	var out string
	switch {
	case category != "":
		out = format.FormatListCategory(h.reg, format.NewStateView(st), category)
	case showPackages:
		out = format.FormatListPackages(h.reg, format.NewStateView(st))
	default:
		out = format.FormatListCategories(h.reg, format.NewStateView(st))
	}

	if out == "" {
		return 0
	}

	return displayWithPager(out)
}

func (h *commandHandler) info(ctx context.Context, u ports.UI, names []string, verbose bool) int {
	st, err := h.stateSvc.Load()
	if err != nil {
		u.Error("load state: %s", err)
		return 1
	}
	for _, name := range names {
		if _, ok := h.reg.Lookup(name); !ok {
			u.Error("unknown package: %s", name)
			return 1
		}
	}
	var sb strings.Builder
	for i, name := range names {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(format.FormatInfoOutput(h.reg, format.NewStateView(st), name, verbose))
	}
	out := sb.String()
	if out == "" {
		return 0
	}
	return displayWithPager(out)
}
