package ui

import (
	"fmt"
	"io"
	"os"
	"unicode"
	"unicode/utf8"

	"github.com/hmwassim/debforge/internal/ports"
)

const (
	reset  = "\033[0m"
	bold   = "\033[1m"
	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
	blue   = "\033[34m"
	gray   = "\033[90m"
)

type ConsoleLogger struct {
	debug bool
	color bool
}

func NewConsoleLogger() *ConsoleLogger {
	return &ConsoleLogger{
		debug: os.Getenv("DEBFORGE_DEBUG") == "1",
		color: useColor(os.Stderr),
	}
}

func (l *ConsoleLogger) Info(format string, args ...any) {
	l.print(blue, "i", format, args...)
}

func (l *ConsoleLogger) Success(format string, args ...any) {
	l.print(green, "*", format, args...)
}

func (l *ConsoleLogger) Warn(format string, args ...any) {
	l.print(yellow, "!", format, args...)
}

func (l *ConsoleLogger) Error(format string, args ...any) {
	l.print(red, "x", format, args...)
}

func (l *ConsoleLogger) Muted(format string, args ...any) {
	l.print(gray, "-", format, args...)
}

func (l *ConsoleLogger) Debug(format string, args ...any) {
	if !l.debug {
		return
	}
	l.print(gray, "-", format, args...)
}

func (l *ConsoleLogger) Prompt(format string, args ...any) bool {
	msg := fmt.Sprintf(format, args...)
	if l.color {
		defaultConsole.writef(os.Stderr, "%s[?]%s %s [y/N] ", bold+yellow, reset, msg)
	} else {
		defaultConsole.writef(os.Stderr, "[?] %s [y/N] ", msg)
	}
	var resp string
	tty, err := os.Open("/dev/tty")
	if err != nil {
		fmt.Scanln(&resp)
	} else {
		defer tty.Close()
		fmt.Fscanln(tty, &resp)
	}
	return resp == "y" || resp == "Y" || resp == "yes" || resp == "YES" || resp == "Yes"
}

func (l *ConsoleLogger) print(color, symbol, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if len(msg) > 0 {
		r, size := utf8.DecodeRuneInString(msg)
		msg = string(unicode.ToUpper(r)) + msg[size:]
	}
	if l.color {
		defaultConsole.writef(os.Stderr, "%s[%s]%s %s\n", bold+color, symbol, reset, msg)
	} else {
		defaultConsole.writef(os.Stderr, "[%s] %s\n", symbol, msg)
	}
}

func useColor(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

var _ ports.Logger = (*ConsoleLogger)(nil)
