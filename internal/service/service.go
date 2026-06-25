package service

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/lockrun"
	"github.com/hmwassim/debforge/internal/ports"
)

type InstallService struct {
	reg      *pkg.Registry
	instReg  *installer.Registry
	resolver *Resolver
	state    *StateManager
	locker   ports.Locker
	lockPath string
	runner   ports.CommandRunner
	fs       ports.FileSystem
}

func NewInstallService(
	reg *pkg.Registry,
	instReg *installer.Registry,
	resolver *Resolver,
	state *StateManager,
	locker ports.Locker,
	lockPath string,
	runner ports.CommandRunner,
	fs ports.FileSystem,
) *InstallService {
	return &InstallService{
		reg:      reg,
		instReg:  instReg,
		resolver: resolver,
		state:    state,
		locker:   locker,
		lockPath: lockPath,
		runner:   runner,
		fs:       fs,
	}
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
