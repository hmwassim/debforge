package aptpty

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/hmwassim/debforge/internal/ports"
	"golang.org/x/term"
)

// ptySession provides read/write access to a command's PTY plus lifecycle
// operations. This separates the PTY loop logic from the os/exec + creack/pty
// creation, making the loop testable with a mock.
type ptySession interface {
	io.ReadWriteCloser
	Wait() error
	Signal(os.Signal) error
	SetSize(rows, cols uint16) error
}

type realPtySession struct {
	ptmx *os.File
	cmd  *exec.Cmd
}

func (s *realPtySession) Read(b []byte) (int, error)  { return s.ptmx.Read(b) }
func (s *realPtySession) Write(b []byte) (int, error) { return s.ptmx.Write(b) }
func (s *realPtySession) Close() error                { return s.ptmx.Close() }
func (s *realPtySession) Wait() error                 { return s.cmd.Wait() }
func (s *realPtySession) Signal(sig os.Signal) error  { return s.cmd.Process.Signal(sig) }
func (s *realPtySession) SetSize(rows, cols uint16) error {
	return pty.Setsize(s.ptmx, &pty.Winsize{Rows: rows, Cols: cols})
}

// ptyFactory creates a ptySession for the given command. Package-level
// variable so tests can inject a mock.
var startPty ptyFactory = func(ctx context.Context, name string, args ...string) (ptySession, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C", "LANGUAGE=C")
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}
	return &realPtySession{ptmx: ptmx, cmd: cmd}, nil
}

type ptyFactory func(ctx context.Context, name string, args ...string) (ptySession, error)

// ---- PTY loop -------------------------------------------------------------

// run drives apt-get interactively through a PTY so its native progress
// output can be parsed and turned into spinner updates. This is the one
// deliberate place in the codebase that still constructs an *exec.Cmd
// directly instead of going through ports.CommandRunner: CommandRunner's
// contract is "run a command, get back its output," whereas this needs a
// live pseudo-terminal (for apt-get's progress reporting and any
// debconf [Y/n] prompts) plus a goroutine pumping stdin/signals into it -
// a fundamentally different, lower-level operation that the simple
// run/capture abstraction was never meant to cover.
func run(ctx context.Context, runner ports.CommandRunner, aptArgs []string, spinner ports.Spinner) error {
	return runWithSession(ctx, runner, aptArgs, spinner, startPty)
}

func runWithSession(ctx context.Context, runner ports.CommandRunner, aptArgs []string, spinner ports.Spinner, factory ptyFactory) error {
	if spinner == nil {
		return fmt.Errorf("aptpty: spinner must not be nil")
	}

	var mode string
	var pkgArgs []string
	if len(aptArgs) > 0 {
		mode = aptArgs[0]
		if len(aptArgs) > 1 {
			pkgArgs = aptArgs[1:]
		}
	}

	state := &runState{phase: phaseDownload}

	dlTotal, dlLabel, err := getDownloadSize(ctx, runner, mode, pkgArgs)
	if err == nil && dlTotal > 0 {
		state.overallTotal = dlTotal
		state.overallLabel = dlLabel
	}

	cmdLine := []string{"apt-get"}
	cmdLine = append(cmdLine, aptArgs...)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sess, err := factory(ctx, cmdLine[0], cmdLine[1:]...)
	if err != nil {
		return fmt.Errorf("starting apt-get: %w", err)
	}
	defer sess.Close()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH, syscall.SIGINT)
	go func() {
		for sig := range sigCh {
			switch sig {
			case syscall.SIGWINCH:
				if w, h, err := term.GetSize(int(os.Stdin.Fd())); err == nil {
					_ = sess.SetSize(uint16(h), uint16(w))
				}
			case syscall.SIGINT:
				cancel()
				_ = sess.Signal(syscall.SIGTERM)
			}
		}
	}()
	if w, h, err := term.GetSize(int(os.Stdin.Fd())); err == nil {
		_ = sess.SetSize(uint16(h), uint16(w))
	}

	go func() { _, _ = io.Copy(sess, os.Stdin) }()

	type readResult struct {
		data []byte
		err  error
	}
	resultCh := make(chan readResult, 100)
	go func() {
		rbuf := make([]byte, 65536)
		for {
			n, err := sess.Read(rbuf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, rbuf[:n])
				resultCh <- readResult{data: chunk}
			}
			if err != nil {
				resultCh <- readResult{err: err}
				return
			}
		}
	}()

	var (
		sbuf    []byte
		cur     int64
		total   int64
		pkg     string
		aptErrs []string
	)

	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()

mainLoop:
	for {
		select {
		case <-ctx.Done():
			break mainLoop

		case result := <-resultCh:
			if len(result.data) > 0 {
				sbuf = append(sbuf, result.data...)

				for {
					nl := bytes.IndexByte(sbuf, '\n')
					if nl < 0 {
						break
					}
					processSegments(sbuf[:nl], state, &cur, &total, &pkg, &aptErrs, spinner)
					sbuf = sbuf[nl+1:]
				}

				if len(sbuf) > 0 {
					for len(sbuf) > 0 && sbuf[len(sbuf)-1] == '\r' {
						sbuf = sbuf[:len(sbuf)-1]
					}
					if lastR := bytes.LastIndexByte(sbuf, '\r'); lastR >= 0 {
						sbuf = sbuf[lastR+1:]
					}
					if len(sbuf) > 0 {
						processSegments(sbuf, state, &cur, &total, &pkg, &aptErrs, spinner)
						sbuf = sbuf[:0]
					}
				}
			}
			if result.err != nil {
				if !errors.Is(result.err, io.EOF) {
					break mainLoop
				}
				processSegments(sbuf, state, &cur, &total, &pkg, &aptErrs, spinner)
				break mainLoop
			}

		case <-ticker.C:
		}

		if state.phase == phaseDownload && total > 0 && state.cumulativeDone+cur > 0 {
			spinner.SetDesc(progressDesc(state, pkg, state.cumulativeDone+cur))
		} else if state.phase == phaseInstall {
			spinner.SetDesc(progressDesc(state, pkg, 0))
		}
	}

	signal.Stop(sigCh)
	close(sigCh)

	if err := sess.Wait(); err != nil {
		var exitErr *exec.ExitError
		code := 0
		if errors.As(err, &exitErr) {
			code = exitErr.ExitCode()
		}
		msg := fmt.Sprintf("apt-get failed with exit code %d", code)
		if len(aptErrs) > 0 {
			return fmt.Errorf("%s: %s", msg, strings.Join(aptErrs, "; "))
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}
