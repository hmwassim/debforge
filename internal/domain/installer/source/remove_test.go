package source

import (
	"context"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/testutil"
)

func TestRemove_removeScript(t *testing.T) {
	runner := &recordingRunner{}
	inst := &Installer{runner: runner, fs: testutil.NewMockFileSystem()}
	p := &pkg.Package{
		Name: "test-src",
		Type: pkg.TypeSource,
		Source: &pkg.SourceConfig{
			RemoveScript: "echo removing",
		},
	}

	if err := inst.Remove(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(runner.scripts) != 1 || runner.scripts[0] != "echo removing" {
		t.Errorf("expected remove script to run, got %v", runner.scripts)
	}
}

func TestRemove_aptGetRemove(t *testing.T) {
	var gotArgs []string
	inst := &Installer{
		fs: testutil.NewMockFileSystem(),
		execApt: func(_ context.Context, _ ports.CommandRunner, args []string, _ ports.Spinner) error {
			gotArgs = append([]string{}, args...)
			return nil
		},
	}
	p := &pkg.Package{
		Name:   "test-src",
		Type:   pkg.TypeSource,
		Remove: []string{"pkg-a", "pkg-b"},
		Source: &pkg.SourceConfig{},
	}

	if err := inst.Remove(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	want := []string{"remove", "-y", "pkg-a", "pkg-b"}
	if len(gotArgs) != len(want) {
		t.Fatalf("got %v, want %v", gotArgs, want)
	}
	for i := range want {
		if gotArgs[i] != want[i] {
			t.Errorf("arg %d: got %q, want %q", i, gotArgs[i], want[i])
		}
	}
}

func TestRemove_bothScriptAndApt(t *testing.T) {
	runner := &recordingRunner{}
	var gotArgs []string
	inst := &Installer{
		runner: runner,
		fs:     testutil.NewMockFileSystem(),
		execApt: func(_ context.Context, _ ports.CommandRunner, args []string, _ ports.Spinner) error {
			gotArgs = append([]string{}, args...)
			return nil
		},
	}
	p := &pkg.Package{
		Name:   "test-src",
		Type:   pkg.TypeSource,
		Remove: []string{"pkg-a"},
		Source: &pkg.SourceConfig{
			RemoveScript: "echo removing",
		},
	}

	if err := inst.Remove(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(runner.scripts) != 1 || runner.scripts[0] != "echo removing" {
		t.Errorf("expected remove script to run, got %v", runner.scripts)
	}
	want := []string{"remove", "-y", "pkg-a"}
	if len(gotArgs) != len(want) {
		t.Fatalf("got %v, want %v", gotArgs, want)
	}
}

func TestRemove_wrongType(t *testing.T) {
	inst := &Installer{}
	p := &pkg.Package{Name: "test", Type: pkg.TypeDeb}
	if err := inst.Remove(context.Background(), p, &testutil.MockSpinner{}); err == nil {
		t.Fatal("expected error for wrong type")
	}
}

func TestRemove_noScriptNoPackages(t *testing.T) {
	inst := &Installer{fs: testutil.NewMockFileSystem()}
	p := &pkg.Package{Name: "test-src", Type: pkg.TypeSource, Source: &pkg.SourceConfig{}}

	if err := inst.Remove(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
}
