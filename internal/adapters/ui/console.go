package ui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/hmwassim/debforge/internal/ports"
)

// consoleOutput serializes writes from multiple goroutines (e.g. spinner + main).
type consoleOutput struct {
	mu sync.Mutex
}

var defaultConsole = &consoleOutput{}

func (c *consoleOutput) writef(w io.Writer, format string, args ...any) {
	c.mu.Lock()
	fmt.Fprintf(w, format, args...)
	c.mu.Unlock()
}

type ConsoleUI struct {
	logger ports.Logger
}

func NewConsoleUI() *ConsoleUI {
	return &ConsoleUI{logger: NewConsoleLogger()}
}

func (u *ConsoleUI) Info(format string, args ...any)    { u.logger.Info(format, args...) }
func (u *ConsoleUI) Success(format string, args ...any) { u.logger.Success(format, args...) }
func (u *ConsoleUI) Warn(format string, args ...any)    { u.logger.Warn(format, args...) }
func (u *ConsoleUI) Error(format string, args ...any)   { u.logger.Error(format, args...) }
func (u *ConsoleUI) Muted(format string, args ...any)   { u.logger.Muted(format, args...) }
func (u *ConsoleUI) Debug(format string, args ...any)   { u.logger.Debug(format, args...) }
func (u *ConsoleUI) Prompt(format string, args ...any) bool {
	return u.logger.Prompt(format, args...)
}

func (u *ConsoleUI) PromptInput(format string, args ...any) string {
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

func (u *ConsoleUI) Spinner(ctx context.Context, description string) ports.Spinner {
	return NewConsoleSpinner(ctx, os.Stderr, description)
}

func (u *ConsoleUI) Progress(total int64, description string) ports.Progress {
	return NewConsoleProgress(os.Stderr, total, description)
}
