package service

import (
	"context"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

func (s *InstallService) Update(ctx context.Context, names []string, force, all bool, spinner ports.Spinner) error {
	return withState(ctx, s.locker, s.lockPath, s.state, func(st *State) error {
		if all {
			names = allManagedPackageNames(s.reg, st)
		} else {
			for _, name := range names {
				p, err := LookupPackage(s.reg, name)
				if err != nil {
					return err
				}
				if err := checkInstalled(ctx, s.state, st, name, s.runner, p.Package, p.Type, spinner); err != nil {
					spinner.DoneInfo()
					return err
				}
			}
		}
		return s.processAll(ctx, names, force, true, st, spinner, "update", "updated")
	})
}

func allManagedPackageNames(reg *pkg.Registry, st *State) []string {
	var names []string
	for name := range st.Packages {
		p, ok := reg.Lookup(name)
		if !ok || p.Type == pkg.TypeApt {
			continue
		}
		names = append(names, name)
	}
	return names
}
