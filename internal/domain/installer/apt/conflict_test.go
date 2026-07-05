package apt

import (
	"context"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/testutil"
)

func TestCheckConflicts_removesFound(t *testing.T) {
	var gotArgs []string
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return []byte("installed"), nil, nil
		},
	}
	inst := &Installer{
		runner: runner,
		execApt: func(_ context.Context, _ ports.CommandRunner, args []string, _ ports.Spinner) error {
			gotArgs = append([]string{}, args...)
			return nil
		},
	}
	p := &pkg.Package{Name: "test-pkg", Apt: &pkg.AptConfig{Conflicts: []string{"conflict-pkg"}}}

	if err := inst.checkConflicts(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("checkConflicts: %v", err)
	}
	if len(gotArgs) == 0 {
		t.Fatal("expected execApt to be called")
	}
	if gotArgs[0] != "remove" || gotArgs[1] != "-y" || gotArgs[2] != "conflict-pkg" {
		t.Errorf("unexpected args: %v", gotArgs)
	}
}

func TestCheckConflicts_none(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, nil, nil
		},
	}
	inst := &Installer{
		runner: runner,
		execApt: func(_ context.Context, _ ports.CommandRunner, _ []string, _ ports.Spinner) error {
			t.Fatal("execApt should not be called")
			return nil
		},
	}
	p := &pkg.Package{Name: "test-pkg", Apt: &pkg.AptConfig{}}

	if err := inst.checkConflicts(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("checkConflicts: %v", err)
	}
}
