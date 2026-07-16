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
	magenta = "\033[95m"
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

var isTerminal = func(w io.Writer) bool {
	if f, ok := w.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}

// ConsoleLogger writes formatted log lines to stderr, using color and
// symbols on terminals and plain text otherwise. When a FileLogger is
// provided, every message is also written to a daily-rotated log file.
type ConsoleLogger struct {
	mu      sync.Mutex
	fileLog *FileLogger
}

// NewConsoleLogger returns a new ConsoleLogger. If fileLog is non-nil,
// messages are also written to a daily-rotated log file.
func NewConsoleLogger(fileLog *FileLogger) *ConsoleLogger {
	return &ConsoleLogger{fileLog: fileLog}
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
	if l.fileLog != nil {
		l.fileLog.log(symbolToLevel(symbol), "%s", msg)
	}
}

// Info prints a blue [i] message.
func (l *ConsoleLogger) Info(format string, args ...any) { l.line(blue, "i", format, args...) }

// Success prints a green [*] message.
func (l *ConsoleLogger) Success(format string, args ...any) { l.line(green, "*", format, args...) }

// Warn prints a yellow [!] message.
func (l *ConsoleLogger) Warn(format string, args ...any) { l.line(yellow, "!", format, args...) }

// Error prints a red [x] message.
func (l *ConsoleLogger) Error(format string, args ...any) { l.line(red, "x", format, args...) }

// Prompt asks a yes/no question on stderr and returns true for y/yes.
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
	_, _ = fmt.Scanln(&response)
	response = strings.ToLower(strings.TrimSpace(response))

	if l.fileLog != nil {
		l.fileLog.log("PROMPT", "%s -> %s", msg, response)
	}

	return response == "y" || response == "yes"
}
