package service

import (
	"context"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

// Update reinstalls the named packages. rerun is always true — every
// package reaches the installer so it can check for newer versions.
// force propagates ForceInstall to the entire tree, overriding the
// installer's own version short-circuit.
func (s *InstallService) Update(ctx context.Context, names []string, force, all bool, spinner ports.Spinner) error {
	return withState(ctx, s.locker, s.lockPath, s.state, func(st *State) error {
		if all {
			names = allManagedPackageNames(s.reg, st)
		}

		for _, name := range names {
			p, err := LookupPackage(s.reg, name)
			if err != nil {
				return err
			}
			p = applyVariant(p, st, name)
			if _, err := checkInstalled(ctx, s.state, st, name, s.runner, s.fs, s.sys, p, spinner); err != nil {
				spinner.DoneInfo()
				return err
			}
		}

		return s.processAll(ctx, names, force, true, st, spinner, "update", "updated")
	})
}

// allManagedPackageNames returns every package name tracked in state that
// still has a registered definition. apt packages are included: the apt
// installer's own isUpToDate check (apt-cache policy) short-circuits when
// nothing changed, the same way deb/source already do, so there is no
// wasted work in sweeping them here. This also keeps each apt package's
// recorded Version in state.json from going stale after `update --all`'s
// system-wide apt-get upgrade bumps installed versions out from under it.
func allManagedPackageNames(reg *pkg.Registry, st *State) []string {
	names := make([]string, 0, len(st.Packages))
	for name := range st.Packages {
		if _, ok := reg.Lookup(name); !ok {
			continue
		}
		names = append(names, name)
	}
	return names
}
