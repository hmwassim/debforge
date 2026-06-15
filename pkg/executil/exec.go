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

func RunWithSpinner(cmd *exec.Cmd, msg string) error {
	terminal := isTerminal(os.Stderr)
	done := make(chan struct{})
	go func() {
		if !terminal {
			<-done
			return
		}
		chars := []string{"|", "/", "-", "\\"}
		i := 0
		for {
			select {
			case <-done:
				fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", len(msg)+2))
				return
			default:
				fmt.Fprintf(os.Stderr, "\r%s %s ", msg, chars[i%len(chars)])
				i++
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()

	err := Run(cmd)
	close(done)
	return err
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
