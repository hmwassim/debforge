// Package aptpty drives apt-get interactively through a PTY so its native
// progress output can be parsed and turned into spinner updates.
package aptpty

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/hmwassim/debforge/internal/dpkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/textutil"
	"golang.org/x/term"
)

// AptExecFunc is the function signature for running apt-get through the PTY.
// Installers hold a field of this type to allow test injection instead of
// calling RunInstall / RunRemove / RunInstallBackports directly.
type AptExecFunc func(ctx context.Context, runner ports.CommandRunner, aptArgs []string, spinner ports.Spinner) error

// AptExec is the default AptExecFunc that runs apt-get through a real PTY.
// It is exported so each installer's NewInstaller can assign it as the
// default; tests can override per-installer.
var AptExec AptExecFunc = run

const (
	phaseDownload = 0
	phaseInstall  = 1
)

type runState struct {
	phase          int
	overallTotal   int64
	overallLabel   string
	cumulativeDone int64
	prevPkgTotal   int64
	installPkg     string
}

// ---- public API -----------------------------------------------------------

// RunInstall runs apt-get install -y for the given packages.
func RunInstall(ctx context.Context, runner ports.CommandRunner, packages []string, spinner ports.Spinner) error {
	if len(packages) == 0 {
		return nil
	}
	args := append([]string{"install", "-y"}, packages...)
	return run(ctx, runner, args, spinner)
}

// RunInstallBackports runs apt-get install -y -t <suite> for the given packages.
func RunInstallBackports(ctx context.Context, runner ports.CommandRunner, packages []string, suite string, spinner ports.Spinner) error {
	if len(packages) == 0 {
		return nil
	}
	if suite == "" {
		suite = "trixie-backports"
	}
	args := append([]string{"install", "-y", "-t", suite}, packages...)
	return run(ctx, runner, args, spinner)
}

// RunRemove runs apt-get remove -y for the given packages.
func RunRemove(ctx context.Context, runner ports.CommandRunner, packages []string, spinner ports.Spinner) error {
	if len(packages) == 0 {
		return nil
	}
	args := append([]string{"remove", "-y"}, packages...)
	return run(ctx, runner, args, spinner)
}

// RunUpdate runs apt-get update to refresh repository metadata.
func RunUpdate(ctx context.Context, runner ports.CommandRunner, spinner ports.Spinner) error {
	spinner.SetDesc("Refreshing repositories...")
	_, _, err := runner.Run(ctx, "apt-get", "update")
	return err
}

// RunUpgrade runs apt-get full-upgrade -y through the PTY so the user sees
// spinner-based per-package progress. full-upgrade handles dependency changes
// that require removing old packages and installing new ones — needed for
// kernel meta-packages (linux-base, linux-headers, linux-image) whose
// versioned dependencies are replaced entirely on each release. upgrade
// alone would silently skip those packages.
func RunUpgrade(ctx context.Context, runner ports.CommandRunner, spinner ports.Spinner) error {
	return AptExec(ctx, runner, []string{"full-upgrade", "-y"}, spinner)
}

// FindInstalledConflicts returns the subset of names that are currently
// installed according to dpkg-query.
func FindInstalledConflicts(ctx context.Context, runner ports.CommandRunner, names []string) []string {
	var found []string
	for _, name := range names {
		ok, err := dpkg.IsInstalled(ctx, runner, name)
		if err != nil {
			continue
		}
		if ok {
			found = append(found, name)
		}
	}
	return found
}

// ---- pre-run: --print-uris ------------------------------------------------

func getDownloadSize(ctx context.Context, runner ports.CommandRunner, mode string, args []string) (int64, string) {
	cmdLine := []string{mode, "--print-uris", "-y"}
	cmdLine = append(cmdLine, args...)

	opts := ports.RunOptions{Env: []string{"LC_ALL=C", "LANG=C", "LANGUAGE=C"}}
	out, _, err := runner.RunWithOptions(ctx, opts, "apt-get", cmdLine...)
	if err != nil {
		return 0, ""
	}

	var total int64
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if len(line) == 0 || line[0] != '\'' {
			continue
		}
		f := strings.Fields(line)
		if len(f) >= 3 {
			sz, err := strconv.ParseInt(f[2], 10, 64)
			if err == nil {
				total += sz
			}
		}
	}

	if total > 0 {
		return total, textutil.FormatSize(total)
	}
	return 0, ""
}

// ---- line processing ------------------------------------------------------

func handleLine(line string, state *runState, cur, total *int64, pkg *string, spinner ports.Spinner) {
	line = stripANSI(line)

	if strings.Contains(line, "Download size:") {
		if slash := strings.LastIndex(line, "/"); slash >= 0 {
			rest := strings.TrimSpace(line[slash+1:])
			if f := strings.Fields(rest); len(f) >= 2 {
				state.overallTotal = parseSize(f[0], f[1])
			}
			state.overallLabel = rest
		}
		return
	}
	if i := strings.Index(line, "Need to get "); i >= 0 {
		rest := line[i+12:]
		if of := strings.Index(rest, " of "); of >= 0 {
			tmp := rest[:of]
			if slash := strings.LastIndex(tmp, "/"); slash >= 0 {
				tmp = strings.TrimSpace(tmp[slash+1:])
			}
			if f := strings.Fields(tmp); len(f) >= 2 {
				state.overallTotal = parseSize(f[0], f[1])
			}
			state.overallLabel = tmp
		}
		return
	}

	if strings.HasPrefix(line, "Fetched ") && strings.Contains(line, " in ") {
		if state.prevPkgTotal > 0 {
			state.cumulativeDone += state.prevPkgTotal
		}
		if state.overallTotal > 0 && spinner != nil {
			final := textutil.FormatSize(state.cumulativeDone)
			tot := textutil.FormatSize(state.overallTotal)
			spinner.SetDesc(fmt.Sprintf("Downloading %s... [%s/%s]", *pkg, final, tot))
		}
		*cur = 0
		*total = 0
		*pkg = ""
		state.prevPkgTotal = 0
		state.phase = phaseInstall
		return
	}

	if c, t, n, ok := parseProgress(line); ok {
		if t != state.prevPkgTotal && state.prevPkgTotal > 0 {
			state.cumulativeDone += state.prevPkgTotal
		}
		state.prevPkgTotal = t
		*cur = c
		*total = t
		*pkg = n
		return
	}

	var p string
	switch {
	case strings.Contains(line, "Setting up "):
		p = after(line, "Setting up ")
	case strings.Contains(line, "Unpacking "):
		p = after(line, "Unpacking ")
	}
	if p != "" {
		state.phase = phaseInstall
		slash := strings.Index(p, "/")
		space := strings.Index(p, " ")
		if slash >= 0 && (space < 0 || slash < space) {
			p = p[slash+1:]
		}
		end := strings.IndexAny(p, " (")
		if end < 0 {
			end = len(p)
		}
		state.installPkg = p[:end]
		return
	}

	if strings.Contains(line, "? [") && strings.Contains(line, "[Y/n]") {
		fmt.Fprintln(os.Stderr, line)
	}
}

func after(s, prefix string) string {
	_, after, _ := strings.Cut(s, prefix)
	return after
}

func processSegments(data []byte, state *runState, cur, total *int64, pkg *string, aptErrs *[]string, spinner ports.Spinner) {
	for _, seg := range bytes.Split(data, []byte{'\r'}) {
		if len(seg) == 0 {
			continue
		}
		handleLine(string(seg), state, cur, total, pkg, spinner)
		collectErr(string(seg), aptErrs)
	}
}

func collectErr(s string, aptErrs *[]string) {
	s = stripANSI(s)
	if len(*aptErrs) >= 5 {
		return
	}
	if strings.HasPrefix(s, "E: ") || strings.HasPrefix(s, "W: ") ||
		strings.HasPrefix(s, "dpkg: ") {
		*aptErrs = append(*aptErrs, s)
	}
}

const pkgWidth = 24

func progressDesc(state *runState, pkg string, cur int64) string {
	if state.phase == phaseDownload {
		display := pkg
		if len(pkg) > pkgWidth {
			display = pkg[:pkgWidth-3] + "..."
		} else {
			display = fmt.Sprintf("%-*s", pkgWidth, display)
		}
		curS := textutil.FormatSize(cur)
		if state.overallLabel != "" {
			return fmt.Sprintf("Downloading %s[%s/%s]", display, curS, state.overallLabel)
		} else if state.overallTotal > 0 {
			return fmt.Sprintf("Downloading %s[%s/%s]", display, curS, textutil.FormatSize(state.overallTotal))
		} else {
			return fmt.Sprintf("Downloading %s[%s/%s]", display, curS, "?")
		}
	} else {
		disp := pkg
		if state.installPkg != "" {
			disp = state.installPkg
		}
		return fmt.Sprintf("Installing %s...", disp)
	}
}

// ---- PTY session abstraction ----------------------------------------------

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

	if total, label := getDownloadSize(ctx, runner, mode, pkgArgs); total > 0 {
		state.overallTotal = total
		state.overallLabel = label
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

	go func() { io.Copy(sess, os.Stdin) }()

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
