package services

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/domain/package"
	"github.com/hmwassim/debforge/internal/installers"
	"github.com/hmwassim/debforge/internal/services/dependency"
	"github.com/hmwassim/debforge/internal/services/state"
	"github.com/hmwassim/debforge/internal/ports"
)

type PackageInstaller interface {
	Install(ctx context.Context, pkgNames []string, variants map[string]string, force bool, spinner ports.Spinner) error
}

type InstallService struct {
	pkgRegistry *pkg.Registry
	instReg     *installers.Registry
	stateSvc    *state.Service
	resolver    *dependency.Resolver
	logger      ports.UI
	locker      ports.Locker
	lockPath    string
}

func NewInstallService(
	pkgRegistry *pkg.Registry,
	instReg *installers.Registry,
	stateSvc *state.Service,
	resolver *dependency.Resolver,
	logger ports.UI,
	locker ports.Locker,
	lockPath string,
) *InstallService {
	return &InstallService{
		pkgRegistry: pkgRegistry,
		instReg:     instReg,
		stateSvc:    stateSvc,
		resolver:    resolver,
		logger:      logger,
		locker:      locker,
		lockPath:    lockPath,
	}
}

func (s *InstallService) Install(ctx context.Context, pkgNames []string, variants map[string]string, force bool, spinner ports.Spinner) error {
	for _, pkgName := range pkgNames {
		if err := s.installSingle(ctx, pkgName, variants, force, spinner); err != nil {
			return err
		}
	}
	return nil
}

func (s *InstallService) installSingle(ctx context.Context, pkgName string, variants map[string]string, force bool, spinner ports.Spinner) error {
	release, err := s.locker.Acquire(ctx, s.lockPath)
	if err != nil {
		return fmt.Errorf("acquiring lock: %w", err)
	}
	defer release()

	p, ok := s.pkgRegistry.Lookup(pkgName)
	if !ok {
		return fmt.Errorf("unknown package: %s", pkgName)
	}

	st, err := s.stateSvc.Load()
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	if s.stateSvc.IsInstalled(st, pkgName) && !force {
		spinner.SetDesc(pkgName + " already installed")
		return nil
	}

	if len(p.Variants) > 0 {
		variant := variants[pkgName]
		if variant == "" {
			return fmt.Errorf("package %s has variants but no variant selected", pkgName)
		}
		if entry, exists := st.Packages[pkgName]; exists && entry.Variant == variant && !force {
			spinner.SetDesc(pkgName + " (" + variant + ") already installed")
			return nil
		}
		p = p.Clone()
		p.Variants = map[string]string{variant: p.Variants[variant]}
	}
	if force {
		if len(p.Variants) == 0 {
			p = p.Clone()
		}
		p.ForceInstall = true
	}

	installed := make(map[string]bool)
	for name := range st.Packages {
		installed[name] = true
	}

	ordered, err := s.resolver.Resolve(p, installed, force, variants)
	if err != nil {
		return fmt.Errorf("resolving dependencies: %w", err)
	}

	for _, dep := range ordered {
		savedVersion := ""
		if entry, exists := st.Packages[dep.Name]; exists {
			dep.Version = entry.Version
			savedVersion = entry.Version
		}
		inst, ok := s.instReg.Lookup(dep.Type)
		if !ok {
			return fmt.Errorf("no installer for type %s", dep.Type)
		}
		if err := inst.Install(ctx, dep, spinner); err != nil {
			return fmt.Errorf("installing %s: %w", dep.Name, err)
		}
		entry := state.PkgEntry{Type: string(dep.Type), Version: dep.Version}
		if len(dep.Variants) > 0 {
			for k := range dep.Variants {
				entry.Variant = k
				break
			}
		}
		s.stateSvc.Add(st, dep.Name, entry)
		if err := s.stateSvc.Save(st); err != nil {
			if rmErr := inst.Remove(ctx, dep, nil); rmErr != nil {
				s.logger.Warn("rollback removal of %s after state-save failure: %v", dep.Name, rmErr)
			}
			return fmt.Errorf("saving state after %s: %w", dep.Name, err)
		}
		if dep.Version != savedVersion || savedVersion == "" || force {
			spinner.SetDesc(dep.Name + " installed")
		}
	}

	return nil
}

func (s *InstallService) UpdateSingle(ctx context.Context, pkgName string, spinner ports.Spinner) error {
	release, err := s.locker.Acquire(ctx, s.lockPath)
	if err != nil {
		return fmt.Errorf("acquiring lock: %w", err)
	}
	defer release()

	p, ok := s.pkgRegistry.Lookup(pkgName)
	if !ok {
		return fmt.Errorf("unknown package: %s", pkgName)
	}

	st, err := s.stateSvc.Load()
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	installed := make(map[string]bool)
	for name := range st.Packages {
		installed[name] = true
	}

	ordered, err := s.resolver.Resolve(p, installed, true, nil)
	if err != nil {
		return fmt.Errorf("resolving dependencies: %w", err)
	}

	for _, dep := range ordered {
		savedVersion := ""
		if entry, exists := st.Packages[dep.Name]; exists {
			dep.Version = entry.Version
			savedVersion = entry.Version
		}
		inst, ok := s.instReg.Lookup(dep.Type)
		if !ok {
			return fmt.Errorf("no installer for type %s", dep.Type)
		}
		if err := inst.Install(ctx, dep, spinner); err != nil {
			return fmt.Errorf("installing %s: %w", dep.Name, err)
		}
		entry := state.PkgEntry{Type: string(dep.Type), Version: dep.Version}
		if len(dep.Variants) > 0 {
			for k := range dep.Variants {
				entry.Variant = k
				break
			}
		}
		s.stateSvc.Add(st, dep.Name, entry)
		if err := s.stateSvc.Save(st); err != nil {
			if rmErr := inst.Remove(ctx, dep, nil); rmErr != nil {
				s.logger.Warn("rollback removal of %s after state-save failure: %v", dep.Name, rmErr)
			}
			return fmt.Errorf("saving state after %s: %w", dep.Name, err)
		}
		if dep.Version != savedVersion || savedVersion == "" {
			spinner.SetDesc(dep.Name + " installed")
		}
	}

	return nil
}

var _ PackageInstaller = (*InstallService)(nil)
