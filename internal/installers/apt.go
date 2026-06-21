package installers

import (
	"context"
	"fmt"
	"sync"

	"github.com/hmwassim/debforge/internal/domain/apt"
	"github.com/hmwassim/debforge/internal/domain/deployer"
	"github.com/hmwassim/debforge/internal/domain/package"
	"github.com/hmwassim/debforge/internal/ports"
)

type AptInstaller struct {
	svc      apt.Service
	deployer *deployer.Deployer
	repos    *RepoManager
	logger   ports.UI

	mu      sync.Mutex
	updated bool
}

func NewAptInstaller(svc apt.Service, d *deployer.Deployer, repos *RepoManager, logger ports.UI) *AptInstaller {
	return &AptInstaller{svc: svc, deployer: d, repos: repos, logger: logger}
}

func (i *AptInstaller) ensureUpdated(ctx context.Context) error {
	i.mu.Lock()
	if i.updated {
		i.mu.Unlock()
		return nil
	}
	i.mu.Unlock()

	if err := i.svc.Update(ctx); err != nil {
		return fmt.Errorf("updating package lists: %w", err)
	}

	i.mu.Lock()
	i.updated = true
	i.mu.Unlock()
	return nil
}

func (i *AptInstaller) Install(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	if p.Type != pkg.TypeApt {
		return fmt.Errorf("apt installer called for type %s", p.Type)
	}

	if err := i.repos.EnsureRepo(ctx, p); err != nil {
		return err
	}

	if err := i.ensureUpdated(ctx); err != nil {
		return err
	}

	if len(p.Conflicts) > 0 {
		if err := i.svc.Remove(ctx, p.Conflicts, spinner); err != nil {
			return fmt.Errorf("removing conflicts: %w", err)
		}
	}

	var installPkgs []string
	installPkgs = append(installPkgs, p.Packages...)
	if p.Primary != "" {
		installPkgs = append([]string{p.Primary}, installPkgs...)
	}
	if err := i.svc.Install(ctx, installPkgs, spinner); err != nil {
		return err
	}

	if len(p.Backports) > 0 {
		if err := i.svc.InstallBackports(ctx, p.Backports, "", spinner); err != nil {
			return fmt.Errorf("installing backports: %w", err)
		}
	}

	if err := i.deployer.DeployPackageConfigs(ctx, p.Configs, p.UserConfigs); err != nil {
		return err
	}

	if err := i.deployer.RunPostInstall(ctx, p.PostInstall); err != nil {
		i.logger.Warn("post-install: %s", err)
	}

	return nil
}

func (i *AptInstaller) Remove(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	if p.Type != pkg.TypeApt {
		return fmt.Errorf("apt installer called for type %s", p.Type)
	}

	var removePkgs []string
	if p.Primary != "" {
		removePkgs = append(removePkgs, p.Primary)
	} else {
		removePkgs = append(removePkgs, p.Packages...)
	}
	if err := i.svc.Remove(ctx, removePkgs, spinner); err != nil {
		return err
	}

	i.cleanupUserConfigs(ctx, p)

	i.repos.CleanupRepo(ctx, p)

	if err := i.svc.Update(ctx); err != nil {
		return fmt.Errorf("updating after removal: %w", err)
	}

	return nil
}

func (i *AptInstaller) Update(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	i.mu.Lock()
	i.updated = false
	i.mu.Unlock()
	return i.Install(ctx, p, spinner)
}

func (i *AptInstaller) cleanupUserConfigs(ctx context.Context, p *pkg.Package) {
	user, err := deployer.InvokingUser()

	if err != nil {
		i.logger.Warn("cannot determine invoking user: %v", err)
		return
	}
	if err := i.deployer.RemoveUserConfigs(ctx, p.UserConfigs, user); err != nil {
		i.logger.Warn("cleaning user configs: %v", err)
	}
}

var _ Installer = (*AptInstaller)(nil)
