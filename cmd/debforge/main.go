package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/hmwassim/debforge/internal/aptpty"
	"github.com/hmwassim/debforge/internal/adapters/exec"
	"github.com/hmwassim/debforge/internal/adapters/fs"
	"github.com/hmwassim/debforge/internal/adapters/lock"
	"github.com/hmwassim/debforge/internal/adapters/store"
	"github.com/hmwassim/debforge/internal/adapters/system"
	"github.com/hmwassim/debforge/internal/adapters/ui"
	"github.com/hmwassim/debforge/internal/buildmeta"
	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/definition"
	aptInst "github.com/hmwassim/debforge/internal/domain/installer/apt"
	configInst "github.com/hmwassim/debforge/internal/domain/installer/config"
	debInst "github.com/hmwassim/debforge/internal/domain/installer/deb"
	sourceInst "github.com/hmwassim/debforge/internal/domain/installer/source"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/self"
	"github.com/hmwassim/debforge/internal/service"
)

var version = buildmeta.DefaultVersion

func main() {
	os.Exit(run())
}

func run() int {
	ctx := context.Background()

	rawArgs := os.Args[1:]
	if len(rawArgs) == 0 {
		usage()
		return 0
	}

	cfgu := self.DefaultConfig()
	if v := os.Getenv("DEBFORGE_PKGS_DIR"); v != "" {
		cfgu.PkgsDir = v
	}
	fsys := fs.NewFileSystem()
	runner := exec.NewRunner()
	locker := lock.NewFLock()
	sys := system.NewSystem()
	u := ui.NewConsoleUI()

	switch rawArgs[0] {
	case "--version":
		fmt.Println("debforge " + version)
		return 0

	case "--self-update":
		updater := self.NewUpdater(cfgu, runner, fsys, u, locker, sys)
		if err := updater.Update(ctx); err != nil {
			u.Error("%s", err)
			return 1
		}
		return 0

	case "--help":
		usage()
		return 0

	case "--self-remove":
		reg, instReg, stateSvc, err := bootstrap(cfgu, fsys, runner, u)
		if err != nil {
			u.Error("bootstrap: %s", err)
			return 1
		}
		remover := self.NewRemover(cfgu, runner, fsys, u, locker, sys, reg, instReg, stateSvc)
		if err := remover.Remove(ctx); err != nil {
			u.Error("%s", err)
			return 1
		}
		return 0
	}

	fs := flag.NewFlagSet("debforge", flag.ContinueOnError)
	y := fs.Bool("y", false, "Skip confirmation prompts")
	yes := fs.Bool("yes", false, "Skip confirmation prompts")
	f := fs.Bool("f", false, "Force operation (reinstall)")
	force := fs.Bool("force", false, "Force operation (reinstall)")
	a := fs.Bool("a", false, "Update all packages (update only)")
	all := fs.Bool("all", false, "Update all packages (update only)")
	fs.Usage = usage

	if err := fs.Parse(rawArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		u.Error("%s", err)
		return 1
	}

	yesMode := *y || *yes
	forceMode := *f || *force
	allMode := *a || *all
	args := fs.Args()

	if yesMode {
		u.SetYes(true)
	}

	if len(args) == 0 {
		usage()
		return 0
	}

	reg, instReg, stateSvc, err := bootstrap(cfgu, fsys, runner, u)
	if err != nil {
		u.Error("bootstrap: %s", err)
		return 1
	}

	switch args[0] {
	case "install":
		names := extractFlags(args[1:], &yesMode, &forceMode, &allMode)
		if len(names) == 0 {
			usage()
			return 1
		}
		if !loadDefs(reg, names, fsys, u) {
			return 1
		}
		return runInstall(ctx, u, reg, instReg, stateSvc, locker, cfgu, runner, fsys, names, forceMode)

	case "remove":
		names := extractFlags(args[1:], &yesMode, &forceMode, &allMode)
		if len(names) == 0 {
			usage()
			return 1
		}
		if !loadDefs(reg, names, fsys, u) {
			return 1
		}
		return runRemove(ctx, u, reg, instReg, stateSvc, locker, cfgu, runner, fsys, names)

	case "update":
		names := extractFlags(args[1:], &yesMode, &forceMode, &allMode)
		if len(names) == 0 && !allMode {
			usage()
			return 1
		}
		if allMode && len(names) > 0 {
			u.Warn("--all updates every managed package; ignoring explicit name(s): %s", strings.Join(names, ", "))
			names = nil
		}
		if !loadDefs(reg, names, fsys, u) {
			return 1
		}
		return runUpdate(ctx, u, reg, instReg, stateSvc, locker, cfgu, runner, fsys, names, forceMode, allMode)

	default:
		usage()
	}
	return 0
}

func runInstall(ctx context.Context, u ports.UI, reg *pkg.Registry, instReg *installer.Registry, stateSvc *service.StateManager, locker ports.Locker, cfg *self.Config, runner ports.CommandRunner, fsys ports.FileSystem, names []string, forceMode bool) int {
	svc := service.NewInstallService(reg, instReg, service.NewResolver(reg), stateSvc, locker, cfg.LockPath, runner, fsys)

	var conflicts []string
	for _, name := range names {
		p, ok := reg.Lookup(name)
		if !ok {
			continue
		}
		if p.Apt != nil {
			conflicts = append(conflicts, aptpty.FindInstalledConflicts(ctx, runner, p.Apt.Conflicts)...)
		}
	}
	if len(conflicts) > 0 {
		u.Info("Conflicting package(s) installed: %s", strings.Join(conflicts, ", "))
	}

	return withConfirm(ctx, u, func(spinner ports.Spinner) error {
		return svc.Run(ctx, names, forceMode, spinner)
	})
}

func runRemove(ctx context.Context, u ports.UI, reg *pkg.Registry, instReg *installer.Registry, stateSvc *service.StateManager, locker ports.Locker, cfg *self.Config, runner ports.CommandRunner, fsys ports.FileSystem, names []string) int {
	svc := service.NewRemoveService(reg, instReg, stateSvc, locker, cfg.LockPath, runner, fsys)
	return withConfirm(ctx, u, func(spinner ports.Spinner) error {
		return svc.Run(ctx, names, spinner)
	})
}

func runUpdate(ctx context.Context, u ports.UI, reg *pkg.Registry, instReg *installer.Registry, stateSvc *service.StateManager, locker ports.Locker, cfg *self.Config, runner ports.CommandRunner, fsys ports.FileSystem, names []string, forceMode, allMode bool) int {
	svc := service.NewInstallService(reg, instReg, service.NewResolver(reg), stateSvc, locker, cfg.LockPath, runner, fsys)
	return withConfirm(ctx, u, func(spinner ports.Spinner) error {
		if err := aptpty.RunUpdate(ctx, runner, spinner); err != nil {
			return err
		}
		if allMode {
			if err := aptpty.RunUpgrade(ctx, runner, spinner); err != nil {
				return err
			}
		}
		return svc.Update(ctx, names, forceMode, allMode, spinner)
	})
}

// extractFlags scans ss for known boolean flags and updates yes, force,
// and all accordingly. Interspersed flags are handled, preserving the
// original behavior where flags like --all could appear after the command
// (e.g. "debforge update --all").
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

func bootstrap(cfg *self.Config, fsys ports.FileSystem, runner ports.CommandRunner, ui ports.UI) (*pkg.Registry, *installer.Registry, *service.StateManager, error) {
	reg := pkg.NewRegistry()
	instReg := installer.NewRegistry()

	instReg.Register(pkg.TypeApt, aptInst.NewInstaller(runner, fsys, ui))
	instReg.Register(pkg.TypeDeb, debInst.NewInstaller(runner, fsys, ui))
	instReg.Register(pkg.TypeSource, sourceInst.NewInstaller(runner, fsys, ui))
	instReg.Register(pkg.TypeConfig, configInst.NewInstaller(runner, fsys, ui))

	if err := definition.LoadAll(cfg.PkgsDir, fsys, reg); err != nil {
		return nil, nil, nil, fmt.Errorf("load definitions: %w", err)
	}

	st := store.NewStore[service.State](fsys, cfg.StatePath)
	stateSvc := service.NewStateManager(st)
	if _, err := stateSvc.Load(); err != nil {
		return nil, nil, nil, fmt.Errorf("load state: %w", err)
	}

	return reg, instReg, stateSvc, nil
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

func usage() {
	fmt.Println("debforge - package manager")
	fmt.Println()
	fmt.Println("Usage: debforge [flags] <command> [<name>...]")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("    -y, --yes           Skip confirmation prompts")
	fmt.Println("    -f, --force         Force operation (reinstall)")
	fmt.Println("    -a, --all           Update all packages (update only)")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("    install <name>...    Install packages")
	fmt.Println("    remove <name>...    Remove packages from system")
	fmt.Println("    update [<name>...]   Reinstall packages (runs apt-get update)")
	fmt.Println("        --all           Update all packages and run apt-get upgrade")
	fmt.Println("    --self-update       Update debforge itself")
	fmt.Println("    --self-remove       Remove debforge from system")
	fmt.Println("    --help              Show this help")
	fmt.Println("    --version           Show version")
}
