package deb

// Install and Remove are not unit-tested here: both ultimately call into
// internal/aptpty, which deliberately constructs *exec.Cmd directly
// (driving a real pty) instead of going through ports.CommandRunner - see
// aptpty.go's own comment on that tradeoff. Without a real apt-get/dpkg
// to drive, there's nothing meaningful to assert beyond "the mock wasn't
// called", which isn't a useful regression test. checkVersion is the one
// piece of deb-specific logic that doesn't touch aptpty at all, so it's
// what's covered here.

import (
	"context"
	"errors"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/testutil"
)

func TestCheckVersion_firstInstall(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return []byte("abc\trefs/tags/v1.0.0\n"), nil, nil
		},
	}
	inst := &Installer{runner: runner}
	p := &pkg.Package{Name: "test-deb", Repo: "https://github.com/o/p.git"}

	updated, err := inst.checkVersion(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("checkVersion: %v", err)
	}
	if !updated {
		t.Error("expected updated=true on first install (no prior recorded version)")
	}
	if p.Version != "1.0.0" {
		t.Errorf("expected p.Version=1.0.0, got %q", p.Version)
	}
}

func TestCheckVersion_unchangedNotUpdated(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return []byte("abc\trefs/tags/v1.0.0\n"), nil, nil
		},
	}
	inst := &Installer{runner: runner}
	p := &pkg.Package{Name: "test-deb", Repo: "https://github.com/o/p.git", Version: "1.0.0"}

	updated, err := inst.checkVersion(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("checkVersion: %v", err)
	}
	if updated {
		t.Error("expected updated=false when the latest tag matches the recorded version")
	}
}

func TestCheckVersion_newerTagIsUpdated(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return []byte("abc\trefs/tags/v2.0.0\n"), nil, nil
		},
	}
	inst := &Installer{runner: runner}
	p := &pkg.Package{Name: "test-deb", Repo: "https://github.com/o/p.git", Version: "1.0.0"}

	updated, err := inst.checkVersion(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("checkVersion: %v", err)
	}
	if !updated {
		t.Error("expected updated=true when a newer tag is available")
	}
	if p.Version != "2.0.0" {
		t.Errorf("expected p.Version updated to 2.0.0, got %q", p.Version)
	}
}

func TestCheckVersion_versionCmd(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return []byte("9.9.9\n"), nil, nil
		},
	}
	inst := &Installer{runner: runner}
	p := &pkg.Package{Name: "test-deb", VersionCmd: "echo 9.9.9"}

	updated, err := inst.checkVersion(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("checkVersion: %v", err)
	}
	if !updated || p.Version != "9.9.9" {
		t.Errorf("expected updated=true and p.Version=9.9.9, got updated=%v p.Version=%q", updated, p.Version)
	}
}

func TestCheckVersion_runnerErrorPropagates(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, nil, errors.New("boom")
		},
	}
	inst := &Installer{runner: runner}
	p := &pkg.Package{Name: "test-deb", VersionCmd: "false"}

	if _, err := inst.checkVersion(context.Background(), p, &testutil.MockSpinner{}); err == nil {
		t.Error("expected an error to propagate from a failing version command")
	}
}
