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

func RunInstall(packages []string) error {
	if len(packages) == 0 {
		return nil
	}
	args := append([]string{"install", "-y"}, packages...)
	return run(args)
}

func RunInstallBackports(packages []string, suite string) error {
	if len(packages) == 0 {
		return nil
	}
	if suite == "" {
		suite = "trixie-backports"
	}
	args := append([]string{"install", "-y", "-t", suite}, packages...)
	return run(args)
}

func RunRemove(packages []string) error {
	if len(packages) == 0 {
		return nil
	}
	args := append([]string{"purge", "-y", "--autoremove"}, packages...)
	return run(args)
}

func RunUpgrade() error {
	return run([]string{"upgrade", "-y"})
}

// ---- pre-run: --print-uris ------------------------------------------------

func getDownloadSize(mode string, args []string) (int64, string) {
	cmdLine := []string{mode, "--print-uris", "-y"}
	cmdLine = append(cmdLine, args...)
	cmd := exec.Command("apt-get", cmdLine...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C", "LANGUAGE=C")

	out, err := cmd.Output()
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

func handleLine(line string, state *runState, cur, total *int64, pkg *string) {
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
		if *total > 0 {
			if state.prevPkgTotal > 0 {
				state.cumulativeDone += state.prevPkgTotal
			}
			showProgress(state, *pkg, state.cumulativeDone)
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
		fmt.Fprintln(os.Stderr, line)
	}
}

func after(s, prefix string) string {
	i := strings.Index(s, prefix)
	if i < 0 {
		return ""
	}
	return s[i+len(prefix):]
}

// ---- display --------------------------------------------------------------

func showProgress(state *runState, pkg string, cur int64) {
	var buf bytes.Buffer
	if state.phase == phaseDownload {
		curS := humanSize(cur)
		if state.overallLabel != "" {
			fmt.Fprintf(&buf, "\rDownloading %s... [%s/%s]\033[K", pkg, curS, state.overallLabel)
		} else if state.overallTotal > 0 {
			fmt.Fprintf(&buf, "\rDownloading %s... [%s/%s]\033[K", pkg, curS, humanSize(state.overallTotal))
		} else {
			fmt.Fprintf(&buf, "\rDownloading %s... [%s/1]\033[K", pkg, curS)
		}
	} else {
		disp := pkg
		if state.installPkg != "" {
			disp = state.installPkg
		}
		fmt.Fprintf(&buf, "\rInstalling %s...\033[K", disp)
	}
	os.Stderr.Write(buf.Bytes())
}

// ---- PTY loop -------------------------------------------------------------

func run(aptArgs []string) error {
	var mode string
	var pkgArgs []string
	if len(aptArgs) > 0 {
		mode = aptArgs[0]
		if len(aptArgs) > 1 {
			pkgArgs = aptArgs[1:]
		}
	}

	state := &runState{phase: phaseDownload}

	if total, label := getDownloadSize(mode, pkgArgs); total > 0 {
		state.overallTotal = total
		state.overallLabel = label
	}

	cmdLine := []string{"apt-get"}
	cmdLine = append(cmdLine, aptArgs...)

	ctx, cancel := context.WithCancel(context.Background())
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
		sbuf  []byte
		cur   int64
		total int64
		pkg   string
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
				for _, seg := range bytes.Split(sbuf[:nl], []byte{'\r'}) {
					if len(seg) == 0 {
						continue
					}
					handleLine(string(seg), state, &cur, &total, &pkg)
				}
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
					seg := stripANSI(string(sbuf))
					matched := false
					switch {
					case strings.Contains(seg, "% ["):
						if c, t, n, ok := parseProgress(seg); ok {
							if t != state.prevPkgTotal && state.prevPkgTotal > 0 {
								state.cumulativeDone += state.prevPkgTotal
							}
							state.prevPkgTotal = t
							cur = c
							total = t
							pkg = n
							matched = true
						}
					case strings.Contains(seg, "? [") && strings.Contains(seg, "[Y/n]"):
						fmt.Fprintln(os.Stderr, seg)
						matched = true
					case strings.Contains(seg, "Download size:") ||
						strings.Contains(seg, "Need to get "):
						handleLine(seg, state, &cur, &total, &pkg)
						matched = true
					}
					if matched {
						sbuf = sbuf[:0]
					}
				}
			}

		case <-timer.C:

		case err := <-errCh:
			if err != nil && !errors.Is(err, io.EOF) {
				break mainLoop
			}
			for _, seg := range bytes.Split(sbuf, []byte{'\r'}) {
				if len(seg) > 0 {
					handleLine(string(seg), state, &cur, &total, &pkg)
				}
			}
			break mainLoop
		}

		if state.phase == phaseDownload && total > 0 {
			showProgress(state, pkg, state.cumulativeDone+cur)
		} else if state.phase == phaseInstall {
			if state.installPkg != "" {
				showProgress(state, state.installPkg, 0)
			} else {
				showProgress(state, pkg, 0)
			}
		}
	}

	signal.Stop(sigCh)
	close(sigCh)

	if err := cmd.Wait(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Errorf("apt-get failed with exit code %d", exitErr.ExitCode())
		}
		return fmt.Errorf("apt-get: %w", err)
	}
	return nil
}
