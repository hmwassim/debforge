package text

import (
	"fmt"
	"io"
	"os"
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

type Logger struct {
	debug bool
}

func New() *Logger {
	return &Logger{debug: os.Getenv("DEBFORGE_DEBUG") == "1"}
}

func (l *Logger) Info(format string, args ...interface{}) {
	l.print(os.Stderr, blue, "i", format, args...)
}

func (l *Logger) Success(format string, args ...interface{}) {
	l.print(os.Stderr, green, "*", format, args...)
}

func (l *Logger) Warn(format string, args ...interface{}) {
	l.print(os.Stderr, yellow, "!", format, args...)
}

func (l *Logger) Error(format string, args ...interface{}) {
	l.print(os.Stderr, red, "x", format, args...)
}

func (l *Logger) Debug(format string, args ...interface{}) {
	if !l.debug {
		return
	}
	l.print(os.Stderr, gray, "-", format, args...)
}

func (l *Logger) print(w io.Writer, color, symbol, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if useColor(w) {
		fmt.Fprintf(w, "%s[%s]%s %s\n", bold+color, symbol, reset, msg)
	} else {
		fmt.Fprintf(w, "[%s] %s\n", symbol, msg)
	}
}

func (l *Logger) Prompt(msg string) bool {
	if useColor(os.Stderr) {
		fmt.Fprintf(os.Stderr, "%s[?]%s %s [y/N] ", bold+yellow, reset, msg)
	} else {
		fmt.Fprintf(os.Stderr, "[?] %s [y/N] ", msg)
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

func IsTerminal(w io.Writer) bool {
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

func useColor(w io.Writer) bool {
	return IsTerminal(w)
}
