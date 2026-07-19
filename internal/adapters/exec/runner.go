// Package exec provides a concrete implementation of ports.CommandRunner
// using os/exec.
package exec

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/hmwassim/debforge/internal/ports"
)

// Runner executes external commands via os/exec.
type Runner struct {
	logFn func(name string, args []string, stdout, stderr []byte, err error)
}

// NewRunner returns a new Runner.
func NewRunner() *Runner {
	return &Runner{}
}

// SetLogFn sets an optional callback invoked after every command execution.
// When set, it receives the command name, arguments, captured stdout/stderr,
// and the error (if any). The callback must be safe for concurrent use.
func (r *Runner) SetLogFn(fn func(string, []string, []byte, []byte, error)) {
	r.logFn = fn
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
	cmd.Env = CleanEnv()
	if len(opts.Env) > 0 {
		cmd.Env = append(cmd.Env, opts.Env...)
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
	if r.logFn != nil {
		r.logFn(name, args, stdoutBuf.Bytes(), stderrBuf.Bytes(), err)
	}
	return stdoutBuf.Bytes(), stderrBuf.Bytes(), err
}

// sensitiveEnvPrefixes lists environment variable prefixes that should
// not be inherited by child processes to prevent credential leakage.
var sensitiveEnvPrefixes = []string{
	"SSH_", "GPG_", "AWS_", "AZURE_", "GOOGLE_",
	"GITHUB_", "GITLAB_", "TOKEN", "SECRET", "PASSWORD",
	"CREDENTIAL", "API_KEY", "PRIVATE_KEY",
}

// CleanEnv returns a minimal environment with PATH, HOME, USER, TERM,
// and LANG/LC_* preserved, stripping sensitive variables like SSH keys,
// cloud credentials, and tokens. This prevents credential leakage to
// child processes (apt-get, git, go build) when running as root.
func CleanEnv() []string {
	env := os.Environ()
	clean := make([]string, 0, len(env))
	for _, e := range env {
		key := strings.SplitN(e, "=", 2)[0]
		// Always keep core variables
		switch key {
		case "PATH", "HOME", "USER", "TERM", "SHELL",
			"LANG", "LC_ALL", "LC_CTYPE", "LC_MESSAGES",
			"DEBIAN_FRONTEND", "DEBCONF_NONINTERACTIVE",
			"TMPDIR", "GOPATH", "GOMODCACHE", "GOCACHE",
			"GOFLAGS", "GONOSUMCHECK", "GONOSUMDB", "GOPRIVATE":
			// GOPATH/GOCACHE are kept for self-update builds;
			// GOFLAGS/GONOSUMCHECK are intentionally kept so Go
			// module behavior is not silently altered.
			clean = append(clean, e)
			continue
		}
		sensitive := false
		for _, prefix := range sensitiveEnvPrefixes {
			if strings.HasPrefix(key, prefix) || strings.Contains(key, prefix) {
				sensitive = true
				break
			}
		}
		if !sensitive {
			clean = append(clean, e)
		}
	}
	return clean
}

var _ ports.CommandRunner = (*Runner)(nil)
