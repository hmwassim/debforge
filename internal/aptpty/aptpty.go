// Package aptpty drives apt-get interactively through a PTY so its native
// progress output can be parsed and turned into spinner updates.
package aptpty

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hmwassim/debforge/internal/dpkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/textutil"
)

// AptExecFunc is the function signature for running apt-get through the PTY.
// Installers hold a field of this type to allow test injection instead of
// calling RunInstall / RunRemove / RunInstallBackports directly.
type AptExecFunc func(ctx context.Context, runner ports.CommandRunner, aptArgs []string, spinner ports.Spinner) error

// AptExec is the default AptExecFunc that runs apt-get through a real PTY.
// It is exported so each installer's NewInstaller can assign it as the
// default; tests can override per-installer.
var AptExec AptExecFunc = run

// DefaultBackportSuite is the default suite used for backport installations
// when the package definition does not specify one. Exported so that other
// packages (installer, setup) can reference the same constant.
const DefaultBackportSuite = "trixie-backports"

const (
	phaseDownload = 0
	phaseInstall  = 1
)

type runState struct {
	phase          int
	overallTotal   int64
	overallLabel   string
	cumulativeDone int64
	prevPkgTotal   int64
	installPkg     string
}

// ---- public API -----------------------------------------------------------

// RunInstall runs apt-get install -y for the given packages.
func RunInstall(ctx context.Context, runner ports.CommandRunner, packages []string, spinner ports.Spinner) error {
	if len(packages) == 0 {
		return nil
	}
	args := append([]string{"install", "-y"}, packages...)
	return run(ctx, runner, args, spinner)
}

// RunInstallBackports runs apt-get install -y -t <suite> for the given packages.
func RunInstallBackports(ctx context.Context, runner ports.CommandRunner, packages []string, suite string, spinner ports.Spinner) error {
	if len(packages) == 0 {
		return nil
	}
	if suite == "" {
		suite = DefaultBackportSuite
	}
	args := append([]string{"install", "-y", "-t", suite}, packages...)
	return run(ctx, runner, args, spinner)
}

// RunRemove runs apt-get remove -y for the given packages.
func RunRemove(ctx context.Context, runner ports.CommandRunner, packages []string, spinner ports.Spinner) error {
	if len(packages) == 0 {
		return nil
	}
	args := append([]string{"remove", "-y"}, packages...)
	return run(ctx, runner, args, spinner)
}

// RunUpdate runs apt-get update to refresh repository metadata.
func RunUpdate(ctx context.Context, runner ports.CommandRunner, spinner ports.Spinner) error {
	spinner.SetDesc("Refreshing repositories...")
	_, _, err := runner.Run(ctx, "apt-get", "update")
	return err
}

// RunUpgrade runs apt-get full-upgrade -y through the PTY so the user sees
// spinner-based per-package progress. full-upgrade handles dependency changes
// that require removing old packages and installing new ones — needed for
// kernel meta-packages (linux-base, linux-headers, linux-image) whose
// versioned dependencies are replaced entirely on each release. upgrade
// alone would silently skip those packages.
func RunUpgrade(ctx context.Context, runner ports.CommandRunner, spinner ports.Spinner) error {
	return AptExec(ctx, runner, []string{"full-upgrade", "-y"}, spinner)
}

// FindInstalledConflicts returns the subset of names that are currently
// installed according to dpkg-query.
func FindInstalledConflicts(ctx context.Context, runner ports.CommandRunner, names []string) []string {
	var found []string
	for _, name := range names {
		ok, err := dpkg.IsInstalled(ctx, runner, name)
		if err != nil {
			continue
		}
		if ok {
			found = append(found, name)
		}
	}
	return found
}

// ---- pre-run: --print-uris ------------------------------------------------

func getDownloadSize(ctx context.Context, runner ports.CommandRunner, mode string, args []string) (int64, string, error) {
	cmdLine := []string{mode, "--print-uris", "-y"}
	cmdLine = append(cmdLine, args...)

	opts := ports.RunOptions{Env: []string{"LC_ALL=C", "LANG=C", "LANGUAGE=C"}}
	out, _, err := runner.RunWithOptions(ctx, opts, "apt-get", cmdLine...)
	if err != nil {
		return 0, "", fmt.Errorf("get download size: %w", err)
	}

	var total int64
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if len(line) == 0 || line[0] != '\'' {
			continue
		}
		f := strings.Fields(line)
		if len(f) >= 3 {
			sz, err := strconv.ParseInt(f[2], 10, 64)
			if err == nil {
				total += sz
			}
		}
	}

	if total > 0 {
		return total, textutil.FormatSize(total), nil
	}
	return 0, "", nil
}
