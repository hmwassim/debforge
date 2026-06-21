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
	if u.currentSpinner != nil {
		u.currentSpinner.Pause()
		defer u.currentSpinner.Resume()
	}
	return u.logger.Prompt(format, args...)
}

func (u *ConsoleUI) Info(format string, args ...any)    { u.logger.Info(format, args...) }
func (u *ConsoleUI) Success(format string, args ...any) { u.logger.Success(format, args...) }
func (u *ConsoleUI) Warn(format string, args ...any)    { u.logger.Warn(format, args...) }
func (u *ConsoleUI) Error(format string, args ...any)   { u.logger.Error(format, args...) }

func (u *ConsoleUI) PromptInput(format string, args ...any) string {
	if u.currentSpinner != nil {
		u.currentSpinner.Pause()
		defer u.currentSpinner.Resume()
	}
	msg := fmt.Sprintf(format, args...)
	defaultConsole.writef(os.Stderr, "[?] %s ", msg)
	tty, err := os.Open("/dev/tty")
	if err != nil {
		var s string
		fmt.Scanln(&s)
		return strings.TrimSpace(s)
	}
	defer tty.Close()
	reader := bufio.NewReader(tty)
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

func (u *ConsoleUI) Spinner(ctx context.Context, desc string) ports.Spinner {
	s := NewDisplay(ctx, os.Stderr, desc)
	u.currentSpinner = s
	return s
}

var _ ports.UI = (*ConsoleUI)(nil)
