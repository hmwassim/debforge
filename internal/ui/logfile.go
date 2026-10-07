package ui

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hmwassim/debforge/internal/system"
)

// Log is the persistent debug log: one file per day, 30 days kept. It
// fails open: if the directory is not writable, logging is disabled once
// and never retried.
type Log struct {
	mu   sync.Mutex
	f    *os.File
	dead bool
	Dir  string
	Now  func() time.Time
}

const keepLogs = 30

// OpenLog prepares a log in dir. It never returns nil.
func OpenLog(dir string) *Log {
	return &Log{Dir: dir, Now: time.Now}
}

func (l *Log) open() bool {
	if l.f != nil {
		return true
	}
	if l.dead || l.Dir == "" {
		return false
	}
	if err := os.MkdirAll(l.Dir, 0o750); err != nil {
		l.dead = true
		return false
	}
	name := filepath.Join(l.Dir, "debforge-"+l.Now().Format("2006-01-02")+".log")
	f, err := os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		l.dead = true
		return false
	}
	l.f = f
	l.prune()
	return true
}

func (l *Log) prune() {
	m, _ := filepath.Glob(filepath.Join(l.Dir, "debforge-*.log"))
	sort.Strings(m)
	for len(m) > keepLogs {
		os.Remove(m[0])
		m = m[1:]
	}
}

// Printf writes one timestamped line.
func (l *Log) Printf(format string, a ...any) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.open() {
		return
	}
	fmt.Fprintf(l.f, "%s %s\n", l.Now().Format(time.RFC3339), fmt.Sprintf(format, a...))
}

// Command is a system.LogFunc.
func (l *Log) Command(c system.Cmd, r system.Result, err error, took time.Duration) {
	status := "ok"
	if err != nil {
		status = "FAILED: " + err.Error()
	}
	l.Printf("exec %s (%s) %s", c.String(), took.Round(time.Millisecond), status)
	if err != nil && len(r.Stderr) > 0 {
		l.Printf("  stderr: %s", strings.TrimSpace(string(r.Stderr)))
	}
}

// Close closes the file.
func (l *Log) Close() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f != nil {
		l.f.Close()
		l.f = nil
	}
}

// Writer returns an io.Writer that logs each line with prefix.
func (l *Log) Writer(prefix string) io.Writer {
	return &lineWriter{l: l, prefix: prefix}
}

type lineWriter struct {
	l      *Log
	prefix string
	buf    []byte
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		w.l.Printf("%s%s", w.prefix, w.buf[:i])
		w.buf = w.buf[i+1:]
	}
	return len(p), nil
}
