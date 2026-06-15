package executil

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/hmwassim/debforge/pkg/text"
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
	if cmd.Stdout == nil {
		cmd.Stdout = io.Discard
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return err
	}

	s := text.StartSpinner(os.Stderr, desc)

	if err := cmd.Wait(); err != nil {
		s.Fail()
		if s := strings.TrimSpace(stderr.String()); s != "" {
			return fmt.Errorf("%s: %w", s, err)
		}
		return err
	}
	s.Done()
	return nil
}
