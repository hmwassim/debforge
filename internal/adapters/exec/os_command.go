package exec

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/hmwassim/debforge/internal/adapters/ui"
)

type OSCommandRunner struct{}

func NewOSCommandRunner() *OSCommandRunner {
	return &OSCommandRunner{}
}

func (r *OSCommandRunner) Run(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return outBuf.Bytes(), errBuf.Bytes(), err
}

func (r *OSCommandRunner) RunWithEnv(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if env != nil {
		cmd.Env = env
	}
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	return outBuf.Bytes(), errBuf.Bytes(), err
}

func shortDesc(name string, args ...string) string {
	var b strings.Builder
	b.WriteString(name)
	for _, a := range args {
		if b.Len() >= 64 {
			b.WriteString(" ...")
			break
		}
		b.WriteString(" ")
		b.WriteString(a)
	}
	return b.String()
}

func (r *OSCommandRunner) RunWithSpinner(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = io.Discard
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	s := ui.NewDisplay(ctx, os.Stderr, shortDesc(name, args...))
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

var _ interface {
	Run(context.Context, string, ...string) ([]byte, []byte, error)
	RunWithSpinner(context.Context, string, ...string) error
} = (*OSCommandRunner)(nil)
