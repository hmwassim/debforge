package ui

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/hmwassim/debforge/internal/ports"
)

type ConsoleUI struct {
	logger         *ConsoleLogger
	currentSpinner *Display
	yes            bool
}

func NewConsoleUI() *ConsoleUI {
	return &ConsoleUI{logger: NewConsoleLogger()}
}

func (u *ConsoleUI) SetYes(yes bool) { u.yes = yes }

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

func (u *ConsoleUI) Info(format string, args ...any)    { u.logger.Info(format, args...) }
func (u *ConsoleUI) Success(format string, args ...any) { u.logger.Success(format, args...) }
func (u *ConsoleUI) Warn(format string, args ...any)    { u.logger.Warn(format, args...) }
func (u *ConsoleUI) Error(format string, args ...any)   { u.logger.Error(format, args...) }

func (u *ConsoleUI) PromptInput(format string, args ...any) string {
	var result string
	u.withSpinnerPaused(func() {
		msg := fmt.Sprintf(format, args...)
		defaultConsole.writef(os.Stderr, "[?] %s ", msg)
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

func (u *ConsoleUI) Spinner(ctx context.Context, desc string) ports.Spinner {
	s := NewDisplay(ctx, os.Stderr, desc)
	u.currentSpinner = s
	return s
}

var _ ports.UI = (*ConsoleUI)(nil)
