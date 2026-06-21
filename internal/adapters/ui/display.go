package ui

import (
	"context"
	"io"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/hmwassim/debforge/internal/ports"
)

var spinFrames = []string{"|", "/", "-", "\\"}

func stripTrailingDots(s string) string {
	for len(s) > 0 && s[len(s)-1] == '.' {
		s = s[:len(s)-1]
	}
	return s
}

// Display owns a single terminal line.  It renders [frame] <content> at
// 100 ms intervals and stops on Done / Fail / context cancel.
// The caller controls the full <content> — no extra "..." is appended.
type Display struct {
	w       io.Writer
	content string
	ctx     context.Context
	stop    chan struct{}
	sdone   chan struct{}
	color   bool

	mu       sync.Mutex
	paused   bool
	done     bool
	doneOnce sync.Once
}

func NewDisplay(ctx context.Context, w io.Writer, content string) *Display {
	if len(content) > 0 {
		r, size := utf8.DecodeRuneInString(content)
		content = string(unicode.ToUpper(r)) + content[size:]
	}
	d := &Display{w: w, content: content, ctx: ctx}
	if !useColor(w) {
		return d
	}
	d.stop = make(chan struct{})
	d.sdone = make(chan struct{})
	d.color = true
	go d.run()
	return d
}

func (d *Display) SetDesc(content string) {
	d.mu.Lock()
	d.content = content
	d.mu.Unlock()
}

func (d *Display) Pause() {
	d.mu.Lock()
	d.paused = true
	d.mu.Unlock()
	if d.color {
		defaultConsole.writef(d.w, "\r\033[K")
	}
}

func (d *Display) Resume() {
	d.mu.Lock()
	d.paused = false
	d.mu.Unlock()
	if d.stop != nil && d.color {
		defaultConsole.writef(d.w, "\r%s[%s]%s %s\033[K", bold+blue, spinFrames[0], reset, d.content)
	}
}

func (d *Display) run() {
	defaultConsole.writef(d.w, "\r%s[%s]%s %s\033[K", bold+blue, spinFrames[0], reset, d.content)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	idx := 1
	for {
		select {
		case <-d.ctx.Done():
			close(d.sdone)
			return
		case <-d.stop:
			close(d.sdone)
			return
		case <-ticker.C:
			d.mu.Lock()
			p := d.paused
			d.mu.Unlock()
			if p {
				continue
			}
			defaultConsole.writef(d.w, "\r%s[%s]%s %s\033[K", bold+blue, spinFrames[idx%len(spinFrames)], reset, d.content)
			idx++
		}
	}
}

func (d *Display) doneWith(mark, code string) {
	d.mu.Lock()
	if d.done {
		d.mu.Unlock()
		return
	}
	d.done = true
	d.paused = false
	d.mu.Unlock()

	if d.stop != nil {
		d.doneOnce.Do(func() {
			close(d.stop)
			<-d.sdone
		})
		d.stop = nil
	}

	content := stripTrailingDots(d.content)
	if d.color {
		defaultConsole.writef(d.w, "\r%s[%s]%s %s\033[K\n", bold+code, mark, reset, content)
	} else {
		defaultConsole.writef(d.w, "[%s] %s\n", mark, content)
	}
}

func (d *Display) Done()     { d.doneWith("*", green) }
func (d *Display) Fail()     { d.doneWith("x", red) }
func (d *Display) DoneWarn() { d.doneWith("!", yellow) }

var _ ports.Spinner = (*Display)(nil)
