package text

import (
	"fmt"
	"os"
)

const (
	Reset  = "\033[0m"
	Bold   = "\033[1m"
	Red    = "\033[31m"
	Green  = "\033[32m"
	Yellow = "\033[33m"
	Blue   = "\033[34m"
	Gray   = "\033[90m"
)

type Logger struct{}

func New() *Logger {
	return &Logger{}
}

func (l *Logger) Info(format string, args ...interface{}) {
	l.print(Blue, "i", format, args...)
}

func (l *Logger) Success(format string, args ...interface{}) {
	l.print(Green, "*", format, args...)
}

func (l *Logger) Warn(format string, args ...interface{}) {
	l.print(Yellow, "!", format, args...)
}

func (l *Logger) Error(format string, args ...interface{}) {
	l.print(Red, "x", format, args...)
}

func (l *Logger) Debug(format string, args ...interface{}) {
	l.print(Gray, "-", format, args...)
}

func (l *Logger) print(color, symbol, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "%s[%s]%s %s\n", Bold+color, symbol, Reset, msg)
}

func Prompt(msg string) bool {
	fmt.Fprintf(os.Stderr, "%s[?]%s %s [y/N] ", Bold+Yellow, Reset, msg)
	var resp string
	tty, err := os.Open("/dev/tty")
	if err != nil {
		fmt.Scanln(&resp)
	} else {
		defer tty.Close()
		fmt.Fscanln(tty, &resp)
	}
	return resp == "y" || resp == "Y"
}
