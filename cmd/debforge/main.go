package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/hmwassim/debforge/internal/adapters/exec"
	"github.com/hmwassim/debforge/internal/adapters/fs"
	"github.com/hmwassim/debforge/internal/adapters/lock"
	"github.com/hmwassim/debforge/internal/adapters/system"
	"github.com/hmwassim/debforge/internal/adapters/ui"
	"github.com/hmwassim/debforge/internal/buildmeta"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/self"
)

var version = buildmeta.DefaultVersion

func main() {
	os.Exit(run())
}

func run() int {
	ctx := context.Background()
	cfgu := self.DefaultConfig()
	if v := os.Getenv("DEBFORGE_PKGS_DIR"); v != "" {
		cfgu.PkgsDir = v
	}
	return runWith(ctx, os.Args[1:], version, cfgu,
		fs.NewFileSystem(), exec.NewRunner(),
		lock.NewFLock(), system.NewSystem(), ui.NewConsoleUI())
}

func runWith(ctx context.Context, rawArgs []string, version string, cfg *self.Config, fsys ports.FileSystem, runner ports.CommandRunner, locker ports.Locker, sys ports.System, ui ports.UI) int {
	if len(rawArgs) == 0 {
		usage()
		return 0
	}

	switch rawArgs[0] {
	case "--version":
		fmt.Println("debforge " + version)
		return 0

	case "--help":
		usage()
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
		ui.Error("%s", err)
		return 1
	}

	yesMode := *y || *yes
	forceMode := *f || *force
	allMode := *a || *all
	selfMode := false
	verboseMode := false
	args := fs.Args()

	if len(args) == 0 {
		usage()
		return 0
	}

	h, err := newHandler(cfg, fsys, runner, locker, ui, sys)
	if err != nil {
		ui.Error("bootstrap: %s", err)
		return 1
	}

	names := extractFlags(args[1:], &yesMode, &forceMode, &allMode, &selfMode, &verboseMode)
	if yesMode {
		ui.SetYes(true)
	}

	switch args[0] {
	case "install":
		if selfMode {
			ui.Error("--self is not supported for install")
			return 1
		}
		if len(names) == 0 {
			usage()
			return 1
		}
		if !loadDefs(h.reg, names, fsys, ui) {
			return 1
		}
		names, ok := h.resolveNames(names, ui)
		if !ok {
			return 1
		}
		return h.install(ctx, ui, names, forceMode)

	case "remove":
		if selfMode {
			return h.selfRemove(ctx, ui)
		}
		if len(names) == 0 {
			usage()
			return 1
		}
		if !loadDefs(h.reg, names, fsys, ui) {
			return 1
		}
		names, ok := h.resolveNames(names, ui)
		if !ok {
			return 1
		}
		return h.remove(ctx, ui, names)

	case "update":
		if selfMode {
			return h.selfUpdate(ctx, ui, forceMode)
		}
		if len(names) == 0 && !allMode {
			usage()
			return 1
		}
		if allMode && len(names) > 0 {
			ui.Warn("--all updates every managed package; ignoring explicit name(s): %s", strings.Join(names, ", "))
			names = nil
		}
		if !loadDefs(h.reg, names, fsys, ui) {
			return 1
		}
		names, ok := h.resolveNames(names, ui)
		if !ok {
			return 1
		}
		return h.update(ctx, ui, names, forceMode, allMode)

	case "setup":
		if selfMode {
			ui.Error("--self is not supported for setup")
			return 1
		}
		return h.setup(ctx, ui, forceMode)

	case "doctor":
		return h.doctor(ctx, ui)

	case "list":
		showPackages := slices.Contains(names, "--packages")
		category := ""
		for _, a := range names {
			if strings.HasPrefix(a, "@") {
				category = a[1:]
				break
			}
		}
		return h.list(ctx, ui, category, showPackages)

	case "search":
		return h.search(ctx, ui, names)

	case "diff":
		return h.diff(ctx, ui, names)

	case "info":
		if len(names) == 0 {
			usage()
			return 1
		}
		names, ok := h.resolveNames(names, ui)
		if !ok {
			return 1
		}
		return h.info(ctx, ui, names, verboseMode)

	default:
		usage()
	}
	return 0
}

func usage() {
	fmt.Println("debforge - package manager")
	fmt.Println()
	fmt.Println("Usage: debforge [flags] <command> [<name>...]")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("    -y, --yes               Skip confirmation prompts")
	fmt.Println("    -f, --force             Force operation (reinstall)")
	fmt.Println("    -a, --all               Update all packages (update only)")
	fmt.Println("    -v, --verbose           Show detailed output where supported")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("    install <name>...       Install packages")
	fmt.Println("    remove <name>...        Remove packages from system")
	fmt.Println("    update [<name>...]      Reinstall packages (runs apt-get update)")
	fmt.Println("        --all               Update all packages and run apt-get upgrade")
	fmt.Println("    setup                   Provision system (repos, firmware, desktop)")
	fmt.Println("        --force             Skip checks, reapply all steps")
	fmt.Println("    doctor                  Check system health")
	fmt.Println("    list                    List available categories")
	fmt.Println("    list @<category>        List packages in a category")
	fmt.Println("    list --packages         List packages grouped by category")
	fmt.Println("    search [<pattern>]      Search packages by name or description")
	fmt.Println("    diff [<path>...]         Show config diff vs sidecar")
	fmt.Println("    info <name>...          Show detailed package information")
	fmt.Println("        -v, --verbose       Show full config and script contents")
	fmt.Println("    update --self           Update debforge itself")
	fmt.Println("    remove --self           Remove debforge from system")
	fmt.Println("    --help                  Show this help")
	fmt.Println("    --version               Show version")
}
