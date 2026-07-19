package service

import (
	"context"
	"errors"

	"github.com/hmwassim/debforge/internal/aptpty"
	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

// ErrNotInstalled is returned when attempting to remove or update a package
// that is not recorded in the state file.
var ErrNotInstalled = errors.New("not installed")

// variantSelector is implemented by installers that allow interactive
// selection of a package variant before the main install flow begins.
type variantSelector interface {
	SelectVariant(ctx context.Context, p *pkg.Package) error
}

type baseService struct {
	reg      *pkg.Registry
	instReg  *installer.Registry
	state    *StateManager
	locker   ports.Locker
	lockPath string
	runner   ports.CommandRunner
	fs       ports.FileSystem
	sys      ports.System
	aptUpdate  ports.AptUpdater
	extrepo    ports.ExtrepoManager
	pkgLister  ports.PackageLister
}

// InstallService orchestrates the installation of one or more packages
// along with their transitive dependencies.
type InstallService struct {
	baseService
	resolver *Resolver
	execApt  aptpty.AptExecFunc
}

// NewInstallService returns a new InstallService.
func NewInstallService(
	reg *pkg.Registry,
	instReg *installer.Registry,
	resolver *Resolver,
	state *StateManager,
	locker ports.Locker,
	lockPath string,
	runner ports.CommandRunner,
	fs ports.FileSystem,
	sys ports.System,
	aptUpdate ports.AptUpdater,
	extrepo ports.ExtrepoManager,
	pkgLister ports.PackageLister,
) *InstallService {
	return &InstallService{
		baseService: baseService{
			reg: reg, instReg: instReg, state: state, locker: locker,
			lockPath: lockPath, runner: runner, fs: fs, sys: sys,
			aptUpdate: aptUpdate, extrepo: extrepo, pkgLister: pkgLister,
		},
		resolver: resolver,
		execApt:  aptpty.AptExec,
	}
}
