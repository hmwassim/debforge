package setup

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/aptpty"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/testutil"
)

// ---- UpgradeStep tests ----------------------------------------------------

func TestUpgradeStep_Check(t *testing.T) {
	step := &UpgradeStep{}
	result := step.Check(context.Background(), &Context{UI: &testutil.MockUI{}})
	if result.Status != StatusMissing {
		t.Errorf("expected StatusMissing, got %v", result.Status)
	}
}

func TestUpgradeStep_Apply(t *testing.T) {
	defer saveAptExec()()
	aptpty.AptExec = func(_ context.Context, _ ports.CommandRunner, _ []string, _ ports.Spinner) error {
		return nil
	}

	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			if name == "apt-get" && len(args) > 0 && args[0] == "update" {
				return nil, nil, nil
			}
			return nil, nil, nil
		},
	}
	step := &UpgradeStep{}
	ui := &testutil.MockUI{}
	cx := &Context{Runner: runner, UI: ui}
	if err := step.Apply(context.Background(), cx, CheckResult{Status: StatusMissing}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
}

func TestUpgradeStep_ApplyUpdateError(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			if name == "apt-get" && len(args) > 0 && args[0] == "update" {
				return nil, nil, errors.New("update failed")
			}
			return nil, nil, nil
		},
	}
	step := &UpgradeStep{}
	ui := &testutil.MockUI{}
	cx := &Context{Runner: runner, UI: ui}
	err := step.Apply(context.Background(), cx, CheckResult{Status: StatusMissing})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "update failed") {
		t.Errorf("expected update failed, got %v", err)
	}
}

func TestUpgradeStep_ApplyUpgradeError(t *testing.T) {
	defer saveAptExec()()
	aptpty.AptExec = func(_ context.Context, _ ports.CommandRunner, args []string, _ ports.Spinner) error {
		return errors.New("upgrade failed")
	}

	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			if name == "apt-get" && len(args) > 0 && args[0] == "update" {
				return nil, nil, nil
			}
			return nil, nil, nil
		},
	}
	step := &UpgradeStep{}
	ui := &testutil.MockUI{}
	cx := &Context{Runner: runner, UI: ui}
	err := step.Apply(context.Background(), cx, CheckResult{Status: StatusMissing})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "upgrade failed") {
		t.Errorf("expected upgrade failed, got %v", err)
	}
}
