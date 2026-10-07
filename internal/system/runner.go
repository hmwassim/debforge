// Package system wraps every interaction with external programs and with
// the host's package database. All external commands go through Runner so
// tests can substitute a fake; nothing in debforge calls os/exec directly.
package system

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Cmd describes one external command invocation.
type Cmd struct {
	Name string
	Args []string
	// Env holds extra KEY=VALUE entries appended to the base environment.
	Env []string
	// Dir is the working directory; empty means "/" (never the caller's cwd).
	Dir   string
	Stdin io.Reader
	// Stdout and Stderr, when set, receive a live copy of the output in
	// addition to the buffered copy returned in Result.
	Stdout io.Writer
	Stderr io.Writer
	// ExtraFiles are passed to the child as fd 3, 4, ...
	ExtraFiles []*os.File
	// Timeout bounds the command; zero means no timeout beyond ctx.
	Timeout time.Duration
	// OwnProcessGroup puts the child in its own process group so a
	// terminal Ctrl-C does not reach it (used for apt/dpkg, which must
	// never be interrupted mid-transaction).
	OwnProcessGroup bool
}

func (c Cmd) String() string {
	return strings.TrimSpace(c.Name + " " + strings.Join(c.Args, " "))
}

// Result is the buffered output of a finished command.
type Result struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// Runner runs external commands.
type Runner interface {
	Run(ctx context.Context, c Cmd) (Result, error)
}

// ExitError is returned when a command ran but exited non-zero.
type ExitError struct {
	Cmd    string
	Code   int
	Stderr string
}

func (e *ExitError) Error() string {
	msg := fmt.Sprintf("%s: exit status %d", e.Cmd, e.Code)
	if tail := lastLines(e.Stderr, 5); tail != "" {
		msg += ": " + tail
	}
	return msg
}

// LogFunc receives every finished command for the file log.
type LogFunc func(c Cmd, r Result, err error, took time.Duration)

// ExecRunner runs commands with os/exec.
type ExecRunner struct {
	Log LogFunc
}

// Run implements Runner.
func (x ExecRunner) Run(ctx context.Context, c Cmd) (Result, error) {
	if c.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	cmd.Env = append(BaseEnv(os.Environ()), c.Env...)
	cmd.Dir = c.Dir
	if cmd.Dir == "" {
		cmd.Dir = "/"
	}
	cmd.Stdin = c.Stdin
	cmd.ExtraFiles = c.ExtraFiles
	if c.OwnProcessGroup {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	// Give children a chance to exit cleanly on cancellation.
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 10 * time.Second

	var stdout, stderr bytes.Buffer
	cmd.Stdout = teeOrBuf(&stdout, c.Stdout)
	cmd.Stderr = teeOrBuf(&stderr, c.Stderr)

	start := time.Now()
	err := cmd.Run()
	res := Result{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if cmd.ProcessState != nil {
		res.ExitCode = cmd.ProcessState.ExitCode()
	}
	err = classify(ctx, c, res, err)
	if x.Log != nil {
		x.Log(c, res, err, time.Since(start))
	}
	return res, err
}

func classify(ctx context.Context, c Cmd, res Result, err error) error {
	if err == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("%s: %w", c.Name, ctxErr)
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return &ExitError{Cmd: c.Name, Code: res.ExitCode, Stderr: string(res.Stderr)}
	}
	return fmt.Errorf("%s: %w", c.Name, err)
}

func teeOrBuf(buf *bytes.Buffer, w io.Writer) io.Writer {
	if w == nil {
		return buf
	}
	return io.MultiWriter(buf, w)
}

// envAllow lists the variables children may inherit. Everything else
// (tokens, cloud credentials, DISPLAY-specific junk) is dropped.
var envAllow = map[string]bool{
	"PATH": true, "HOME": true, "USER": true, "LOGNAME": true, "TERM": true,
	"SUDO_USER": true, "SUDO_UID": true, "SUDO_GID": true,
	"http_proxy": true, "https_proxy": true, "no_proxy": true,
	"HTTP_PROXY": true, "HTTPS_PROXY": true, "NO_PROXY": true,
	"SSH_AUTH_SOCK": true, "TMPDIR": true,
}

// BaseEnv filters environ down to the allowlist and pins a C locale so
// parsed output is stable.
func BaseEnv(environ []string) []string {
	out := make([]string, 0, len(envAllow)+4)
	for _, kv := range environ {
		k, _, ok := strings.Cut(kv, "=")
		if ok && envAllow[k] {
			out = append(out, kv)
		}
	}
	return append(out, "LC_ALL=C", "LANG=C", "LANGUAGE=C", "GIT_TERMINAL_PROMPT=0")
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.TrimSpace(strings.Join(lines, " | "))
}

// HasPCIVendor reports whether any PCI device has the given vendor id
// (4 lowercase hex digits), reading sysfs under root ("" in production).
func HasPCIVendor(root, vendor string) bool {
	m, _ := filepath.Glob(root + "/sys/bus/pci/devices/*/vendor")
	for _, f := range m {
		b, err := os.ReadFile(f)
		if err == nil && strings.TrimSpace(strings.ToLower(string(b))) == "0x"+vendor {
			return true
		}
	}
	return false
}
