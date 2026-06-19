package commands

import (
	"context"
	"testing"

	"github.com/hmwassim/debforge/internal/ports"
)

type noopSpinner struct{}

func (s *noopSpinner) Done()          {}
func (s *noopSpinner) Fail()          {}
func (s *noopSpinner) Pause()         {}
func (s *noopSpinner) Resume()        {}
func (s *noopSpinner) SetDesc(string) {}
func (s *noopSpinner) DoneWarn()      {}

type mockUI struct{}

func (m *mockUI) Info(format string, args ...any)                        {}
func (m *mockUI) Success(format string, args ...any)                     {}
func (m *mockUI) Warn(format string, args ...any)                        {}
func (m *mockUI) Error(format string, args ...any)                       {}
func (m *mockUI) Muted(format string, args ...any)                       {}
func (m *mockUI) Debug(format string, args ...any)                       {}
func (m *mockUI) Prompt(format string, args ...any) bool                 { return true }
func (m *mockUI) PromptInput(format string, args ...any) string          { return "" }
func (m *mockUI) Spinner(ctx context.Context, desc string) ports.Spinner { return &noopSpinner{} }
func (m *mockUI) Progress(total int64, desc string) ports.Progress       { return nil }

func TestRegistryRegisterAndLookup(t *testing.T) {
	reg := NewRegistry()
	cmd := &mockCommand{name: "test"}
	reg.Register(cmd)

	found, ok := reg.Lookup("test")
	if !ok {
		t.Fatal("expected command to be found")
	}
	if found != cmd {
		t.Fatal("expected same command")
	}

	_, ok = reg.Lookup("nonexistent")
	if ok {
		t.Fatal("expected not found")
	}
}

func TestPromptVariant(t *testing.T) {
	ui := &mockUI{}
	variants := map[string]string{
		"stable": "Stable release",
		"beta":   "Beta release",
	}

	// This would need a more sophisticated mock to test properly
	// For now just verify it doesn't panic
	result := PromptVariant(ui, variants)
	if result != "" {
		t.Logf("got variant: %s", result)
	}
}

func TestPromptVariantEmpty(t *testing.T) {
	ui := &mockUI{}
	result := PromptVariant(ui, nil)
	if result != "" {
		t.Fatal("expected empty result for nil variants")
	}
}

type mockCommand struct {
	name string
}

func (m *mockCommand) Name() string                                 { return m.name }
func (m *mockCommand) Usage() string                                { return "mock command" }
func (m *mockCommand) Run(ctx context.Context, args []string) error { return nil }
