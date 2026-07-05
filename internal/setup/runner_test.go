package setup

import (
	"context"
	"errors"
	"testing"

	"github.com/hmwassim/debforge/internal/testutil"
)

// ---- Runner tests ----------------------------------------------------------

func TestRunner_Satisfied(t *testing.T) {
	step := &mockStep{name: "test", checkResult: CheckResult{Status: StatusSatisfied}}
	runner := NewRunner(step)
	err := runner.Run(context.Background(), &Context{
		UI: &testutil.MockUI{},
	})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if step.applyCalled {
		t.Error("Apply should not be called for satisfied step")
	}
}

func TestRunner_Missing(t *testing.T) {
	step := &mockStep{name: "test", checkResult: CheckResult{Status: StatusMissing, Summary: "not found"}}
	runner := NewRunner(step)
	err := runner.Run(context.Background(), &Context{
		UI: &testutil.MockUI{},
	})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if !step.applyCalled {
		t.Error("Apply should be called for missing step")
	}
}

func TestRunner_Drifted(t *testing.T) {
	step := &mockStep{name: "test", checkResult: CheckResult{Status: StatusDrifted, Summary: "modified"}}
	runner := NewRunner(step)
	err := runner.Run(context.Background(), &Context{
		UI: &testutil.MockUI{},
	})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if !step.applyCalled {
		t.Error("Apply should be called for drifted step")
	}
}

func TestRunner_Conflict(t *testing.T) {
	step := &mockStep{name: "test", checkResult: CheckResult{Status: StatusConflict, Summary: "conflict"}}
	runner := NewRunner(step)
	err := runner.Run(context.Background(), &Context{
		UI: &testutil.MockUI{},
	})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if !step.applyCalled {
		t.Error("Apply should be called for conflict step")
	}
}

func TestRunner_Error(t *testing.T) {
	step := &mockStep{name: "test", checkResult: CheckResult{Status: StatusError, Summary: "check failed"}}
	runner := NewRunner(step)
	err := runner.Run(context.Background(), &Context{
		UI: &testutil.MockUI{},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if step.applyCalled {
		t.Error("Apply should not be called for error step")
	}
}

func TestRunner_ApplyError(t *testing.T) {
	step := &mockStep{
		name:        "test",
		checkResult: CheckResult{Status: StatusMissing, Summary: "not found"},
		applyErr:    errors.New("apply failed"),
	}
	runner := NewRunner(step)
	err := runner.Run(context.Background(), &Context{
		UI: &testutil.MockUI{},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRunner_ForceSkipsCheck(t *testing.T) {
	step := &mockStep{name: "test", checkResult: CheckResult{Status: StatusSatisfied}}
	runner := NewRunner(step)
	err := runner.Run(context.Background(), &Context{
		UI:    &testutil.MockUI{},
		Force: true,
	})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if !step.applyCalled {
		t.Error("Apply should be called in force mode even for satisfied step")
	}
}

func TestRunner_StopsOnError(t *testing.T) {
	step1 := &mockStep{name: "first", checkResult: CheckResult{Status: StatusSatisfied}}
	step2 := &mockStep{name: "second", checkResult: CheckResult{Status: StatusError, Summary: "boom"}}
	step3 := &mockStep{name: "third", checkResult: CheckResult{Status: StatusSatisfied}}
	runner := NewRunner(step1, step2, step3)
	err := runner.Run(context.Background(), &Context{
		UI: &testutil.MockUI{},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if step3.applyCalled {
		t.Error("runner should stop after step2 error")
	}
}

func TestCheckAll(t *testing.T) {
	step1 := &mockStep{name: "a", checkResult: CheckResult{Status: StatusSatisfied}}
	step2 := &mockStep{name: "b", checkResult: CheckResult{Status: StatusMissing}}
	runner := NewRunner(step1, step2)
	results := runner.CheckAll(context.Background(), &Context{
		UI: &testutil.MockUI{},
	})
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Status != StatusSatisfied {
		t.Errorf("expected satisfied, got %v", results[0].Status)
	}
	if results[1].Status != StatusMissing {
		t.Errorf("expected missing, got %v", results[1].Status)
	}
}
