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
	"github.com/hmwassim/debforge/internal/ports"
	"golang.org/x/term"
)

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

func RunInstall(ctx context.Context, runner ports.CommandRunner, packages []string, spinner ports.Spinner) error {
	if len(packages) == 0 {
		return nil
	}
	args := append([]string{"install", "-y"}, packages...)
	return run(ctx, runner, args, spinner)
}

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

func RunRemove(ctx context.Context, runner ports.CommandRunner, packages []string, spinner ports.Spinner) error {
	if len(packages) == 0 {
		return nil
	}
	args := append([]string{"autoremove", "-y"}, packages...)
	return run(ctx, runner, args, spinner)
}

// ---- pre-run: --print-uris ------------------------------------------------

// getDownloadSize shells out via the injected ports.CommandRunner (rather
// than os/exec directly) to ask apt-get how much it would download, so this
// is a plain non-interactive command and fits the CommandRunner abstraction
// like any other.
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
		return total, humanSize(total)
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
	if strings.Contains(line, "Need to get ") {
		if i := strings.Index(line, "Need to get "); i >= 0 {
			rest := line[i+12:]
			if of := strings.Index(rest, " of "); of >= 0 {
				tmp := rest[:of]
				if f := strings.Fields(tmp); len(f) >= 2 {
					state.overallTotal = parseSize(f[0], f[1])
				}
				state.overallLabel = tmp
			}
		}
		return
	}

	if strings.HasPrefix(line, "Fetched ") && strings.Contains(line, " in ") {
		if state.prevPkgTotal > 0 {
			state.cumulativeDone += state.prevPkgTotal
		}
		if state.overallTotal > 0 && spinner != nil {
			final := humanSize(state.cumulativeDone)
			tot := humanSize(state.overallTotal)
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

	if state.phase == phaseInstall {
		var p string
		switch {
		case strings.Contains(line, "Setting up "):
			p = after(line, "Setting up ")
		case strings.Contains(line, "Unpacking "):
			p = after(line, "Unpacking ")
		}
		if p != "" {
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
	}

	if strings.Contains(line, "? [") && strings.Contains(line, "[Y/n]") {
		// Forward apt-get's own interactive prompt (e.g. an unexpected
		// debconf question) verbatim. This is the wrapped tool's own
		// output, not a debforge status message, so it intentionally
		// bypasses the spinner/UI abstraction rather than being phrased
		// as a SetDesc update.
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

// ---- display --------------------------------------------------------------

func progressDesc(state *runState, pkg string, cur int64) string {
	if state.phase == phaseDownload {
		curS := humanSize(cur)
		if state.overallLabel != "" {
			return fmt.Sprintf("Downloading %s... [%s/%s]", pkg, curS, state.overallLabel)
		} else if state.overallTotal > 0 {
			return fmt.Sprintf("Downloading %s... [%s/%s]", pkg, curS, humanSize(state.overallTotal))
		} else {
			return fmt.Sprintf("Downloading %s... [%s/1]", pkg, curS)
		}
	} else {
		disp := pkg
		if state.installPkg != "" {
			disp = state.installPkg
		}
		return fmt.Sprintf("Installing %s...", disp)
	}
}

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

	cmd := exec.CommandContext(ctx, cmdLine[0], cmdLine[1:]...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C", "LANGUAGE=C")

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return fmt.Errorf("starting apt-get: %w", err)
	}
	defer ptmx.Close()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH, syscall.SIGINT)
	go func() {
		for sig := range sigCh {
			switch sig {
			case syscall.SIGWINCH:
				if w, h, err := term.GetSize(int(os.Stdin.Fd())); err == nil {
					_ = pty.Setsize(ptmx, &pty.Winsize{Rows: uint16(h), Cols: uint16(w)})
				}
			case syscall.SIGINT:
				cancel()
				if cmd.Process != nil {
					cmd.Process.Signal(syscall.SIGTERM)
				}
			}
		}
	}()
	if w, h, err := term.GetSize(int(os.Stdin.Fd())); err == nil {
		_ = pty.Setsize(ptmx, &pty.Winsize{Rows: uint16(h), Cols: uint16(w)})
	}

	go func() { io.Copy(ptmx, os.Stdin) }()

	dataCh := make(chan []byte, 100)
	errCh := make(chan error, 1)
	go func() {
		rbuf := make([]byte, 65536)
		for {
			n, err := ptmx.Read(rbuf)
			if err != nil {
				errCh <- err
				return
			}
			chunk := make([]byte, n)
			copy(chunk, rbuf[:n])
			dataCh <- chunk
		}
	}()

	var (
		sbuf     []byte
		cur      int64
		total    int64
		pkg      string
		aptErrs  []string
	)

	timer := time.NewTimer(0)
	if !timer.Stop() {
		<-timer.C
	}

mainLoop:
	for {
		timer.Reset(150 * time.Millisecond)
		select {
		case <-ctx.Done():
			break mainLoop

		case chunk := <-dataCh:
			sbuf = append(sbuf, chunk...)

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

		case <-timer.C:

		case err := <-errCh:
			if err != nil && !errors.Is(err, io.EOF) {
				break mainLoop
			}
			processSegments(sbuf, state, &cur, &total, &pkg, &aptErrs, spinner)
			break mainLoop
		}

		if state.phase == phaseDownload && total > 0 && state.cumulativeDone+cur > 0 {
			spinner.SetDesc(progressDesc(state, pkg, state.cumulativeDone+cur))
		} else if state.phase == phaseInstall {
			spinner.SetDesc(progressDesc(state, pkg, 0))
		}
	}

	signal.Stop(sigCh)
	close(sigCh)

	if err := cmd.Wait(); err != nil {
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
