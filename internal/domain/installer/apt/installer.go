// Package apt implements installer.Installer for apt-type packages (system
// packages managed via apt-get).
package apt

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/hmwassim/debforge/internal/aptpty"
	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

// Installer installs and removes apt packages.
type Installer struct {
	runner  ports.CommandRunner
	fs      ports.FileSystem
	ui      ports.UI
	sys     ports.System
	execApt aptpty.AptExecFunc
}

// NewInstaller returns a new apt Installer.
func NewInstaller(runner ports.CommandRunner, fs ports.FileSystem, ui ports.UI, sys ports.System) *Installer {
	return &Installer{runner: runner, fs: fs, ui: ui, sys: sys, execApt: aptpty.AptExec}
}

// Install installs the apt packages described by p, including extrepo
// setup, conflict resolution, variant selection, and backports.
func (i *Installer) Install(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	if err := installer.AssertType(p.Type, pkg.TypeApt, "apt"); err != nil {
		return err
	}
	if len(p.Packages) == 0 && len(p.Apt.Variants) == 0 {
		return fmt.Errorf("no packages or variants defined for apt type")
	}

	if p.Apt.Variant == skipVariant {
		return nil
	}

	if err := i.checkGPU(ctx, p); err != nil {
		return err
	}

	if err := i.checkConflicts(ctx, p, spinner); err != nil {
		return err
	}

	if !p.SkipRepoSetup {
		if err := i.enableExtrepos(ctx, p, spinner); err != nil {
			return err
		}
	}

	if err := installer.RunPreInstall(ctx, i.runner, spinner, p.Name, p.PreInstall); err != nil {
		return err
	}
	if p.PreInstall != "" {
		if err := aptpty.RunUpdate(ctx, i.runner, spinner); err != nil {
			return fmt.Errorf("apt-get update: %w", err)
		}
	}

	if !p.ForceInstall {
		upToDate, err := i.isUpToDate(ctx, p, spinner)
		if err != nil {
			return err
		}
		if upToDate {
			return nil
		}
	}

	if err := i.SelectVariant(ctx, p); err != nil {
		return err
	}

	if err := i.installBackports(ctx, p, spinner); err != nil {
		return err
	}

	if err := i.installMain(ctx, p, spinner); err != nil {
		return err
	}

	if p.Version == "" {
		if c, err := i.candidateVersion(ctx, primarySystemPackage(p)); err == nil && c != "" {
			p.Version = c
		}
	}

	if err := i.writeConfigs(p, spinner); err != nil {
		return err
	}

	if err := installer.RunPostInstall(ctx, i.runner, spinner, p.Name, p.PostInstall); err != nil {
		return err
	}

	return nil
}

// isUpToDate checks whether all system packages managed by p are already
// at their candidate version. Returns true (no install needed) when every
// package is up to date. On first install (p.Version is "") or when a
// newer candidate is available it returns false so the install proceeds
// and records the candidate version.
func (i *Installer) isUpToDate(ctx context.Context, p *pkg.Package, spinner ports.Spinner) (bool, error) {
	candidate, err := i.candidateVersion(ctx, primarySystemPackage(p))
	if err != nil {
		return false, err
	}
	if candidate == "" {
		return false, nil // can't determine state, proceed
	}
	if p.Version == "" {
		p.Version = candidate
		return false, nil // first install, record version and proceed
	}
	if candidate == p.Version {
		spinner.SetDesc(p.Name + " already up to date")
		return true, nil
	}
	p.Version = candidate
	return false, nil // new candidate available, proceed
}

// candidateVersion returns the candidate version from apt-cache policy
// for the given system package name, or "" when the package is not known
// to the apt cache.
func (i *Installer) candidateVersion(ctx context.Context, pkgName string) (string, error) {
	out, _, err := i.runner.Run(ctx, "apt-cache", "policy", pkgName)
	if err != nil {
		return "", fmt.Errorf("check policy for %s: %w", pkgName, err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Candidate: ") {
			v := strings.TrimPrefix(line, "Candidate: ")
			if v == "(none)" {
				return "", nil
			}
			return v, nil
		}
	}
	return "", nil
}

// Remove removes the apt packages described by p, including extrepo
// cleanup and variant handling.
func (i *Installer) Remove(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	if err := installer.AssertType(p.Type, pkg.TypeApt, "apt"); err != nil {
		return err
	}

	pkgs := removePackages(p)

	if len(pkgs) == 0 {
		return nil
	}

	spinner.SetDesc("removing " + p.Name + "...")

	if err := i.execApt(ctx, i.runner, append([]string{"remove", "-y"}, pkgs...), spinner); err != nil {
		return err
	}

	if err := installer.RunPostRemove(ctx, i.runner, spinner, p.Name, p.PostRemove); err != nil {
		return err
	}

	return nil
}

// ---- GPU check ------------------------------------------------------------

// CheckGPU verifies that an NVIDIA GPU is present when pkgName is "nvidia".
// It returns nil when the package is unrelated or the GPU is detected.
func CheckGPU(ctx context.Context, runner ports.CommandRunner, pkgName string) error {
	if strings.ToLower(pkgName) != "nvidia" {
		return nil
	}
	out, _, err := runner.Run(ctx, "lspci")
	if err != nil {
		return fmt.Errorf("check GPU: %w", err)
	}
	if !strings.Contains(strings.ToLower(string(out)), "nvidia") {
		return fmt.Errorf("NVIDIA GPU required but not found")
	}
	return nil
}

func (i *Installer) checkGPU(ctx context.Context, p *pkg.Package) error {
	return CheckGPU(ctx, i.runner, p.Name)
}

// ---- conflicts ------------------------------------------------------------

func (i *Installer) checkConflicts(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	found := aptpty.FindInstalledConflicts(ctx, i.runner, p.Apt.Conflicts)
	if len(found) == 0 {
		return nil
	}
	return i.execApt(ctx, i.runner, append([]string{"remove", "-y"}, found...), spinner)
}

// ---- extrepo --------------------------------------------------------------

func (i *Installer) enableExtrepos(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	anyEnabled := false
	for _, repo := range p.Apt.Extrepo {
		enabled, err := i.extrepoEnabled(ctx, repo)
		if err != nil {
			return fmt.Errorf("check extrepo %s: %w", repo, err)
		}
		if enabled {
			continue
		}
		spinner.SetDesc("enabling extrepo " + repo)
		if _, _, err := i.runner.Run(ctx, "extrepo", "enable", repo); err != nil {
			return fmt.Errorf("enable extrepo %s: %w", repo, err)
		}
		anyEnabled = true
	}
	if anyEnabled {
		if err := aptpty.RunUpdate(ctx, i.runner, spinner); err != nil {
			return fmt.Errorf("apt-get update: %w", err)
		}
	}
	return nil
}

func (i *Installer) extrepoEnabled(ctx context.Context, repo string) (bool, error) {
	if strings.Contains(repo, "/") || strings.Contains(repo, "..") {
		return false, nil
	}
	path := "/etc/apt/sources.list.d/extrepo_" + repo + ".sources"
	exists, err := i.fs.Exists(path)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}
	data, err := i.fs.ReadFile(path)
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Enabled:") {
			val := strings.TrimSpace(line[8:])
			return val != "no", nil
		}
	}
	return true, nil
}

func (i *Installer) disableExtrepos(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	for _, repo := range p.Apt.Extrepo {
		spinner.SetDesc("disabling extrepo " + repo)
		if _, _, err := i.runner.Run(ctx, "extrepo", "disable", repo); err != nil {
			return fmt.Errorf("disable extrepo %s: %w", repo, err)
		}
	}
	return nil
}

// ---- variant selection ----------------------------------------------------

// skipVariant is the sentinel value used when the user chooses to skip
// installing a variant-only package.
const skipVariant = "__skip__"

// SelectVariant prompts the user to choose a variant when p has multiple
// options (e.g. open vs proprietary GPU drivers). When a variant was
// previously saved on p it is accepted without prompting.
func (i *Installer) SelectVariant(ctx context.Context, p *pkg.Package) error {
	if len(p.Apt.Variants) == 0 {
		p.Apt.Variant = ""
		return nil
	}
	if p.Apt.Variant != "" {
		if _, ok := p.Apt.Variants[p.Apt.Variant]; ok || p.Apt.Variant == skipVariant {
			return nil
		}
		p.Apt.Variant = ""
	}

	names := make([]string, 0, len(p.Apt.Variants))
	for k := range p.Apt.Variants {
		names = append(names, k)
	}
	slices.Sort(names)

	var opts []string
	for i, name := range names {
		opts = append(opts, fmt.Sprintf("  [%d] %s -> %s", i+1, name, strings.Join(p.Apt.Variants[name], ", ")))
	}

	msg := fmt.Sprintf("Select variant for %s:\n  [0] Skip\n%s", p.Name, strings.Join(opts, "\n"))

	i.ui.Info(msg)

	input := i.ui.PromptInput("0", "Variant [%s]", "0")
	if input == "" {
		input = "0"
	}
	if input == "0" {
		p.Apt.Variant = skipVariant
		return nil
	}
	if n, err := strconv.Atoi(input); err == nil && n >= 1 && n <= len(names) {
		p.Apt.Variant = names[n-1]
		return nil
	}
	if slices.Contains(names, input) {
		p.Apt.Variant = input
		return nil
	}
	return fmt.Errorf("invalid variant %q for %s (choose from: %s)", input, p.Name, strings.Join(names, ", "))
}

// ---- backports ------------------------------------------------------------

func (i *Installer) installBackports(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	if len(p.Apt.Backports) == 0 {
		return nil
	}
	spinner.SetDesc("installing backports for " + p.Name)
	suite := p.Apt.BackportSuite
	if suite == "" {
		suite = aptpty.DefaultBackportSuite
	}
	args := append([]string{"install", "-y", "-t", suite}, p.Apt.Backports...)
	return i.execApt(ctx, i.runner, args, spinner)
}

// ---- main packages --------------------------------------------------------

func (i *Installer) installMain(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	pkgs := installPackages(p)
	if len(pkgs) == 0 {
		return nil
	}
	spinner.SetDesc("installing " + p.Name)
	return i.execApt(ctx, i.runner, append([]string{"install", "-y"}, pkgs...), spinner)
}

// ---- config files ---------------------------------------------------------

func (i *Installer) writeConfigs(p *pkg.Package, spinner ports.Spinner) error {
	return installer.WriteAllConfigs(i.fs, i.sys, spinner, p)
}

// ---- batch support --------------------------------------------------------

// CollectExtrepos returns all extrepo repo names that p needs. Used by the
// service layer to enable all extrepos up-front before any installs.
func (i *Installer) CollectExtrepos(p *pkg.Package) []string {
	if p.Apt == nil {
		return nil
	}
	return p.Apt.Extrepo
}

// Prepare does per-package setup without running the main apt-get install.
// It handles GPU check, conflict removal, extrepo setup, pre-install scripts,
// version check, variant selection, and backports. Returns BatchArgs with the
// package names for the batch apt-get install call.
func (i *Installer) Prepare(ctx context.Context, p *pkg.Package, spinner ports.Spinner) (installer.BatchArgs, error) {
	if err := installer.AssertType(p.Type, pkg.TypeApt, "apt"); err != nil {
		return installer.BatchArgs{}, err
	}
	if len(p.Packages) == 0 && len(p.Apt.Variants) == 0 {
		return installer.BatchArgs{}, fmt.Errorf("no packages or variants defined for apt type")
	}

	if p.Apt.Variant == skipVariant {
		return installer.BatchArgs{Skipped: true}, nil
	}

	if err := i.checkGPU(ctx, p); err != nil {
		return installer.BatchArgs{}, err
	}

	if err := i.checkConflicts(ctx, p, spinner); err != nil {
		return installer.BatchArgs{}, err
	}

	if !p.SkipRepoSetup {
		if err := i.enableExtrepos(ctx, p, spinner); err != nil {
			return installer.BatchArgs{}, err
		}
	}

	if err := installer.RunPreInstall(ctx, i.runner, spinner, p.Name, p.PreInstall); err != nil {
		return installer.BatchArgs{}, err
	}
	if p.PreInstall != "" {
		if err := aptpty.RunUpdate(ctx, i.runner, spinner); err != nil {
			return installer.BatchArgs{}, fmt.Errorf("apt-get update: %w", err)
		}
	}

	if !p.ForceInstall {
		upToDate, err := i.isUpToDate(ctx, p, spinner)
		if err != nil {
			return installer.BatchArgs{}, err
		}
		if upToDate {
			return installer.BatchArgs{Skipped: true}, nil
		}
	}

	if err := i.SelectVariant(ctx, p); err != nil {
		return installer.BatchArgs{}, err
	}

	if p.Apt.Variant == skipVariant {
		return installer.BatchArgs{Skipped: true}, nil
	}

	if err := i.installBackports(ctx, p, spinner); err != nil {
		return installer.BatchArgs{}, err
	}

	pkgs := installPackages(p)
	return installer.BatchArgs{AptPkgs: pkgs}, nil
}

// Finalize runs post-install work after the batch apt-get install: version
// recording, config writing, and post-install scripts.
func (i *Installer) Finalize(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	if p.Version == "" {
		if c, err := i.candidateVersion(ctx, primarySystemPackage(p)); err == nil && c != "" {
			p.Version = c
		}
	}

	if err := i.writeConfigs(p, spinner); err != nil {
		return err
	}

	return installer.RunPostInstall(ctx, i.runner, spinner, p.Name, p.PostInstall)
}
