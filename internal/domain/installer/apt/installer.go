package apt

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/hmwassim/debforge/internal/aptpty"
	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

type Installer struct {
	runner ports.CommandRunner
	fs     ports.FileSystem
	ui     ports.UI
}

func NewInstaller(runner ports.CommandRunner, fs ports.FileSystem, ui ports.UI) *Installer {
	return &Installer{runner: runner, fs: fs, ui: ui}
}

func (i *Installer) Install(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	if p.Type != pkg.TypeApt {
		return fmt.Errorf("apt installer called for type %s", p.Type)
	}
	if len(p.Packages) == 0 && len(p.Apt.Variants) == 0 {
		return fmt.Errorf("no packages or variants defined for apt type")
	}

	if err := i.checkConflicts(ctx, p, spinner); err != nil {
		return err
	}

	if !p.SkipRepoSetup {
		if err := i.enableExtrepos(ctx, p, spinner); err != nil {
			return err
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

	if err := i.selectVariant(ctx, p, spinner); err != nil {
		return err
	}

	if err := i.installBackports(ctx, p, spinner); err != nil {
		return err
	}

	if err := i.installMain(ctx, p, spinner); err != nil {
		return err
	}

	if err := i.writeConfigs(ctx, p, spinner); err != nil {
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
	name := p.PrimarySystemPackage()
	candidate, err := i.candidateVersion(ctx, name)
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

func (i *Installer) Remove(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	if p.Type != pkg.TypeApt {
		return fmt.Errorf("apt installer called for type %s", p.Type)
	}

	pkgs := p.Packages
	if len(p.Remove) > 0 {
		pkgs = p.Remove
	}
	if p.Apt.Variant != "" {
		if v, ok := p.Apt.Variants[p.Apt.Variant]; ok {
			pkgs = append(pkgs, v)
		}
	}

	if len(pkgs) == 0 {
		return nil
	}

	spinner.SetDesc("removing " + p.Name + "...")

	if err := aptpty.RunRemove(ctx, i.runner, pkgs, spinner); err != nil {
		return err
	}

	i.disableExtrepos(ctx, p, spinner)

	if err := installer.RunPostRemove(ctx, i.runner, spinner, p.Name, p.PostRemove); err != nil {
		return err
	}

	return nil
}

// ---- conflicts ------------------------------------------------------------

func (i *Installer) checkConflicts(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	found := aptpty.FindInstalledConflicts(ctx, i.runner, p.Apt.Conflicts)
	if len(found) == 0 {
		return nil
	}
	return aptpty.RunRemove(ctx, i.runner, found, spinner)
}

// ---- extrepo --------------------------------------------------------------

func (i *Installer) enableExtrepos(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	for _, repo := range p.Apt.Extrepo {
		spinner.SetDesc("enabling extrepo " + repo)
		if _, _, err := i.runner.Run(ctx, "extrepo", "enable", repo); err != nil {
			return fmt.Errorf("enable extrepo %s: %w", repo, err)
		}
	}
	if len(p.Apt.Extrepo) > 0 {
		spinner.SetDesc("refreshing package list...")
		if _, _, err := i.runner.Run(ctx, "apt-get", "update"); err != nil {
			return fmt.Errorf("apt-get update: %w", err)
		}
	}
	return nil
}

func (i *Installer) disableExtrepos(ctx context.Context, p *pkg.Package, spinner ports.Spinner) {
	for _, repo := range p.Apt.Extrepo {
		spinner.SetDesc("disabling extrepo " + repo)
		if _, _, err := i.runner.Run(ctx, "extrepo", "disable", repo); err != nil {
			// best-effort
		}
	}
}

// ---- variant selection ----------------------------------------------------

func (i *Installer) selectVariant(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	if len(p.Apt.Variants) == 0 {
		return nil
	}
	if p.Apt.Variant != "" {
		return nil
	}

	var names []string
	for name := range p.Apt.Variants {
		names = append(names, name)
	}
	sort.Strings(names)

	var opts []string
	for _, name := range names {
		opts = append(opts, fmt.Sprintf("  %s -> %s", name, p.Apt.Variants[name]))
	}

	msg := fmt.Sprintf("Select variant for %s:\n%s", p.Name, strings.Join(opts, "\n"))

	i.ui.Info(msg)

	input := i.ui.PromptInput(names[0], "Variant [%s]", names[0])
	if input == "" {
		input = names[0]
	}
	// validate
	valid := false
	for _, name := range names {
		if input == name {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid variant %q for %s (choose from: %s)", input, p.Name, strings.Join(names, ", "))
	}

	p.Apt.Variant = input
	return nil
}

// ---- backports ------------------------------------------------------------

func (i *Installer) installBackports(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	if len(p.Apt.Backports) == 0 {
		return nil
	}
	spinner.SetDesc("installing backports for " + p.Name)
	return aptpty.RunInstallBackports(ctx, i.runner, p.Apt.Backports, "", spinner)
}

// ---- main packages --------------------------------------------------------

func (i *Installer) installMain(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	pkgs := p.Packages
	if p.Apt.Variant != "" {
		if v, ok := p.Apt.Variants[p.Apt.Variant]; ok {
			pkgs = append(pkgs, v)
		}
	}
	if len(pkgs) == 0 {
		return nil
	}
	spinner.SetDesc("installing " + p.Name)
	return aptpty.RunInstall(ctx, i.runner, pkgs, spinner)
}

// ---- config files ---------------------------------------------------------

func (i *Installer) writeConfigs(_ context.Context, p *pkg.Package, spinner ports.Spinner) error {
	return installer.WriteConfigs(i.fs, spinner, p)
}
