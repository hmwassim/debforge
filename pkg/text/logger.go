package text

import (
	"fmt"
	"io"
	"os"
)

type Logger struct {
	debug bool
	color bool
}

func New() *Logger {
	return &Logger{
		debug: os.Getenv("DEBFORGE_DEBUG") == "1",
		color: useColor(os.Stderr),
	}
}

func (l *Logger) Info(format string, args ...interface{}) {
	l.print(blue, "i", format, args...)
}

func (l *Logger) Success(format string, args ...interface{}) {
	l.print(green, "*", format, args...)
}

func (l *Logger) Warn(format string, args ...interface{}) {
	l.print(yellow, "!", format, args...)
}

func (l *Logger) Error(format string, args ...interface{}) {
	l.print(red, "x", format, args...)
}

func (l *Logger) Debug(format string, args ...interface{}) {
	if !l.debug {
		return
	}
	l.print(gray, "-", format, args...)
}

func (l *Logger) print(color, symbol, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if l.color {
		ConsoleWritef(os.Stderr, "%s[%s]%s %s\n", bold+color, symbol, reset, msg)
	} else {
		ConsoleWritef(os.Stderr, "[%s] %s\n", symbol, msg)
	}
}

func (l *Logger) Prompt(msg string) bool {
	if l.color {
		ConsoleWritef(os.Stderr, "%s[?]%s %s [y/N] ", bold+yellow, reset, msg)
	} else {
		ConsoleWritef(os.Stderr, "[?] %s [y/N] ", msg)
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
