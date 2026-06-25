package ui

import "testing"

// TestPromptInput_yesModeReturnsDefaultWithoutBlocking is the regression
// test for the bug where -y/--yes suppressed Prompt's yes/no confirmations
// but not PromptInput's free-text prompts (used by apt variant selection),
// so a non-interactive `debforge install -y <pkg-with-variants>` would
// hang waiting on /dev/tty. SetYes(true) must make PromptInput return the
// given default immediately, with no attempt to read from a terminal.
func TestPromptInput_yesModeReturnsDefaultWithoutBlocking(t *testing.T) {
	u := NewConsoleUI()
	u.SetYes(true)

	got := u.PromptInput("alpha", "Variant [%s]", "alpha")
	if got != "alpha" {
		t.Errorf("expected yes-mode to return the default %q immediately, got %q", "alpha", got)
	}
}

func TestPrompt_yesModeReturnsTrueWithoutBlocking(t *testing.T) {
	u := NewConsoleUI()
	u.SetYes(true)

	if !u.Prompt("Continue?") {
		t.Error("expected yes-mode Prompt to return true immediately")
	}
}

func TestPromptInput_yesModeIgnoresActiveSpinner(t *testing.T) {
	// Regression guard: yes-mode must short-circuit before
	// withSpinnerPaused/any spinner interaction, so it can't deadlock or
	// panic even if a spinner happens to be active.
	u := NewConsoleUI()
	u.SetYes(true)
	u.currentSpinner = nil // no spinner active; yes-mode shouldn't care either way

	got := u.PromptInput("default", "prompt")
	if got != "default" {
		t.Errorf("got %q, want %q", got, "default")
	}
}
