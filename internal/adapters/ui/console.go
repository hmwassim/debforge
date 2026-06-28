// Package ui provides terminal UI primitives including logging, interactive
// prompts, and an animated spinner display.
package ui

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/hmwassim/debforge/internal/ports"
)

// ConsoleUI implements ports.UI with terminal output and interactive prompts.
type ConsoleUI struct {
	logger         *ConsoleLogger
	currentSpinner *Display
	yes            bool
}

// NewConsoleUI returns a ConsoleUI that writes to stderr.
func NewConsoleUI() *ConsoleUI {
	return &ConsoleUI{logger: NewConsoleLogger()}
}

// SetYes sets whether yes/no prompts automatically return the default value.
func (u *ConsoleUI) SetYes(yes bool) { u.yes = yes }

// Prompt asks a yes/no question formatted with format and args,
// returning true when the user answers yes.
func (u *ConsoleUI) Prompt(format string, args ...any) bool {
	if u.yes {
		return true
	}
	var result bool
	u.withSpinnerPaused(func() {
		result = u.logger.Prompt(format, args...)
	})
	return result
}

// Info prints an informational message.
func (u *ConsoleUI) Info(format string, args ...any) {
	u.withSpinnerPaused(func() { u.logger.Info(format, args...) })
}

// Success prints a success message.
func (u *ConsoleUI) Success(format string, args ...any) {
	u.withSpinnerPaused(func() { u.logger.Success(format, args...) })
}

// Warn prints a warning message.
func (u *ConsoleUI) Warn(format string, args ...any) {
	u.withSpinnerPaused(func() { u.logger.Warn(format, args...) })
}

// Error prints an error message.
func (u *ConsoleUI) Error(format string, args ...any) {
	u.withSpinnerPaused(func() { u.logger.Error(format, args...) })
}

// PromptInput asks for text input with a formatted prompt, returning the
// user's response. When yes mode is set, it returns defaultVal without
// prompting.
func (u *ConsoleUI) PromptInput(defaultVal, format string, args ...any) string {
	if u.yes {
		return defaultVal
	}
	var result string
	u.withSpinnerPaused(func() {
		msg := fmt.Sprintf(format, args...)
		w := os.Stderr
		if isTerminal(w) {
			defaultConsole.writef(w, "%s[?]%s %s ", bold+yellow, reset, msg)
		} else {
			defaultConsole.writef(w, "[?] %s ", msg)
		}
		tty, err := os.Open("/dev/tty")
		if err != nil {
			var s string
			fmt.Scanln(&s)
			result = strings.TrimSpace(s)
			return
		}
		defer tty.Close()
		reader := bufio.NewReader(tty)
		line, _ := reader.ReadString('\n')
		result = strings.TrimSpace(line)
	})
	return result
}

// withSpinnerPaused pauses the active spinner (if any) for the duration of
// fn, then resumes it. Prompt and PromptInput both need to silence the
// spinner while waiting on user input; this is that one shared sequence
// instead of two inlined copies of the same Pause/defer Resume block.
func (u *ConsoleUI) withSpinnerPaused(fn func()) {
	if u.currentSpinner == nil {
		fn()
		return
	}
	u.currentSpinner.Pause()
	defer u.currentSpinner.Resume()
	fn()
}

// Spinner creates a new animated spinner with the given description.
func (u *ConsoleUI) Spinner(ctx context.Context, desc string) ports.Spinner {
	s := NewDisplay(ctx, os.Stderr, desc)
	u.currentSpinner = s
	return s
}

var _ ports.UI = (*ConsoleUI)(nil)
