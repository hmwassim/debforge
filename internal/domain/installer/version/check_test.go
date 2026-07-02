package version

import (
	"context"
	"errors"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/testutil"
)

func TestGatherVersion_versionCmd(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return []byte("1.2.3\n"), nil, nil
		},
	}
	p := &pkg.Package{Name: "test-pkg", VersionCmd: "echo 1.2.3"}
	got, err := GatherVersion(context.Background(), runner, p)
	if err != nil {
		t.Fatalf("GatherVersion: %v", err)
	}
	if got != "1.2.3" {
		t.Errorf("got %q, want %q", got, "1.2.3")
	}
}

func TestGatherVersion_versionCmdError(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, nil, errors.New("command failed")
		},
	}
	p := &pkg.Package{Name: "test-pkg", VersionCmd: "fail"}
	_, err := GatherVersion(context.Background(), runner, p)
	if err == nil {
		t.Fatal("expected error from failed VersionCmd")
	}
}

func TestGatherVersion_repoLookup(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return []byte("abc\trefs/tags/v2.0.0\n"), nil, nil
		},
	}
	p := &pkg.Package{
		Name: "test-pkg",
		Repo: "https://github.com/o/p.git",
	}
	got, err := GatherVersion(context.Background(), runner, p)
	if err != nil {
		t.Fatalf("GatherVersion: %v", err)
	}
	if got != "2.0.0" {
		t.Errorf("got %q, want %q", got, "2.0.0")
	}
}

func TestGatherVersion_neither(t *testing.T) {
	p := &pkg.Package{Name: "test-pkg"}
	got, err := GatherVersion(context.Background(), nil, p)
	if err != nil {
		t.Fatalf("GatherVersion: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestApplyVersionUpdate_empty(t *testing.T) {
	spinner := &testutil.MockSpinner{}
	p := &pkg.Package{Name: "test", Version: "1.0"}
	_, err := ApplyVersionUpdate(spinner, p, "")
	if err == nil {
		t.Fatal("expected error for empty latest")
	}
}

func TestApplyVersionUpdate_alreadyUpToDate(t *testing.T) {
	spinner := &testutil.MockSpinner{}
	p := &pkg.Package{Name: "test", Version: "1.0"}
	updated, err := ApplyVersionUpdate(spinner, p, "1.0")
	if err != nil {
		t.Fatalf("ApplyVersionUpdate: %v", err)
	}
	if updated {
		t.Error("expected updated=false when versions match")
	}
	if p.Version != "1.0" {
		t.Errorf("Version changed to %q", p.Version)
	}
}

func TestApplyVersionUpdate_newer(t *testing.T) {
	spinner := &testutil.MockSpinner{}
	p := &pkg.Package{Name: "test", Version: "1.0"}
	updated, err := ApplyVersionUpdate(spinner, p, "2.0")
	if err != nil {
		t.Fatalf("ApplyVersionUpdate: %v", err)
	}
	if !updated {
		t.Error("expected updated=true when versions differ")
	}
	if p.Version != "2.0" {
		t.Errorf("Version = %q, want %q", p.Version, "2.0")
	}
}
