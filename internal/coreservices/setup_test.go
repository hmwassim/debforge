package services

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/coresetup"
	config "github.com/hmwassim/debforge/internal/config"
	"github.com/hmwassim/debforge/internal/ports"
)

func TestSetDiff(t *testing.T) {
	tests := []struct {
		name string
		prev []string
		cur  []string
		want []string
	}{
		{name: "nil prev", prev: nil, cur: []string{"a", "b"}, want: nil},
		{name: "empty prev", prev: []string{}, cur: []string{"a", "b"}, want: nil},
		{name: "no diff", prev: []string{"a", "b"}, cur: []string{"a", "b"}, want: nil},
		{name: "some removed", prev: []string{"a", "b", "c"}, cur: []string{"a"}, want: []string{"b", "c"}},
		{name: "all removed", prev: []string{"a", "b"}, cur: []string{}, want: []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := setDiff(tt.prev, tt.cur)
			if len(got) != len(tt.want) {
				t.Fatalf("setDiff(%v, %v) = %v, want %v", tt.prev, tt.cur, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("setDiff(%v, %v) = %v, want %v", tt.prev, tt.cur, got, tt.want)
				}
			}
		})
	}
}

func TestContainsLine(t *testing.T) {
	tests := []struct {
		haystack string
		needle   string
		want     bool
	}{
		{haystack: "a\nb\nc", needle: "b", want: true},
		{haystack: "a\nb\nc", needle: "d", want: false},
		{haystack: "abc", needle: "abc", want: true},
		{haystack: "abc", needle: "ab", want: false},
		{haystack: "", needle: "", want: true},
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := containsLine(tt.haystack, tt.needle)
			if got != tt.want {
				t.Fatalf("containsLine(%q, %q) = %v, want %v", tt.haystack, tt.needle, got, tt.want)
			}
		})
	}
}

func TestEnsureSourcesListAlreadyTrixie(t *testing.T) {
	fs := newMemFS()
	fs.WriteFile("/etc/apt/sources.list", []byte("deb trixie main"), 0644)

	data, _ := fs.ReadFile("/etc/apt/sources.list")
	if !strings.Contains(string(data), "trixie") {
		t.Fatal("expected trixie to be present")
	}
}

func TestEnsureSourcesListAlreadyHasTrixie(t *testing.T) {
	ctx := context.Background()
	fs := newMemFS()
	fs.WriteFile("/etc/apt/sources.list", []byte("some trixie content"), 0644)
	cfg := &config.Config{SourcesListPath: "/etc/apt/sources.list"}

	s := &SetupService{fs: fs, cfg: cfg, runner: &mockRunner{}, logger: &mockUI{}}
	if err := s.ensureSourcesList(ctx); err != nil {
		t.Fatalf("ensureSourcesList: %v", err)
	}

	data, _ := fs.ReadFile("/etc/apt/sources.list")
	if string(data) != "some trixie content" {
		t.Fatalf("expected unchanged content, got %q", string(data))
	}
}

func TestEnablei386AlreadyEnabled(t *testing.T) {
	ctx := context.Background()
	runner := &mockRunner{stdout: []byte("i386\namd64")}
	s := &SetupService{runner: runner}
	if err := s.enablei386(ctx); err != nil {
		t.Fatalf("enablei386: %v", err)
	}
}

func TestEnablei386AddsArch(t *testing.T) {
	ctx := context.Background()
	runner := &mockRunner{stdout: []byte("amd64")}
	s := &SetupService{runner: runner}
	if err := s.enablei386(ctx); err != nil {
		t.Fatalf("enablei386: %v", err)
	}
}

func TestInstallFlathubAlreadyConfigured(t *testing.T) {
	ctx := context.Background()
	runner := &mockRunner{stdout: []byte("flathub\nsome-other")}
	s := &SetupService{runner: runner, logger: &mockUI{}}
	if err := s.installFlathub(ctx); err != nil {
		t.Fatalf("installFlathub: %v", err)
	}
}

func TestInstallFlathubNotConfigured(t *testing.T) {
	ctx := context.Background()
	runner := &mockRunner{stdout: []byte("some-other")}
	s := &SetupService{runner: runner, logger: &mockUI{}}
	if err := s.installFlathub(ctx); err != nil {
		t.Fatalf("installFlathub: %v", err)
	}
}

type setupMockLocker struct {
	acquireErr error
}

func (m *setupMockLocker) Acquire(ctx context.Context, path string) (func(), error) {
	if m.acquireErr != nil {
		return nil, m.acquireErr
	}
	return func() {}, nil
}

func TestSetupServiceRunNeedsRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("skipping root check when running as root")
	}
	s := &SetupService{}
	err := s.Run(context.Background(), false)
	if err == nil {
		t.Fatal("expected error when not root")
	}
	if !strings.Contains(err.Error(), "must be run as root") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateSourcesBackup(t *testing.T) {
	ctx := context.Background()
	fs := newMemFS()
	fs.WriteFile("/etc/apt/sources.list", []byte("original"), 0644)

	s := &SetupService{fs: fs, runner: &mockRunner{}, logger: &mockUI{}}
	if err := s.createSourcesBackupOnce(ctx, "/etc/apt/sources.list.debforge-backup", "/etc/apt/sources.list"); err != nil {
		t.Fatalf("createSourcesBackupOnce: %v", err)
	}

	data, _ := fs.ReadFile("/etc/apt/sources.list.debforge-backup")
	if string(data) != "original" {
		t.Fatalf("backup content = %q, want %q", string(data), "original")
	}
}

func TestCreateSourcesBackupAlreadyExists(t *testing.T) {
	ctx := context.Background()
	fs := newMemFS()
	fs.WriteFile("/etc/apt/sources.list.debforge-backup", []byte("existing backup"), 0644)

	s := &SetupService{fs: fs, runner: &mockRunner{}, logger: &mockUI{}}
	if err := s.createSourcesBackupOnce(ctx, "/etc/apt/sources.list.debforge-backup", "/etc/apt/sources.list"); err != nil {
		t.Fatalf("createSourcesBackupOnce: %v", err)
	}

	data, _ := fs.ReadFile("/etc/apt/sources.list.debforge-backup")
	if string(data) != "existing backup" {
		t.Fatalf("backup was overwritten, got %q", string(data))
	}
}

var _ ports.CommandRunner = (*mockRunner)(nil)
var _ ports.UI = (*mockUI)(nil)

type mockApt struct {
	installed map[string]bool
	checkErr  map[string]error
}

func (m *mockApt) Install(ctx context.Context, packages []string) error { return nil }
func (m *mockApt) InstallBackports(ctx context.Context, packages []string, suite string) error {
	return nil
}
func (m *mockApt) Remove(ctx context.Context, packages []string) error { return nil }
func (m *mockApt) Update(ctx context.Context) error                    { return nil }
func (m *mockApt) Upgrade(ctx context.Context) error                   { return nil }
func (m *mockApt) CheckInstalled(ctx context.Context, pkg string) (bool, error) {
	if err := m.checkErr[pkg]; err != nil {
		return false, err
	}
	return m.installed[pkg], nil
}

func TestVerifySetupAllInstalled(t *testing.T) {
	ctx := context.Background()
	defs := []coresetup.GroupDef{
		{
			Name:     "test-group",
			Packages: []string{"pkg-a", "pkg-b"},
			Configs: []coresetup.ConfigDef{
				{Dest: "/etc/test.conf", Content: "test-content", Mode: 0644},
			},
			Services: []string{"test-service"},
		},
	}
	fs := newMemFS()
	fs.WriteFile("/etc/test.conf", []byte("test-content"), 0644)
	apt := &mockApt{installed: map[string]bool{"pkg-a": true, "pkg-b": true}}
	runner := &mockRunner{}
	s := &SetupService{apt: apt, fs: fs, runner: runner}

	if !s.verifySetup(ctx, defs) {
		t.Fatal("expected verifySetup to return true")
	}
}

func TestVerifySetupPackageMissing(t *testing.T) {
	ctx := context.Background()
	defs := []coresetup.GroupDef{
		{
			Name:     "test-group",
			Packages: []string{"pkg-a", "pkg-b"},
		},
	}
	apt := &mockApt{installed: map[string]bool{"pkg-a": true, "pkg-b": false}}
	s := &SetupService{apt: apt}

	if s.verifySetup(ctx, defs) {
		t.Fatal("expected verifySetup to return false when package is missing")
	}
}

func TestVerifySetupPackageCheckError(t *testing.T) {
	ctx := context.Background()
	defs := []coresetup.GroupDef{
		{
			Name:     "test-group",
			Packages: []string{"pkg-a"},
		},
	}
	apt := &mockApt{
		installed: map[string]bool{"pkg-a": true},
		checkErr:  map[string]error{"pkg-a": os.ErrPermission},
	}
	s := &SetupService{apt: apt}

	if s.verifySetup(ctx, defs) {
		t.Fatal("expected verifySetup to return false when CheckInstalled returns error")
	}
}

func TestVerifySetupConfigWrongContent(t *testing.T) {
	ctx := context.Background()
	defs := []coresetup.GroupDef{
		{
			Configs: []coresetup.ConfigDef{
				{Dest: "/etc/test.conf", Content: "expected", Mode: 0644},
			},
		},
	}
	fs := newMemFS()
	fs.WriteFile("/etc/test.conf", []byte("wrong"), 0644)
	s := &SetupService{fs: fs}

	if s.verifySetup(ctx, defs) {
		t.Fatal("expected verifySetup to return false when config content differs")
	}
}

func TestVerifySetupConfigWrongPerms(t *testing.T) {
	ctx := context.Background()
	defs := []coresetup.GroupDef{
		{
			Configs: []coresetup.ConfigDef{
				{Dest: "/etc/test.conf", Content: "data", Mode: 0600},
			},
		},
	}
	fs := newMemFS()
	fs.WriteFile("/etc/test.conf", []byte("data"), 0644)
	s := &SetupService{fs: fs}

	if s.verifySetup(ctx, defs) {
		t.Fatal("expected verifySetup to return false when config permissions differ")
	}
}

func TestVerifySetupConfigMissing(t *testing.T) {
	ctx := context.Background()
	defs := []coresetup.GroupDef{
		{
			Configs: []coresetup.ConfigDef{
				{Dest: "/etc/missing.conf", Content: "data", Mode: 0644},
			},
		},
	}
	fs := newMemFS()
	s := &SetupService{fs: fs}

	if s.verifySetup(ctx, defs) {
		t.Fatal("expected verifySetup to return false when config file missing")
	}
}

func TestVerifySetupServiceNotActive(t *testing.T) {
	ctx := context.Background()
	defs := []coresetup.GroupDef{
		{
			Services: []string{"test-service"},
		},
	}
	runner := &mockRunner{err: os.ErrNotExist}
	s := &SetupService{runner: runner}

	if s.verifySetup(ctx, defs) {
		t.Fatal("expected verifySetup to return false when service not active")
	}
}

type seqRunner struct {
	results []struct {
		stdout []byte
		stderr []byte
		err    error
	}
	call int
}

func (r *seqRunner) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	if r.call >= len(r.results) {
		return nil, nil, nil
	}
	res := r.results[r.call]
	r.call++
	return res.stdout, res.stderr, res.err
}

func (r *seqRunner) RunWithEnv(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, []byte, error) {
	return nil, nil, nil
}

func (r *seqRunner) RunWithSpinner(ctx context.Context, name string, args ...string) error {
	return nil
}

func TestVerifySetupServiceNotEnabled(t *testing.T) {
	ctx := context.Background()
	defs := []coresetup.GroupDef{
		{
			Services: []string{"test-service"},
		},
	}
	runner := &seqRunner{
		results: []struct {
			stdout []byte
			stderr []byte
			err    error
		}{
			{stdout: []byte("active")}, // is-active succeeds
			{err: os.ErrNotExist},      // is-enabled fails
		},
	}
	s := &SetupService{runner: runner}

	if s.verifySetup(ctx, defs) {
		t.Fatal("expected verifySetup to return false when service not enabled")
	}
}

func TestVerifySetupEmptyDefs(t *testing.T) {
	ctx := context.Background()
	s := &SetupService{}
	if !s.verifySetup(ctx, nil) {
		t.Fatal("expected verifySetup to return true for empty defs")
	}
}

func TestEnsureResolvSymlinkCreatesWhenMissing(t *testing.T) {
	cfg := &config.Config{
		StubResolvConfPath: "/run/systemd/resolve/stub-resolv.conf",
		ResolvConfPath:     "/etc/resolv.conf",
	}
	fs := newMemFS()
	fs.WriteFile("/run/systemd/resolve/stub-resolv.conf", []byte("nameserver 127.0.0.53"), 0644)
	s := &SetupService{fs: fs, cfg: cfg, logger: &mockUI{}}

	if err := s.ensureResolvSymlink(); err != nil {
		t.Fatalf("ensureResolvSymlink: %v", err)
	}
	target, err := fs.Readlink("/etc/resolv.conf")
	if err != nil {
		t.Fatalf("expected symlink to be created: %v", err)
	}
	if target != "/run/systemd/resolve/stub-resolv.conf" {
		t.Fatalf("symlink target = %q, want %q", target, "/run/systemd/resolve/stub-resolv.conf")
	}
}

func TestEnsureResolvSymlinkAlreadyCorrect(t *testing.T) {
	cfg := &config.Config{
		StubResolvConfPath: "/run/systemd/resolve/stub-resolv.conf",
		ResolvConfPath:     "/etc/resolv.conf",
	}
	fs := newMemFS()
	fs.Symlink("/run/systemd/resolve/stub-resolv.conf", "/etc/resolv.conf")
	s := &SetupService{fs: fs, cfg: cfg, logger: &mockUI{}}

	if err := s.ensureResolvSymlink(); err != nil {
		t.Fatalf("ensureResolvSymlink: %v", err)
	}
}

func TestEnsureResolvSymlinkUpdatesStale(t *testing.T) {
	cfg := &config.Config{
		StubResolvConfPath: "/run/systemd/resolve/stub-resolv.conf",
		ResolvConfPath:     "/etc/resolv.conf",
	}
	fs := newMemFS()
	fs.Symlink("/old/stub-resolv.conf", "/etc/resolv.conf")
	s := &SetupService{fs: fs, cfg: cfg, logger: &mockUI{}}

	if err := s.ensureResolvSymlink(); err != nil {
		t.Fatalf("ensureResolvSymlink: %v", err)
	}
	target, err := fs.Readlink("/etc/resolv.conf")
	if err != nil {
		t.Fatalf("expected symlink to exist: %v", err)
	}
	if target != "/run/systemd/resolve/stub-resolv.conf" {
		t.Fatalf("symlink target = %q, want %q", target, "/run/systemd/resolve/stub-resolv.conf")
	}
}

func TestEnsureResolvSymlinkSkipsRegularFile(t *testing.T) {
	cfg := &config.Config{
		StubResolvConfPath: "/run/systemd/resolve/stub-resolv.conf",
		ResolvConfPath:     "/etc/resolv.conf",
	}
	fs := newMemFS()
	fs.WriteFile("/etc/resolv.conf", []byte("nameserver 8.8.8.8"), 0644)
	s := &SetupService{fs: fs, cfg: cfg, logger: &mockUI{}}

	if err := s.ensureResolvSymlink(); err != nil {
		t.Fatalf("ensureResolvSymlink: %v", err)
	}
	data, _ := fs.ReadFile("/etc/resolv.conf")
	if string(data) != "nameserver 8.8.8.8" {
		t.Fatalf("expected file content unchanged, got %q", string(data))
	}
}

func TestEnsureResolvSymlinkLstatError(t *testing.T) {
	cfg := &config.Config{
		StubResolvConfPath: "/run/systemd/resolve/stub-resolv.conf",
		ResolvConfPath:     "/etc/resolv.conf",
	}
	fs := &errorFS{err: os.ErrPermission}
	s := &SetupService{fs: fs, cfg: cfg, logger: &mockUI{}}

	if err := s.ensureResolvSymlink(); err == nil {
		t.Fatal("expected error from Lstat failure")
	}
}

type errorFS struct {
	err error
}

func (e *errorFS) ReadFile(name string) ([]byte, error)                             { return nil, e.err }
func (e *errorFS) WriteFile(name string, data []byte, perm os.FileMode) error       { return e.err }
func (e *errorFS) AtomicWriteFile(name string, data []byte, perm os.FileMode) error { return e.err }
func (e *errorFS) ReadDir(name string) ([]os.DirEntry, error)                       { return nil, e.err }
func (e *errorFS) Stat(name string) (os.FileInfo, error)                            { return nil, e.err }
func (e *errorFS) Lstat(name string) (os.FileInfo, error)                           { return nil, e.err }
func (e *errorFS) MkdirAll(path string, perm os.FileMode) error                     { return e.err }
func (e *errorFS) MkdirTemp(dir, pattern string) (string, error)                    { return "", e.err }
func (e *errorFS) RemoveAll(path string) error                                      { return e.err }
func (e *errorFS) Chmod(name string, mode os.FileMode) error                        { return e.err }
func (e *errorFS) Rename(oldPath, newPath string) error                             { return e.err }
func (e *errorFS) Symlink(target, link string) error                                { return e.err }
func (e *errorFS) Readlink(name string) (string, error)                             { return "", e.err }

func TestEnsureResolvSymlinkTargetMissingWarns(t *testing.T) {
	cfg := &config.Config{
		StubResolvConfPath: "/run/systemd/resolve/stub-resolv.conf",
		ResolvConfPath:     "/etc/resolv.conf",
	}
	fs := newMemFS()
	s := &SetupService{fs: fs, cfg: cfg, logger: &mockUI{}}

	if err := s.ensureResolvSymlink(); err != nil {
		t.Fatalf("ensureResolvSymlink: %v", err)
	}
}
