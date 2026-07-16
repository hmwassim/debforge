package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// RetentionDays is the number of daily log files (including today's) kept
// under a FileLogger's directory. Older debforge-*.log files are removed
// on rotation. Command output can now include full apt-get transcripts
// (see aptpty.LineLog), so without a cap this directory would grow
// unbounded over months of use.
const RetentionDays = 30

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
// concurrent use, a no-op when the logger's directory is unwritable, and a
// no-op on a nil *FileLogger (the DEBFORGE_NO_LOG=1 / disabled case) so
// callers never need to guard every call site with a nil check.
func (f *FileLogger) Write(level, format string, args ...any) {
	if f == nil {
		return
	}
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

// rotate closes the current file (if any), opens a new one for the given
// date, and prunes daily files older than RetentionDays. Directory creation
// is idempotent. The log dir/file perms are 0750/0640: debforge routinely
// runs as root (install/remove/setup require it), so without this, every
// local user could read the full command and apt-get history under a
// root-owned, world-readable directory.
func (f *FileLogger) rotate(date string) {
	if f.file != nil {
		f.file.Close()
		f.file = nil
	}
	_ = os.MkdirAll(f.dir, 0750)
	path := filepath.Join(f.dir, fmt.Sprintf("debforge-%s.log", date))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640)
	if err != nil {
		f.file = nil
		f.date = ""
		return
	}
	f.file = file
	f.date = date
	f.prune()
}

// prune removes debforge-*.log files beyond the most recent RetentionDays,
// oldest first. Best-effort: any error (unreadable dir, unremovable file)
// is silently ignored, matching the rest of FileLogger's fail-open design -
// a full disk or a permissions hiccup here must never block the command
// the user actually ran.
func (f *FileLogger) prune() {
	entries, err := os.ReadDir(f.dir)
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if !e.IsDir() && strings.HasPrefix(n, "debforge-") && strings.HasSuffix(n, ".log") {
			names = append(names, n)
		}
	}
	if len(names) <= RetentionDays {
		return
	}
	sort.Strings(names) // date-suffixed names sort chronologically
	for _, n := range names[:len(names)-RetentionDays] {
		_ = os.Remove(filepath.Join(f.dir, n))
	}
}

// Close flushes and closes the underlying file. After Close the logger is
// inert — further log calls are no-ops. Safe to call on a nil *FileLogger.
func (f *FileLogger) Close() {
	if f == nil {
		return
	}
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
