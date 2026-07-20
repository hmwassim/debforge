// Package service implements the high-level install, remove, and update
// workflows that coordinate package resolution, execution, and state
// persistence.
package service

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/lockrun"
	"github.com/hmwassim/debforge/internal/ports"
)

// SelectVariants runs interactive variant selection for every package in
// the dependency tree of names. When force is true the saved state variant
// is ignored so the user can choose a different variant on re-install.
// Selected variants are written directly to the registry copy rather than
// persisted to state — state persistence only happens inside processOne
// after a successful install, so a failed download, GPU check, or Ctrl+C
// does not leave a stale variant record.
func (s *InstallService) SelectVariants(ctx context.Context, names []string, force bool) error {
	st, err := s.state.Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	for _, name := range names {
		p, err := LookupPackage(s.reg, name)
		if err != nil {
			return err
		}
		deps, err := s.resolver.Resolve(p)
		if err != nil {
			return fmt.Errorf("resolve deps: %w", err)
		}
		for _, dep := range deps {
			if dep.Apt == nil || len(dep.Apt.Variants) == 0 {
				continue
			}
			if !force && s.state.LookupVariant(st, dep.Name) != "" {
				continue
			}
			inst, err := LookupInstaller(s.instReg, dep.Type)
			if err != nil {
				return err
			}
			vs, ok := inst.(variantSelector)
			if !ok {
				continue
			}
			if err := vs.SelectVariant(ctx, dep); err != nil {
				return err
			}
			if regPkg, ok := s.reg.Lookup(dep.Name); ok && regPkg.Apt != nil {
				regPkg.Apt.Variant = dep.Apt.Variant
			}
		}
	}
	return nil
}

func (s *InstallService) Run(ctx context.Context, names []string, force bool, spinner ports.Spinner) error {
	return withState(ctx, s.locker, s.lockPath, s.state, func(st *State) error {
		pctx := &pipelineCtx{
			st:        st,
			spinner:   spinner,
			force:     force,
			rerun:     force,
			verb:      "install",
			pastTense: "installed",
		}
		return s.processAll(ctx, names, pctx)
	})
}

func withState(ctx context.Context, locker ports.Locker, lockPath string, state *StateManager, fn func(*State) error) error {
	return lockrun.WithLock(ctx, locker, lockPath, func() error {
		st, err := state.Load()
		if err != nil {
			return fmt.Errorf("load state: %w", err)
		}
		return fn(st)
	})
}
