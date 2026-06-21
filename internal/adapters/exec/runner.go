package exec

import (
	"bytes"
	"context"
	"os"
	"os/exec"

	"github.com/hmwassim/debforge/internal/ports"
)

type Runner struct{}

func NewRunner() *Runner {
	return &Runner{}
}

// Run executes name with args using the current environment and working
// directory, capturing stdout/stderr. It is the common case of
// RunWithOptions and is implemented in terms of it so there is exactly one
// place that builds an *exec.Cmd.
func (r *Runner) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	return r.RunWithOptions(ctx, ports.RunOptions{}, name, args...)
}

// RunWithOptions executes name with args, applying opts. See ports.RunOptions
// for details on Dir, Env, Stdout, and Stderr semantics.
func (r *Runner) RunWithOptions(ctx context.Context, opts ports.RunOptions, name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)

	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}
	if len(opts.Env) > 0 {
		cmd.Env = append(os.Environ(), opts.Env...)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	if opts.Stdout != nil {
		cmd.Stdout = opts.Stdout
	} else {
		cmd.Stdout = &stdoutBuf
	}
	if opts.Stderr != nil {
		cmd.Stderr = opts.Stderr
	} else {
		cmd.Stderr = &stderrBuf
	}

	err := cmd.Run()
	return stdoutBuf.Bytes(), stderrBuf.Bytes(), err
}

var _ ports.CommandRunner = (*Runner)(nil)
