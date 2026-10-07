// Package apt drives apt-get non-interactively. Progress comes from
// APT::Status-Fd (machine-readable), never from parsing the human output,
// and dpkg is never asked a question: conffiles keep the local version,
// debconf uses preseeded answers, and the dpkg lock is waited for.
package apt

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hmwassim/debforge/internal/system"
)

// BackportsSuite is the default backports target release.
const BackportsSuite = "trixie-backports"

// ProgressFunc receives progress updates. phase is "download" or
// "install"; pct is 0-100.
type ProgressFunc func(phase string, pct float64, detail string)

// Apt runs apt-get through a system.Runner.
type Apt struct {
	R system.Runner
	// PinFile, when set, is kept in sync with SyncBackportPins before every
	// install/upgrade and after every backports install or removal.
	PinFile string
}

// Transaction is one apt-get install call.
type Transaction struct {
	Install []string
	// Remove is removed in the same transaction (pkg- syntax), used for
	// conflicts so nothing is removed unless the install can proceed.
	Remove []string
	// Target is the -t release, e.g. trixie-backports; empty for default.
	Target string
}

var env = []string{
	"DEBIAN_FRONTEND=noninteractive",
	"DEBCONF_NONINTERACTIVE_SEEN=true",
	"APT_LISTCHANGES_FRONTEND=none",
	"NEEDRESTART_MODE=l",
}

func baseArgs(cmd string) []string {
	return []string{
		"-y", "-q",
		"-o", "APT::Status-Fd=3",
		"-o", "APT::Color=0",
		"-o", "DPkg::Lock::Timeout=300",
		"-o", "Dpkg::Use-Pty=0",
		"-o", "Dpkg::Options::=--force-confdef",
		"-o", "Dpkg::Options::=--force-confold",
		cmd,
	}
}

// Install runs one install transaction.
func (a *Apt) Install(ctx context.Context, t Transaction, prog ProgressFunc) error {
	if len(t.Install) == 0 && len(t.Remove) == 0 {
		return nil
	}
	if err := a.SyncBackportPins(ctx); err != nil {
		return err
	}
	args := baseArgs("install")
	if t.Target != "" {
		args = append(args[:len(args)-1], "-t", t.Target, "install")
	}
	args = append(args, t.Install...)
	for _, r := range t.Remove {
		args = append(args, r+"-")
	}
	if err := a.run(ctx, "install", args, prog); err != nil {
		return err
	}
	if t.Target != "" {
		return a.SyncBackportPins(ctx)
	}
	return nil
}

// Remove removes packages (not purge: conffiles stay, like apt's default).
func (a *Apt) Remove(ctx context.Context, pkgs []string, prog ProgressFunc) error {
	if len(pkgs) == 0 {
		return nil
	}
	if err := a.run(ctx, "remove", append(baseArgs("remove"), pkgs...), prog); err != nil {
		return err
	}
	return a.SyncBackportPins(ctx)
}

// FullUpgrade runs apt-get full-upgrade.
func (a *Apt) FullUpgrade(ctx context.Context, prog ProgressFunc) error {
	if err := a.SyncBackportPins(ctx); err != nil {
		return err
	}
	return a.run(ctx, "full-upgrade", baseArgs("full-upgrade"), prog)
}

// Update runs apt-get update. Per-repository failures (which apt reports
// with exit 0) are returned as warnings.
func (a *Apt) Update(ctx context.Context) (warnings []string, err error) {
	res, err := a.R.Run(ctx, system.Cmd{
		Name: "apt-get", Args: []string{"-q", "-o", "DPkg::Lock::Timeout=300", "update"},
		Env: env, Stdin: strings.NewReader(""), OwnProcessGroup: true,
	})
	for _, l := range strings.Split(string(res.Stdout)+"\n"+string(res.Stderr), "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "W: ") || strings.HasPrefix(l, "E: ") || strings.HasPrefix(l, "Err:") {
			warnings = append(warnings, l)
		}
	}
	if err != nil {
		return warnings, fmt.Errorf("apt-get update failed: %w", err)
	}
	return warnings, nil
}

// PendingUpgrades returns how many packages full-upgrade would change.
func (a *Apt) PendingUpgrades(ctx context.Context) (int, error) {
	res, err := a.R.Run(ctx, system.Cmd{Name: "apt-get", Args: []string{"-s", "-q", "full-upgrade"}, Env: env})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, l := range strings.Split(string(res.Stdout), "\n") {
		if strings.HasPrefix(l, "Inst ") || strings.HasPrefix(l, "Remv ") {
			n++
		}
	}
	return n, nil
}

// PendingInstall returns how many packages "apt-get install [-t target]
// pkgs" would install or upgrade (0 means the request is satisfied).
func (a *Apt) PendingInstall(ctx context.Context, pkgs []string, target string) (int, error) {
	if len(pkgs) == 0 {
		return 0, nil
	}
	args := []string{"-s", "-q"}
	if target != "" {
		args = append(args, "-t", target)
	}
	args = append(append(args, "install"), pkgs...)
	res, err := a.R.Run(ctx, system.Cmd{Name: "apt-get", Args: args, Env: env})
	if err != nil {
		return 0, fmt.Errorf("apt-get -s install: %w", err)
	}
	n := 0
	for _, l := range strings.Split(string(res.Stdout), "\n") {
		if strings.HasPrefix(l, "Inst ") {
			n++
		}
	}
	return n, nil
}

// Preseed feeds debconf answers ("pkg question type value") before install.
func (a *Apt) Preseed(ctx context.Context, lines []string) error {
	if len(lines) == 0 {
		return nil
	}
	_, err := a.R.Run(ctx, system.Cmd{
		Name: "debconf-set-selections", Stdin: strings.NewReader(strings.Join(lines, "\n") + "\n"),
	})
	return err
}

// ErrInterrupted is wrapped when apt finished after the user pressed Ctrl-C.
var ErrInterrupted = errors.New("interrupted")

// run executes apt-get with a Status-Fd pipe. apt runs in its own process
// group and is not cancelled by ctx: once a transaction starts it is
// allowed to finish so dpkg is never left half-configured. A failure while
// ctx was cancelled is wrapped with ErrInterrupted.
func (a *Apt) run(ctx context.Context, verb string, args []string, prog ProgressFunc) error {
	r, w, err := os.Pipe()
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	var errs []string
	var mu sync.Mutex
	wg.Add(1)
	go func() {
		defer wg.Done()
		parseStatus(r, prog, func(msg string) {
			mu.Lock()
			errs = append(errs, msg)
			mu.Unlock()
		})
	}()
	res, runErr := a.R.Run(context.WithoutCancel(ctx), system.Cmd{
		Name: "apt-get", Args: args, Env: env,
		Stdin: strings.NewReader(""), ExtraFiles: []*os.File{w},
		OwnProcessGroup: true,
	})
	w.Close()
	wg.Wait()
	r.Close()

	if runErr != nil {
		for _, l := range strings.Split(string(res.Stdout)+"\n"+string(res.Stderr), "\n") {
			if strings.HasPrefix(l, "E: ") {
				errs = append(errs, strings.TrimPrefix(l, "E: "))
			}
		}
		msg := "apt-get " + verb
		if len(errs) > 0 {
			runErr = fmt.Errorf("%s failed: %s", msg, strings.Join(dedupe(errs), "; "))
		} else {
			runErr = fmt.Errorf("%s failed: %w", msg, runErr)
		}
	}
	// A transaction that completed is reported as success even if the user
	// pressed Ctrl-C meanwhile: the caller must record what was installed
	// before it notices the cancelled context and stops.
	if runErr != nil && ctx.Err() != nil {
		return fmt.Errorf("%w (%w)", runErr, ErrInterrupted)
	}
	return runErr
}

// parseStatus reads APT::Status-Fd lines:
//
//	dlstatus:<pkg>:<percent>:<description>
//	pmstatus:<pkg>:<percent>:<description>
//	pmerror:<pkg>:<percent>:<message>
func parseStatus(r io.Reader, prog ProgressFunc, onErr func(string)) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	last := time.Time{}
	for sc.Scan() {
		kind, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		f := strings.SplitN(rest, ":", 3)
		if len(f) < 3 {
			continue
		}
		pct, _ := strconv.ParseFloat(f[1], 64)
		switch kind {
		case "dlstatus":
			if prog != nil && time.Since(last) > 50*time.Millisecond {
				prog("download", pct, f[2])
				last = time.Now()
			}
		case "pmstatus":
			if prog != nil {
				prog("install", pct, f[2])
			}
		case "pmerror":
			onErr(f[0] + ": " + f[2])
		}
	}
	_, _ = io.Copy(io.Discard, r)
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
