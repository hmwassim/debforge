package text

import (
	"fmt"
	"io"
	"time"
)

type Progress struct {
	total    int64
	current  int64
	desc     string
	w        io.Writer
	start    time.Time
	last     time.Time
	frameIdx int
	color    bool
}

func NewProgress(w io.Writer, total int64, desc string) *Progress {
	return &Progress{total: total, desc: desc, w: w, start: time.Now(), color: useColor(w)}
}

func (p *Progress) Write(buf []byte) (int, error) {
	n := len(buf)
	p.current += int64(n)
	if time.Since(p.last) < 100*time.Millisecond {
		return n, nil
	}
	p.last = time.Now()
	p.write()
	return n, nil
}

func (p *Progress) Done() {
	p.current = p.total
	p.write()
}

func (p *Progress) Fail() {
	pre, suf := ansiPair(p.color, errorColor)
	if IsTerminal(p.w) {
		fmt.Fprintf(p.w, "\r%s[x]%s %s...\033[K\n", pre, suf, p.desc)
	} else {
		fmt.Fprintf(p.w, "[x] %s...\n", p.desc)
	}
}

func (p *Progress) write() {
	if p.current >= p.total {
		pre, suf := ansiPair(p.color, successColor)
		if IsTerminal(p.w) {
			fmt.Fprintf(p.w, "\r%s[*]%s %s...\033[K\n", pre, suf, p.desc)
		} else {
			fmt.Fprintf(p.w, "[*] %s...\n", p.desc)
		}
		return
	}
	if !IsTerminal(p.w) {
		return
	}
	frame := spinFrames[p.frameIdx%len(spinFrames)]
	p.frameIdx++
	tv, unit := formatSize(p.total)
	divisor := float64(p.total) / tv
	cv := float64(p.current) / divisor
	pre, suf := ansiPair(p.color, frameColor)
	fmt.Fprintf(p.w, "\r%s[%s]%s %s... [%.0f/%.0f %s]\033[K", pre, frame, suf, p.desc, cv, tv, unit)
}

func formatSize(bytes int64) (float64, string) {
	switch {
	case bytes >= 1024*1024*1024:
		return float64(bytes) / (1024 * 1024 * 1024), "GB"
	case bytes >= 1024*1024:
		return float64(bytes) / (1024 * 1024), "MB"
	case bytes >= 1024:
		return float64(bytes) / 1024, "KB"
	default:
		return float64(bytes), "B"
	}
}
