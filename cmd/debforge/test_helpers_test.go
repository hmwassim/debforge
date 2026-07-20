package main

import (
	"context"

	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/self"
	"github.com/hmwassim/debforge/internal/service"
	"github.com/hmwassim/debforge/internal/testutil"
)

// mockInstaller is a trivial installer.Installer test double.
type mockInstaller struct {
	installErr error
	removeErr  error
}

func (m *mockInstaller) Install(_ context.Context, _ *pkg.Package, _ ports.Spinner) error {
	return m.installErr
}

func (m *mockInstaller) Remove(_ context.Context, _ *pkg.Package, _ ports.Spinner) error {
	return m.removeErr
}

// mockCmdRunner is a ports.CommandRunner for tests that need to handle
// multiple commands. Each handler is keyed by command name; unmatched
// calls fall through to the default handler (or return nil,nil,nil).
type mockCmdRunner struct {
	handlers map[string]func(ctx context.Context, args ...string) ([]byte, []byte, error)
	def      func(ctx context.Context, name string, args ...string) ([]byte, []byte, error)
}

func (m *mockCmdRunner) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	if h, ok := m.handlers[name]; ok {
		return h(ctx, args...)
	}
	if m.def != nil {
		return m.def(ctx, name, args...)
	}
	return nil, nil, nil
}

func (m *mockCmdRunner) RunWithOptions(ctx context.Context, _ ports.RunOptions, name string, args ...string) ([]byte, []byte, error) {
	return m.Run(ctx, name, args...)
}

var _ ports.CommandRunner = (*mockCmdRunner)(nil)

// newHandlerForTest constructs a commandHandler with the given dependencies,
// skipping the real filesystem/definition loading that newHandler does.
func newHandlerForTest(
	reg *pkg.Registry,
	instReg *installer.Registry,
	stateSvc *service.StateManager,
	locker ports.Locker,
	cfg *self.Config,
	runner ports.CommandRunner,
	fsys ports.FileSystem,
	sys ports.System,
) *commandHandler {
	return &commandHandler{
		reg: reg, instReg: instReg, stateSvc: stateSvc,
		locker: locker, cfg: cfg, runner: runner, fsys: fsys, sys: sys,
		factory: service.NewServiceFactory(service.Deps{
			Reg: reg, InstReg: instReg, State: stateSvc, Locker: locker,
			LockPath: cfg.LockPath, Runner: runner, Fs: fsys, Sys: sys,
			AptUpd: testutil.NopAptUpdater{}, Extrepo: testutil.NopExtrepoManager{},
		}),
		pkgList: testutil.NopPackageLister{},
	}
}

type newHandlerFileInfo struct {
	name  string
	size  int64
	isDir bool
}

func (n newHandlerFileInfo) Name() string { return n.name }
func (n newHandlerFileInfo) Size() int64  { return n.size }
func (n newHandlerFileInfo) IsDir() bool  { return n.isDir }
