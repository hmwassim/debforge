// Command debforge is an opinionated Debian Trixie provisioner and package
// manager.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/hmwassim/debforge/internal/apt"
	"github.com/hmwassim/debforge/internal/catalog"
	"github.com/hmwassim/debforge/internal/files"
	"github.com/hmwassim/debforge/internal/system"
	"github.com/hmwassim/debforge/internal/ui"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

// usageError marks a command-line mistake (exit code 2).
type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

func usageErr(msg string) error { return usageError{msg} }

func commands() []*command {
	return []*command{
		{name: "install", args: "<pkg>...", help: "install packages", flags: []string{"force", "dry-run", "variant"}, run: cmdInstall},
		{name: "remove", args: "<pkg>...", help: "remove packages (and what depends on them)", flags: []string{"dry-run", "self", "all"}, run: cmdRemove},
		{name: "update", args: "[<pkg>...]", help: "update packages; --all also upgrades the system; --self updates debforge", flags: []string{"all", "force", "dry-run", "variant", "self"}, run: cmdUpdate},
		{name: "setup", help: "provision the system (repositories, drivers, desktop)", flags: []string{"force", "dry-run"}, run: cmdSetup},
		{name: "doctor", help: "check the system against the setup profile", run: cmdDoctor},
		{name: "list", args: "[@category]", help: "list packages", flags: []string{"installed", "names"}, run: cmdList},
		{name: "search", args: "<term>...", help: "search package names and descriptions", run: cmdSearch},
		{name: "info", args: "<pkg>...", help: "show package details", flags: []string{"verbose"}, run: cmdInfo},
		{name: "diff", args: "[<path>...]", help: "show pending .debforge-new config updates", run: cmdDiff},
		{name: "sync", help: "find packages removed behind debforge's back", flags: []string{"dry-run"}, run: cmdSync},
		{name: "completion", args: "bash|zsh|fish", help: "print a shell completion script", run: cmdCompletion},
		{name: "version", help: "print the debforge version", run: cmdVersion},
	}
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(argv []string) int {
	cmds := commands()
	inv, err := parse(cmds, argv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 2
	}
	if inv.cmd == nil && inv.has("version") {
		fmt.Println("debforge " + version)
		return 0
	}
	if inv.cmd == nil || inv.has("help") {
		fmt.Print(usage(cmds))
		if inv.cmd == nil && !inv.has("help") {
			return 2
		}
		return 0
	}

	opts := ui.Detect(inv.has("yes"))
	if inv.has("no-color") {
		opts.Color = false
	}
	u := ui.New(opts)
	logs := ui.OpenLog(DefaultPaths.Logs)
	if os.Geteuid() != 0 {
		logs = ui.OpenLog("") // non-root runs don't log to /var/log
	}
	defer logs.Close()
	logs.Printf("debforge %s: %v", version, argv)

	// A closed output pipe (debforge ... | head) must never kill debforge
	// halfway through a transaction; writes just fail with EPIPE instead.
	signal.Ignore(syscall.SIGPIPE)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigs := make(chan os.Signal, 2)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigs)
	go func() {
		for range sigs {
			if ctx.Err() == nil {
				u.Warn("interrupted: finishing the current step, then stopping (apt is never cut off mid-transaction)")
				cancel()
			} else {
				u.Warn("still finishing the current step; please wait")
			}
		}
	}()

	a := &App{
		Ctx: ctx, UI: u, Log: logs, R: system.ExecRunner{Log: logs.Command}, Paths: DefaultPaths,
		Getenv: os.Getenv, Euid: os.Geteuid(), Files: &files.Engine{},
		Hardware: func(h *catalog.Hardware) bool { return system.HasPCIVendor("", h.PCIVendor) },
		Version:  version,
	}
	err = inv.cmd.run(a, inv)
	switch {
	case err == nil:
		return 0
	case errors.As(err, new(usageError)):
		u.Error("%v", err)
		return 2
	case errors.Is(err, context.Canceled) || errors.Is(err, apt.ErrInterrupted):
		u.Error("%v", err)
		return 130
	default:
		u.Error("%v", err)
		logs.Printf("error: %v", err)
		return 1
	}
}
