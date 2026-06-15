package executil

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

func Run(cmd *exec.Cmd) error {
	if cmd.Stdout == nil {
		cmd.Stdout = io.Discard
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if s := strings.TrimSpace(stderr.String()); s != "" {
			return fmt.Errorf("%s: %w", s, err)
		}
		return err
	}
	return nil
}

func RunWithSpinner(cmd *exec.Cmd, desc string) error {
	w := os.Stderr
	msg := "[i] " + desc

	if cmd.Stdout == nil {
		cmd.Stdout = io.Discard
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return err
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	if isTerminal(w) {
		fmt.Fprintf(w, "%s [ ]", msg)
		chars := []string{"|", "/", "-", "\\"}
		i := 0
		for {
			select {
			case err := <-done:
				fmt.Fprintf(w, "\r%s\n", msg)
				if err != nil {
					if s := strings.TrimSpace(stderr.String()); s != "" {
						return fmt.Errorf("%s: %w", s, err)
					}
					return err
				}
				return nil
			default:
				fmt.Fprintf(w, "\r%s [%s]", msg, chars[i%len(chars)])
				i++
				time.Sleep(100 * time.Millisecond)
			}
		}
	}

	err := <-done
	if err != nil {
		if s := strings.TrimSpace(stderr.String()); s != "" {
			return fmt.Errorf("%s: %w", s, err)
		}
		return err
	}
	return nil
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
