package testutil

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/ports"
)

// MockSpinner is a no-op ports.Spinner that records its last description.
type MockSpinner struct{ Desc string }

func (m *MockSpinner) Done()            {}
func (m *MockSpinner) Fail()            {}
func (m *MockSpinner) DoneWarn()        {}
func (m *MockSpinner) DoneInfo()        {}
func (m *MockSpinner) Pause()           {}
func (m *MockSpinner) Resume()          {}
func (m *MockSpinner) SetDesc(d string) { m.Desc = d }

// MockUI is a minimal ports.UI for tests. PromptInputFunc/PromptFunc let a
// test control interactive answers; when Yes is true, PromptInput mirrors
// the real ConsoleUI's yes-mode behavior (returns defaultVal immediately)
// unless PromptInputFunc is explicitly set.
type MockUI struct {
	Yes bool

	Infos    []string
	Warnings []string
	Errors   []string

	PromptFunc      func(format string, args ...any) bool
	PromptInputFunc func(defaultVal, format string, args ...any) string
}

func (m *MockUI) Info(format string, args ...any) {
	m.Infos = append(m.Infos, fmt.Sprintf(format, args...))
}
func (m *MockUI) Success(format string, args ...any) {}
func (m *MockUI) Warn(format string, args ...any) {
	m.Warnings = append(m.Warnings, fmt.Sprintf(format, args...))
}
func (m *MockUI) Error(format string, args ...any) {
	m.Errors = append(m.Errors, fmt.Sprintf(format, args...))
}

func (m *MockUI) Prompt(format string, args ...any) bool {
	if m.PromptFunc != nil {
		return m.PromptFunc(format, args...)
	}
	return true
}

func (m *MockUI) PromptInput(defaultVal, format string, args ...any) string {
	if m.Yes {
		return defaultVal
	}
	if m.PromptInputFunc != nil {
		return m.PromptInputFunc(defaultVal, format, args...)
	}
	return defaultVal
}

func (m *MockUI) Spinner(_ context.Context, _ string) ports.Spinner { return &MockSpinner{} }
func (m *MockUI) SetYes(yes bool)                                   { m.Yes = yes }

var _ ports.UI = (*MockUI)(nil)
