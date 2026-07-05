package aptpty

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/textutil"
)

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
		if len(pkg) >= pkgWidth {
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
