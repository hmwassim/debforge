package text

import (
	"fmt"
	"io"
	"sync"
	"time"
)

type Spinner struct {
	w     io.Writer
	desc  string
	stop  chan struct{}
	done  chan struct{}
	color bool

	mu     sync.Mutex
	paused bool
}

func StartSpinner(w io.Writer, desc string) *Spinner {
	s := &Spinner{w: w, desc: desc}
	if !IsTerminal(w) {
		return s
	}
	s.stop = make(chan struct{})
	s.done = make(chan struct{})
	s.color = useColor(w)
	go s.run()
	return s
}

func (s *Spinner) Pause() {
	s.mu.Lock()
	s.paused = true
	s.mu.Unlock()
}

func (s *Spinner) Resume() {
	s.mu.Lock()
	s.paused = false
	s.mu.Unlock()
}

func (s *Spinner) run() {
	pre, suf := ansiPair(s.color, frameColor)
	fmt.Fprintf(s.w, "%s[%s]%s %s\033[K", pre, spinFrames[0], suf, s.desc)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	idx := 1
	for {
		select {
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
			fmt.Fprintf(s.w, "\r%s[%s]%s %s\033[K", pre, spinFrames[idx%len(spinFrames)], suf, s.desc)
			idx++
		}
	}
}

func (s *Spinner) doneFail(ok bool) {
	s.mu.Lock()
	s.paused = false
	s.mu.Unlock()

	if s.stop != nil {
		close(s.stop)
		<-s.done
		s.stop = nil
	}

	mark, pair := "[*]", successColor
	if !ok {
		mark, pair = "[x]", errorColor
	}
	pre, suf := ansiPair(s.color, pair)
	if IsTerminal(s.w) {
		fmt.Fprintf(s.w, "\r%s%s%s %s\033[K\n", pre, mark, suf, s.desc)
	} else {
		fmt.Fprintf(s.w, "%s %s\n", mark, s.desc)
	}
}

func (s *Spinner) Done() {
	s.doneFail(true)
}

func (s *Spinner) Fail() {
	s.doneFail(false)
}
