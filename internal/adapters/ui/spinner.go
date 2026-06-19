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

type ConsoleSpinner struct {
	w     io.Writer
	desc  string
	ctx   context.Context
	stop  chan struct{}
	done  chan struct{}
	color bool

	mu       sync.Mutex
	paused   bool
	doneOnce sync.Once
}

func NewConsoleSpinner(ctx context.Context, w io.Writer, desc string) *ConsoleSpinner {
	if len(desc) > 0 {
		r, size := utf8.DecodeRuneInString(desc)
		desc = string(unicode.ToUpper(r)) + desc[size:]
	}
	s := &ConsoleSpinner{w: w, desc: desc, ctx: ctx}
	if !useColor(w) {
		return s
	}
	s.stop = make(chan struct{})
	s.done = make(chan struct{})
	s.color = true
	go s.run()
	return s
}

func (s *ConsoleSpinner) SetDesc(desc string) {
	s.mu.Lock()
	s.desc = desc
	s.mu.Unlock()
}

func (s *ConsoleSpinner) Pause() {
	s.mu.Lock()
	s.paused = true
	s.mu.Unlock()
	if s.color {
		defaultConsole.writef(s.w, "\r\033[K")
	}
}

func (s *ConsoleSpinner) Resume() {
	s.mu.Lock()
	s.paused = false
	s.mu.Unlock()
	if s.stop != nil && s.color {
		defaultConsole.writef(s.w, "\r%s[%s]%s %s\033[K", bold+blue, spinFrames[0], reset, s.desc)
	}
}

func (s *ConsoleSpinner) run() {
	defaultConsole.writef(s.w, "\r%s[%s]%s %s\033[K", bold+blue, spinFrames[0], reset, s.desc)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	idx := 1
	for {
		select {
		case <-s.ctx.Done():
			close(s.done)
			return
		case <-s.stop:
			close(s.done)
			return
		case <-ticker.C:
			s.mu.Lock()
			p := s.paused
			s.mu.Unlock()
			if p {
				continue
			}
			defaultConsole.writef(s.w, "\r%s[%s]%s %s\033[K", bold+blue, spinFrames[idx%len(spinFrames)], reset, s.desc)
			idx++
		}
	}
}

func (s *ConsoleSpinner) doneFail(ok bool) {
	s.mu.Lock()
	s.paused = false
	s.mu.Unlock()

	if s.stop != nil {
		s.doneOnce.Do(func() {
			close(s.stop)
			<-s.done
		})
		s.stop = nil
	}

	mark, code := "*", green
	if !ok {
		mark, code = "x", red
	}
	if s.color {
		defaultConsole.writef(s.w, "\r%s[%s]%s %s\033[K\n", bold+code, mark, reset, s.desc)
	} else {
		defaultConsole.writef(s.w, "[%s] %s\n", mark, s.desc)
	}
}

func (s *ConsoleSpinner) Done() { s.doneFail(true) }

func (s *ConsoleSpinner) Fail() { s.doneFail(false) }

var _ ports.Spinner = (*ConsoleSpinner)(nil)
