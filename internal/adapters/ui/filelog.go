package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileLogger writes timestamped log lines to a daily-rotated file under a
// configurable directory. It is safe for concurrent use. If the log directory
// cannot be created or files cannot be opened, logging is silently skipped.
type FileLogger struct {
	mu   sync.Mutex
	dir  string
	file *os.File
	date string
}

// NewFileLogger returns a FileLogger that writes to dir. The directory is
// created lazily on the first write.
func NewFileLogger(dir string) *FileLogger {
	return &FileLogger{dir: dir}
}

// Write logs a timestamped line with the given level. It is safe for
// concurrent use and a no-op when the logger's directory is unwritable.
func (f *FileLogger) Write(level, format string, args ...any) {
	f.log(level, format, args...)
}

// log writes a single timestamped line to the current day's log file.
func (f *FileLogger) log(level, format string, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	if f.date != today || f.file == nil {
		f.rotate(today)
	}
	if f.file == nil {
		return
	}

	msg := fmt.Sprintf(format, args...)
	ts := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(f.file, "[%s] [%s] %s\n", ts, level, msg)
}

// rotate closes the current file (if any) and opens a new one for the given
// date. Directory creation is idempotent.
func (f *FileLogger) rotate(date string) {
	if f.file != nil {
		f.file.Close()
		f.file = nil
	}
	_ = os.MkdirAll(f.dir, 0755)
	path := filepath.Join(f.dir, fmt.Sprintf("debforge-%s.log", date))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		f.file = nil
		f.date = ""
		return
	}
	f.file = file
	f.date = date
}

// Close flushes and closes the underlying file. After Close the logger is
// inert — further log calls are no-ops.
func (f *FileLogger) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.file != nil {
		f.file.Close()
		f.file = nil
		f.date = ""
	}
}

func symbolToLevel(symbol string) string {
	switch symbol {
	case "i":
		return "INFO"
	case "*":
		return "SUCCESS"
	case "!":
		return "WARN"
	case "x":
		return "ERROR"
	case "?":
		return "PROMPT"
	default:
		return "UNKNOWN"
	}
}
