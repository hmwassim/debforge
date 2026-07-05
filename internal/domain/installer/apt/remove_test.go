package apt

import (
	"context"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/testutil"
)

func TestRemove_callsExecApt(t *testing.T) {
	var gotArgs []string
	inst := &Installer{
		execApt: func(_ context.Context, _ ports.CommandRunner, args []string, _ ports.Spinner) error {
			gotArgs = append([]string{}, args...)
			return nil
		},
	}
	p := &pkg.Package{Name: "test-pkg", Type: pkg.TypeApt, Packages: []string{"pkg-a"}, Apt: &pkg.AptConfig{}}

	if err := inst.Remove(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	want := []string{"remove", "-y", "pkg-a"}
	if len(gotArgs) != len(want) {
		t.Fatalf("got %v, want %v", gotArgs, want)
	}
	for i := range want {
		if gotArgs[i] != want[i] {
			t.Errorf("arg %d: got %q, want %q", i, gotArgs[i], want[i])
		}
	}
}

func TestRemove_emptySkips(t *testing.T) {
	inst := &Installer{
		execApt: func(_ context.Context, _ ports.CommandRunner, _ []string, _ ports.Spinner) error {
			t.Fatal("execApt should not be called with no packages")
			return nil
		},
	}
	p := &pkg.Package{Name: "test-pkg", Type: pkg.TypeApt, Apt: &pkg.AptConfig{}}

	if err := inst.Remove(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
}

func TestRemove_usesRemoveField(t *testing.T) {
	var gotArgs []string
	inst := &Installer{
		execApt: func(_ context.Context, _ ports.CommandRunner, args []string, _ ports.Spinner) error {
			gotArgs = append([]string{}, args...)
			return nil
		},
	}
	p := &pkg.Package{
		Name:     "test-pkg",
		Type:     pkg.TypeApt,
		Packages: []string{"pkg-a"},
		Remove:   []string{"rm-pkg"},
		Apt:      &pkg.AptConfig{},
	}

	if err := inst.Remove(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	want := []string{"remove", "-y", "rm-pkg"}
	if len(gotArgs) != len(want) {
		t.Fatalf("got %v, want %v", gotArgs, want)
	}
	for i := range want {
		if gotArgs[i] != want[i] {
			t.Errorf("arg %d: got %q, want %q", i, gotArgs[i], want[i])
		}
	}
}

func TestRemove_usesVariantPackages(t *testing.T) {
	var gotArgs []string
	inst := &Installer{
		execApt: func(_ context.Context, _ ports.CommandRunner, args []string, _ ports.Spinner) error {
			gotArgs = append([]string{}, args...)
			return nil
		},
	}
	p := &pkg.Package{
		Name:     "test-pkg",
		Type:     pkg.TypeApt,
		Packages: []string{"base-pkg"},
		Apt:      &pkg.AptConfig{Variant: "pro", Variants: map[string][]string{"pro": {"pro-pkg"}}},
	}

	if err := inst.Remove(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	want := []string{"remove", "-y", "base-pkg", "pro-pkg"}
	if len(gotArgs) != len(want) {
		t.Fatalf("got %v, want %v", gotArgs, want)
	}
	for i := range want {
		if gotArgs[i] != want[i] {
			t.Errorf("arg %d: got %q, want %q", i, gotArgs[i], want[i])
		}
	}
}

func TestRemove_wrongType(t *testing.T) {
	inst := &Installer{}
	p := &pkg.Package{Name: "test-pkg", Type: pkg.TypeDeb}

	if err := inst.Remove(context.Background(), p, &testutil.MockSpinner{}); err == nil {
		t.Fatal("expected error for wrong type")
	}
}
