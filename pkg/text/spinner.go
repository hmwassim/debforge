package text

import (
	"fmt"
	"io"
	"time"
)

var spinFrames = []string{"|", "/", "-", "\\"}

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
	pre, suf := ansiPair(s.color, bold+blue)
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

func (s *Spinner) Done() {
	if s.stop == nil {
		return
	}
	close(s.stop)
	<-s.done
	s.stop = nil
	pre, suf := ansiPair(s.color, bold+green)
	fmt.Fprintf(s.w, "\r%s[*]%s %s\n", pre, suf, s.desc)
}

func (s *Spinner) Fail() {
	if s.stop == nil {
		return
	}
	close(s.stop)
	<-s.done
	s.stop = nil
	pre, suf := ansiPair(s.color, bold+red)
	fmt.Fprintf(s.w, "\r%s[x]%s %s\n", pre, suf, s.desc)
}

func ansiPair(ok bool, code string) (string, string) {
	if ok {
		return code, reset
	}
	return "", ""
}
