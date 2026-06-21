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

type Display struct {
	w       io.Writer
	content string
	ctx     context.Context
	stop    chan struct{}
	sdone   chan struct{}

	mu     sync.Mutex
	paused bool
	done   bool
	dOnce  sync.Once
}

func NewDisplay(ctx context.Context, w io.Writer, content string) *Display {
	if len(content) > 0 {
		r, size := utf8.DecodeRuneInString(content)
		content = string(unicode.ToUpper(r)) + content[size:]
	}
	d := &Display{w: w, content: content, ctx: ctx}
	d.stop = make(chan struct{})
	d.sdone = make(chan struct{})
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
	defaultConsole.writef(d.w, "\r\033[K")
}

func (d *Display) Resume() {
	d.mu.Lock()
	d.paused = false
	d.mu.Unlock()
	defaultConsole.writef(d.w, "\r%s[%s]%s %s\033[K", bold+blue, spinFrames[0], reset, d.content)
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
	paused := d.paused
	d.paused = false
	d.mu.Unlock()

	d.dOnce.Do(func() {
		if d.stop != nil {
			close(d.stop)
			<-d.sdone
			d.stop = nil
		}
	})

	content := stripTrailingDots(d.content)
	if paused {
		defaultConsole.writef(d.w, "%s[%s]%s %s\n", bold+code, mark, reset, content)
	} else {
		defaultConsole.writef(d.w, "\r%s[%s]%s %s\033[K\n", bold+code, mark, reset, content)
	}
}

func (d *Display) Done()     { d.doneWith("*", green) }
func (d *Display) Fail()     { d.doneWith("x", red) }
func (d *Display) DoneWarn() { d.doneWith("!", yellow) }

var _ ports.Spinner = (*Display)(nil)
