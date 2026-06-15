package text

import (
	"fmt"
	"io"
	"time"
)

type Spinner struct {
	w     io.Writer
	desc  string
	stop  chan struct{}
	done  chan struct{}
	color bool
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

func (s *Spinner) run() {
	pre, suf := ansiPair(s.color, frameColor)
	// First write has no \r — the cursor is already at a fresh line
	// (either from a prior \n or because no output preceded this).
	fmt.Fprintf(s.w, "%s[%s]%s %s", pre, spinFrames[0], suf, s.desc)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	idx := 1
	for {
		select {
		case <-s.stop:
			close(s.done)
			return
		case <-ticker.C:
			fmt.Fprintf(s.w, "\r%s[%s]%s %s", pre, spinFrames[idx%len(spinFrames)], suf, s.desc)
			idx++
		}
	}
}

func (s *Spinner) Pause() {
	if s.stop == nil {
		return
	}
	close(s.stop)
	<-s.done
	s.stop = nil
}

func (s *Spinner) Resume() {
	if s.stop != nil || !IsTerminal(s.w) {
		return
	}
	s.stop = make(chan struct{})
	s.done = make(chan struct{})
	go s.run()
}

func (s *Spinner) Done() {
	if s.stop != nil {
		close(s.stop)
		<-s.done
		s.stop = nil
	}
	pre, suf := ansiPair(s.color, successColor)
	cr := ""
	if IsTerminal(s.w) {
		cr = "\r"
	}
	fmt.Fprintf(s.w, "%s%s[*]%s %s\n", cr, pre, suf, s.desc)
}

func (s *Spinner) Fail() {
	if s.stop != nil {
		close(s.stop)
		<-s.done
		s.stop = nil
	}
	pre, suf := ansiPair(s.color, errorColor)
	cr := ""
	if IsTerminal(s.w) {
		cr = "\r"
	}
	fmt.Fprintf(s.w, "%s%s[x]%s %s\n", cr, pre, suf, s.desc)
}

func (s *Spinner) UpdateDesc(desc string) {
	s.desc = desc
}
