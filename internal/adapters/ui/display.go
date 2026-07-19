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

// Display is an animated terminal spinner that shows a rotating frame and
// a description string. It degrades gracefully to a static [i] prefix when
// the output is not a terminal.
type Display struct {
	w       io.Writer
	content string
	ctx     context.Context
	stop    chan struct{}
	sdone   chan struct{}
	fileLog *FileLogger

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

// NewDisplay starts a new spinner goroutine that animates content on w
// until Done, Fail, DoneWarn, or DoneInfo is called. If fileLog is
// non-nil, spinner transitions are written to the log file.
func NewDisplay(ctx context.Context, w io.Writer, content string, fileLog *FileLogger) *Display {
	content = textutil.UcFirst(content)
	d := &Display{w: w, content: content, ctx: ctx, tty: isTerminal(w), fileLog: fileLog}
	if fileLog != nil {
		fileLog.log("INFO", "spinner: %s", content)
	}
	d.stop = make(chan struct{})
	d.sdone = make(chan struct{})
	go d.run()
	return d
}

// SetDesc updates the spinner's description text. The new description is
// logged to the file only when it actually changes.
func (d *Display) SetDesc(content string) {
	uc := textutil.UcFirst(content)
	d.mu.Lock()
	changed := d.content != uc
	d.content = uc
	d.mu.Unlock()
	if changed && d.fileLog != nil {
		d.fileLog.log("INFO", "spinner: %s", uc)
	}
}

// Pause temporarily stops the spinner animation and clears the current line.
// The clear is unconditional — a ticker frame can land right after Stop()
// returns (race between writef and the stop channel), and the clear removes
// that residual frame before the caller writes its own line.
func (d *Display) Pause() {
	d.mu.Lock()
	if !d.done {
		d.paused = true
	}
	d.mu.Unlock()
	if d.tty {
		defaultConsole.writef(d.w, "\r\033[K")
	}
}

// Resume restarts the spinner animation after a Pause.
func (d *Display) Resume() {
	d.mu.Lock()
	if d.done {
		d.mu.Unlock()
		return
	}
	d.paused = false
	content := d.content
	d.mu.Unlock()
	if d.tty {
		defaultConsole.writef(d.w, "\r%s[%s]%s %s\033[K", bold+magenta, spinFrames[0], reset, content)
	}
}

func (d *Display) run() {
	defer close(d.sdone)
	defer func() {
		if r := recover(); r != nil {
			if d.fileLog != nil {
				d.fileLog.log("ERROR", "spinner panic: %v", r)
			}
		}
		d.mu.Lock()
		d.done = true
		d.mu.Unlock()
	}()

	d.mu.Lock()
	content := d.content
	d.mu.Unlock()

	if !d.tty {
		// Nothing to animate without a terminal: emit one line up front
		// and let doneWith print the final state when the spinner ends.
		defaultConsole.writef(d.w, "[%s] %s\n", "i", content)
		<-d.stopOrCtxDone()
		return
	}
	defaultConsole.writef(d.w, "\r%s[%s]%s %s\033[K", bold+magenta, spinFrames[0], reset, content)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	idx := 1
	for {
		select {
		case <-d.ctx.Done():
			return
		case <-d.stop:
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

	if d.fileLog != nil {
		d.fileLog.log(symbolToLevel(mark), "spinner done: %s", content)
	}
}

// Done marks the spinner as successfully completed with a checkmark.
func (d *Display) Done() { d.doneWith("*", green) }

// Fail marks the spinner as failed with an x mark.
func (d *Display) Fail() { d.doneWith("x", red) }

// DoneWarn marks the spinner as completed with a warning.
func (d *Display) DoneWarn() { d.doneWith("!", yellow) }

// DoneInfo marks the spinner as completed with an informational marker.
func (d *Display) DoneInfo() { d.doneWith("i", blue) }

// Stop stops the spinner animation without printing a completion message.
func (d *Display) Stop() {
	d.mu.Lock()
	if d.done {
		d.mu.Unlock()
		return
	}
	d.done = true
	paused := d.paused
	d.mu.Unlock()

	d.dOnce.Do(func() {
		if d.stop != nil {
			close(d.stop)
			<-d.sdone
			d.stop = nil
		}
	})
	_ = paused // Discarded — Stop is silent.
}

var _ ports.Spinner = (*Display)(nil)
