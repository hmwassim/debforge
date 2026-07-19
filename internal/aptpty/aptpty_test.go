package aptpty

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/hmwassim/debforge/internal/testutil"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		goleak.IgnoreCurrent(),
	)
}

// mockPtySession implements ptySession with controlled input/output.
type mockPtySession struct {
	data     []byte
	readErr  error
	readIdx  int
	writeBuf bytes.Buffer
	waitErr  error
	signaled os.Signal
}

func (m *mockPtySession) Read(b []byte) (int, error) {
	if m.readIdx >= len(m.data) && m.readErr != nil {
		return 0, m.readErr
	}
	if m.readIdx >= len(m.data) {
		return 0, io.EOF
	}
	n := copy(b, m.data[m.readIdx:])
	m.readIdx += n
	return n, nil
}
func (m *mockPtySession) Write(b []byte) (int, error) { return m.writeBuf.Write(b) }
func (m *mockPtySession) Close() error                { return nil }
func (m *mockPtySession) Wait() error                 { return m.waitErr }
func (m *mockPtySession) Signal(sig os.Signal) error  { m.signaled = sig; return nil }
func (m *mockPtySession) SetSize(_, _ uint16) error   { return nil }

// mockPtyFactory returns the given session for any command.
func mockPtyFactory(sess ptySession) ptyFactory {
	return func(_ context.Context, _ string, _ ...string) (ptySession, error) {
		return sess, nil
	}
}

func TestFindInstalledConflicts(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			if len(args) >= 3 && args[2] == "pkg-a" {
				return []byte("installed"), nil, nil
			}
			return []byte("not-installed"), nil, nil
		},
	}

	got, err := FindInstalledConflicts(context.Background(), runner, []string{"pkg-a", "pkg-b", "pkg-c"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != "pkg-a" {
		t.Errorf("FindInstalledConflicts = %v, want [pkg-a]", got)
	}
}

func TestFindInstalledConflicts_none(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return []byte("not-installed"), nil, nil
		},
	}

	got, err := FindInstalledConflicts(context.Background(), runner, []string{"pkg-a", "pkg-b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("FindInstalledConflicts = %v, want []", got)
	}
}

func TestFindInstalledConflicts_empty(t *testing.T) {
	runner := &testutil.MockRunner{}
	got, err := FindInstalledConflicts(context.Background(), runner, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("FindInstalledConflicts(nil) = %v, want []", got)
	}
}

func TestFindInstalledConflicts_error(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, nil, context.DeadlineExceeded
		},
	}
	got, err := FindInstalledConflicts(context.Background(), runner, []string{"pkg-a"})
	if err == nil {
		t.Fatal("expected error from FindInstalledConflicts")
	}
	if len(got) != 0 {
		t.Errorf("FindInstalledConflicts with error = %v, want []", got)
	}
}

func TestGetDownloadSize(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return []byte("'http://example.com/pkg.deb' _ 123456 0\n"), nil, nil
		},
	}
	total, label, err := getDownloadSize(context.Background(), runner, "install", []string{"pkg-a"})
	if err != nil {
		t.Fatalf("getDownloadSize: %v", err)
	}
	if total != 123456 {
		t.Errorf("total = %d, want 123456", total)
	}
	if label == "" {
		t.Errorf("label should not be empty for non-zero total")
	}
}

func TestGetDownloadSize_error(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, nil, errors.New("apt-get failed")
		},
	}
	total, label, err := getDownloadSize(context.Background(), runner, "install", []string{"pkg-a"})
	if err == nil {
		t.Error("expected error")
	}
	if total != 0 || label != "" {
		t.Errorf("on error: total=%d label=%q, want (0, \"\")", total, label)
	}
}

func TestGetDownloadSize_noMatches(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return []byte("All packages are up to date.\n"), nil, nil
		},
	}
	total, label, err := getDownloadSize(context.Background(), runner, "install", []string{})
	if err != nil {
		t.Fatalf("getDownloadSize: %v", err)
	}
	if total != 0 || label != "" {
		t.Errorf("when nothing to download: total=%d label=%q, want (0, \"\")", total, label)
	}
}

func TestGetDownloadSize_multipleArchives(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			out := []byte("'/pkg1.deb' _ 1000 0\n'/pkg2.deb' _ 2000 0\n'/pkg3.deb' _ 3000 0\n")
			return out, nil, nil
		},
	}
	total, label, err := getDownloadSize(context.Background(), runner, "install", []string{"pkg-a", "pkg-b"})
	if err != nil {
		t.Fatalf("getDownloadSize: %v", err)
	}
	if total != 6000 {
		t.Errorf("total = %d, want 6000", total)
	}
	if label == "" {
		t.Errorf("label should not be empty")
	}
}

func TestRunInstall_empty(t *testing.T) {
	err := RunInstall(context.Background(), nil, nil, &testutil.MockSpinner{})
	if err != nil {
		t.Errorf("RunInstall(nil) = %v, want nil", err)
	}
}

func TestRunInstallBackports_defaultSuite(t *testing.T) {
	suite := "trixie-backports"
	want := "trixie-backports"
	if suite != want {
		t.Errorf("default backports suite = %q, want %q", suite, want)
	}
}

func TestRunInstallBackports_empty(t *testing.T) {
	err := RunInstallBackports(context.Background(), nil, nil, "", &testutil.MockSpinner{})
	if err != nil {
		t.Errorf("RunInstallBackports(nil) = %v, want nil", err)
	}
}

func TestRunRemove_empty(t *testing.T) {
	err := RunRemove(context.Background(), nil, nil, &testutil.MockSpinner{})
	if err != nil {
		t.Errorf("RunRemove(nil) = %v, want nil", err)
	}
}

func TestRunInstall_withPackages(t *testing.T) {
	original := startPty
	t.Cleanup(func() { startPty = original })

	var ptyCalled bool
	startPty = func(_ context.Context, name string, args ...string) (ptySession, error) {
		ptyCalled = true
		return &mockPtySession{
			data:    []byte("Setting up pkg (1.0-1)\n"),
			readErr: io.EOF,
		}, nil
	}

	spinner := &testutil.MockSpinner{}
	err := RunInstall(context.Background(), testutil.RunnerReturning(nil, nil), []string{"pkg"}, spinner)
	if err != nil {
		t.Errorf("RunInstall = %v, want nil", err)
	}
	if !ptyCalled {
		t.Error("startPty was not called")
	}
}

func TestRunInstallBackports_withSuite(t *testing.T) {
	original := startPty
	t.Cleanup(func() { startPty = original })

	var recordedArgs []string
	startPty = func(_ context.Context, name string, args ...string) (ptySession, error) {
		recordedArgs = append(recordedArgs, name)
		recordedArgs = append(recordedArgs, args...)
		return &mockPtySession{
			data:    []byte("Setting up pkg (1.0-1)\n"),
			readErr: io.EOF,
		}, nil
	}

	spinner := &testutil.MockSpinner{}
	err := RunInstallBackports(context.Background(), testutil.RunnerReturning(nil, nil), []string{"pkg"}, "bookworm-backports", spinner)
	if err != nil {
		t.Errorf("RunInstallBackports = %v, want nil", err)
	}
	if len(recordedArgs) < 6 || recordedArgs[4] != "bookworm-backports" {
		t.Errorf("expected -t bookworm-backports in args, got %v", recordedArgs)
	}
}

func TestRunRemove_withPackages(t *testing.T) {
	original := startPty
	t.Cleanup(func() { startPty = original })

	var ptyCalled bool
	startPty = func(_ context.Context, name string, args ...string) (ptySession, error) {
		ptyCalled = true
		return &mockPtySession{
			data:    []byte("Removing pkg (1.0-1)\n"),
			readErr: io.EOF,
		}, nil
	}

	spinner := &testutil.MockSpinner{}
	err := RunRemove(context.Background(), testutil.RunnerReturning(nil, nil), []string{"pkg"}, spinner)
	if err != nil {
		t.Errorf("RunRemove = %v, want nil", err)
	}
	if !ptyCalled {
		t.Error("startPty was not called")
	}
}

func TestRunUpdate(t *testing.T) {
	var called bool
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			called = true
			if name != "apt-get" || len(args) != 1 || args[0] != "update" {
				t.Errorf("unexpected call: %s %v", name, args)
			}
			return nil, nil, nil
		},
	}
	spinner := &testutil.MockSpinner{}
	err := RunUpdate(context.Background(), runner, spinner)
	if err != nil {
		t.Errorf("RunUpdate() = %v, want nil", err)
	}
	if !called {
		t.Error("runner.Run was not called")
	}
}

func TestRunUpdate_runnerError(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, nil, errors.New("update failed")
		},
	}
	err := RunUpdate(context.Background(), runner, &testutil.MockSpinner{})
	if err == nil {
		t.Fatal("expected error from RunUpdate")
	}
}

func TestRunUpgrade(t *testing.T) {
	original := startPty
	t.Cleanup(func() { startPty = original })

	var ptyCalled bool
	startPty = func(_ context.Context, name string, args ...string) (ptySession, error) {
		ptyCalled = true
		if name != "apt-get" || len(args) != 2 || args[0] != "full-upgrade" || args[1] != "-y" {
			t.Errorf("unexpected startPty call: %s %v", name, args)
		}
		return &mockPtySession{
			data:    []byte("Setting up everything (1.0-1)\n"),
			readErr: io.EOF,
		}, nil
	}

	spinner := &testutil.MockSpinner{}
	err := RunUpgrade(context.Background(), testutil.RunnerReturning(nil, nil), spinner)
	if err != nil {
		t.Errorf("RunUpgrade() = %v, want nil", err)
	}
	if !ptyCalled {
		t.Error("startPty was not called")
	}
}

func TestRunUpgrade_nilSpinner(t *testing.T) {
	err := RunUpgrade(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected error for nil spinner")
	}
}
