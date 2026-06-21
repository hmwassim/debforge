package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"golang.org/x/term"
)

var (
	reset   = "\033[0m"
	bold    = "\033[1m"
	red     = "\033[31m"
	green   = "\033[32m"
	yellow  = "\033[33m"
	blue    = "\033[34m"
	magenta = "\033[35m"
)

type consoleOutput struct {
	mu sync.Mutex
}

var defaultConsole = &consoleOutput{}

func (c *consoleOutput) writef(w io.Writer, format string, args ...any) {
	c.mu.Lock()
	fmt.Fprintf(w, format, args...)
	c.mu.Unlock()
}

func isTerminal(w io.Writer) bool {
	if f, ok := w.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}

type ConsoleLogger struct {
	mu sync.Mutex
}

func NewConsoleLogger() *ConsoleLogger {
	return &ConsoleLogger{}
}

func (l *ConsoleLogger) line(color, symbol, format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	msg := fmt.Sprintf(format, args...)
	w := os.Stderr
	if isTerminal(w) {
		fmt.Fprintf(w, "%s[%s]%s %s\n", bold+color, symbol, reset, msg)
	} else {
		fmt.Fprintf(w, "[%s] %s\n", symbol, msg)
	}
}

func (l *ConsoleLogger) Info(format string, args ...any)    { l.line(blue, "i", format, args...) }
func (l *ConsoleLogger) Success(format string, args ...any) { l.line(green, "*", format, args...) }
func (l *ConsoleLogger) Warn(format string, args ...any)    { l.line(yellow, "!", format, args...) }
func (l *ConsoleLogger) Error(format string, args ...any)   { l.line(red, "x", format, args...) }

func (l *ConsoleLogger) Prompt(format string, args ...any) bool {
	l.mu.Lock()
	msg := fmt.Sprintf(format, args...)
	w := os.Stderr
	if isTerminal(w) {
		fmt.Fprintf(w, "%s[?]%s %s [y/N] ", bold+yellow, reset, msg)
	} else {
		fmt.Fprintf(w, "[?] %s [y/N] ", msg)
	}
	l.mu.Unlock()

	var response string
	fmt.Scanln(&response)
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}
