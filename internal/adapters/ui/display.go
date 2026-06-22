package ui

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/textutil"
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

	// tty reports whether w is a terminal, using the same isTerminal
	// check ConsoleLogger uses for Info/Warn/Error/Prompt. Without this,
	// a spinner piped to a file or log collector would interleave raw
	// ANSI escapes and carriage returns into the log on every tick, while
	// every other UI output already degrades gracefully to plain text.
	tty bool

	mu     sync.Mutex
	paused bool
	done   bool
	dOnce  sync.Once
}

func NewDisplay(ctx context.Context, w io.Writer, content string) *Display {
	content = textutil.UcFirst(content)
	d := &Display{w: w, content: content, ctx: ctx, tty: isTerminal(w)}
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
	if d.tty {
		defaultConsole.writef(d.w, "\r\033[K")
	}
}

func (d *Display) Resume() {
	d.mu.Lock()
	d.paused = false
	content := d.content
	d.mu.Unlock()
	if d.tty {
		defaultConsole.writef(d.w, "\r%s[%s]%s %s\033[K", bold+magenta, spinFrames[0], reset, content)
	}
}

func (d *Display) run() {
	if !d.tty {
		// Nothing to animate without a terminal: emit one line up front
		// and let doneWith print the final state when the spinner ends.
		defaultConsole.writef(d.w, "[%s] %s\n", "i", d.content)
		<-d.stopOrCtxDone()
		close(d.sdone)
		return
	}

	defaultConsole.writef(d.w, "\r%s[%s]%s %s\033[K", bold+magenta, spinFrames[0], reset, d.content)
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
			content := d.content
			d.mu.Unlock()
			if p {
				continue
			}
			defaultConsole.writef(d.w, "\r%s[%s]%s %s\033[K", bold+magenta, spinFrames[idx%len(spinFrames)], reset, content)
			idx++
		}
	}
}

// stopOrCtxDone returns a channel that closes when either the spinner is
// stopped or its context is done, for the non-tty run() path which has no
// ticker to multiplex against.
func (d *Display) stopOrCtxDone() <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		select {
		case <-d.ctx.Done():
		case <-d.stop:
		}
		close(ch)
	}()
	return ch
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
	content := d.content
	d.mu.Unlock()

	d.dOnce.Do(func() {
		if d.stop != nil {
			close(d.stop)
			<-d.sdone
			d.stop = nil
		}
	})

	content = stripTrailingDots(content)
	switch {
	case !d.tty:
		defaultConsole.writef(d.w, "[%s] %s\n", mark, content)
	case paused:
		defaultConsole.writef(d.w, "%s[%s]%s %s\n", bold+code, mark, reset, content)
	default:
		defaultConsole.writef(d.w, "\r%s[%s]%s %s\033[K\n", bold+code, mark, reset, content)
	}
}

func (d *Display) Done()     { d.doneWith("*", green) }
func (d *Display) Fail()     { d.doneWith("x", red) }
func (d *Display) DoneWarn() { d.doneWith("!", yellow) }

var _ ports.Spinner = (*Display)(nil)
