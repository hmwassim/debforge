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

// Deps groups the infrastructure dependencies shared by InstallService and
// RemoveService so callers pass one struct instead of 10+ positional params.
type Deps struct {
	Reg      *pkg.Registry
	InstReg  *installer.Registry
	State    *StateManager
	Locker   ports.Locker
	LockPath string
	Runner   ports.CommandRunner
	Fs       ports.FileSystem
	Sys      ports.System
	AptUpd   ports.AptUpdater
	Extrepo  ports.ExtrepoManager
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
}

// InstallService orchestrates the installation of one or more packages
// along with their transitive dependencies.
type InstallService struct {
	baseService
	resolver *Resolver
	execApt  aptpty.AptExecFunc
}

// NewInstallService returns a new InstallService.
func NewInstallService(deps Deps, resolver *Resolver) *InstallService {
	return &InstallService{
		baseService: baseService{
			reg: deps.Reg, instReg: deps.InstReg, state: deps.State, locker: deps.Locker,
			lockPath: deps.LockPath, runner: deps.Runner, fs: deps.Fs, sys: deps.Sys,
			aptUpdate: deps.AptUpd, extrepo: deps.Extrepo,
		},
		resolver: resolver,
		execApt:  aptpty.AptExec,
	}
}
