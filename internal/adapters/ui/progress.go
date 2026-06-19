package ui

import (
	"io"
	"sync"
	"time"

	"github.com/hmwassim/debforge/internal/ports"
)

type ConsoleProgress struct {
	mu       sync.Mutex
	total    int64
	current  int64
	desc     string
	w        io.Writer
	start    time.Time
	last     time.Time
	frameIdx int
	color    bool
}

func NewConsoleProgress(w io.Writer, total int64, desc string) *ConsoleProgress {
	return &ConsoleProgress{total: total, desc: desc, w: w, start: time.Now(), color: useColor(w)}
}

func (p *ConsoleProgress) Write(buf []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := len(buf)
	p.current += int64(n)
	if p.current >= p.total {
		return n, nil
	}
	if time.Since(p.last) < 100*time.Millisecond {
		return n, nil
	}
	p.last = time.Now()
	p.write()
	return n, nil
}

func (p *ConsoleProgress) Done() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.current = p.total
	p.write()
}

func (p *ConsoleProgress) Fail() {
	if p.color {
		defaultConsole.writef(p.w, "\r%s[x]%s %s...\033[K\n", bold+red, reset, p.desc)
	} else {
		defaultConsole.writef(p.w, "[x] %s...\n", p.desc)
	}
}

func (p *ConsoleProgress) write() {
	if p.current >= p.total {
		if p.color {
			defaultConsole.writef(p.w, "\r%s[*]%s %s...\033[K\n", bold+green, reset, p.desc)
		} else {
			defaultConsole.writef(p.w, "[*] %s...\n", p.desc)
		}
		return
	}
	if !p.color {
		return
	}
	if p.total <= 0 {
		return
	}
	frame := spinFrames[p.frameIdx%len(spinFrames)]
	p.frameIdx++
	tv, unit := formatSize(p.total)
	divisor := float64(p.total) / tv
	cv := float64(p.current) / divisor
	defaultConsole.writef(p.w, "\r%s[%s]%s %s... [%.0f/%.0f %s]\033[K", bold+blue, frame, reset, p.desc, cv, tv, unit)
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

var _ ports.Progress = (*ConsoleProgress)(nil)
