package executil

import (
	"bytes"
	"errors"
	"io"
	"os/exec"
	"strings"
)

func Run(cmd *exec.Cmd) error {
	if cmd.Stdout == nil {
		cmd.Stdout = io.Discard
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if s := strings.TrimSpace(stderr.String()); s != "" {
			return errors.New(s)
		}
		return err
	}
	return nil
}
