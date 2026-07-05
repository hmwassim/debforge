package setup

import (
	"context"

	"github.com/hmwassim/debforge/internal/aptpty"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/testutil"
)

// ---- mock step -------------------------------------------------------------

type mockStep struct {
	name        string
	checkResult CheckResult
	applyErr    error
	applyCalled bool
}

func (s *mockStep) Name() string                                    { return s.name }
func (s *mockStep) Check(_ context.Context, _ *Context) CheckResult { return s.checkResult }
func (s *mockStep) Apply(_ context.Context, _ *Context, _ CheckResult) error {
	s.applyCalled = true
	return s.applyErr
}

// ---- package step helpers --------------------------------------------------

func mockDpkgRunner(result string, err error) *testutil.MockRunner {
	return &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			if err != nil {
				return nil, nil, err
			}
			n := len(args) - 2
			lines := make([]byte, 0, n*10)
			for i := 0; i < n; i++ {
				lines = append(lines, result+"\n"...)
			}
			return lines, nil, nil
		},
	}
}

// ---- config-heavy step helpers --------------------------------------------

func newPkgCfgCx(fs *testutil.MockFileSystem, runner *testutil.MockRunner) *Context {
	if fs == nil {
		fs = testutil.NewMockFileSystem()
	}
	return &Context{
		Fsys:         fs,
		Runner:       runner,
		UI:           &testutil.MockUI{},
		ConfigHashes: make(map[string]string),
	}
}

func pkgCfgRunner(pkgResult string, pkgErr error, extra func(name string, args ...string) ([]byte, []byte, error)) *testutil.MockRunner {
	return &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			if name == "dpkg-query" {
				if pkgErr != nil {
					return nil, nil, pkgErr
				}
				n := len(args) - 2
				lines := make([]byte, 0, n*10)
				for i := 0; i < n; i++ {
					lines = append(lines, pkgResult+"\n"...)
				}
				return lines, nil, nil
			}
			if extra != nil {
				return extra(name, args...)
			}
			return nil, nil, nil
		},
	}
}

// ---- DesktopStep helpers ---------------------------------------------------

type mockSysDesktop struct {
	env map[string]string
}

func (m *mockSysDesktop) IsPrivileged() bool                              { return false }
func (m *mockSysDesktop) Getenv(key string) string                        { return m.env[key] }
func (m *mockSysDesktop) UserHomeDir() (string, error)                    { return "/home/user", nil }
func (m *mockSysDesktop) LookupUser(name string) (*ports.UserInfo, error) { return nil, nil }

func desktopCx(runner *testutil.MockRunner, de string) *Context {
	fs := testutil.NewMockFileSystem()
	return desktopCxWithFs(fs, runner, de)
}

func desktopCxWithFs(fs *testutil.MockFileSystem, runner *testutil.MockRunner, de string) *Context {
	return &Context{
		Fsys:         fs,
		Runner:       runner,
		Sys:          &mockSysDesktop{env: map[string]string{"XDG_CURRENT_DESKTOP": de}},
		UI:           &testutil.MockUI{},
		ConfigHashes: make(map[string]string),
	}
}

func desktopCxSatisfied(runner *testutil.MockRunner, de string) *Context {
	fs := testutil.NewMockFileSystem()
	fs.Files["/home/user/.config/bashrc.d"] = []byte{} // dir marker
	fs.Files["/home/user/.bashrc"] = bashrcDBlock
	return desktopCxWithFs(fs, runner, de)
}

// ---- ReposStep helpers -----------------------------------------------------

func newReposCx(fs *testutil.MockFileSystem, force bool) *Context {
	if fs == nil {
		fs = testutil.NewMockFileSystem()
	}
	return &Context{
		Fsys:         fs,
		Runner:       &testutil.MockRunner{RunFunc: func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) { return nil, nil, nil }},
		UI:           &testutil.MockUI{},
		Force:        force,
		ConfigHashes: make(map[string]string),
	}
}

// ---- UpgradeStep helpers ---------------------------------------------------

func saveAptExec() func() {
	orig := aptpty.AptExec
	return func() { aptpty.AptExec = orig }
}
