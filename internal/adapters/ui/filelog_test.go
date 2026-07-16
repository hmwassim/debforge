package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileLogger_LogWritesToFile(t *testing.T) {
	dir := t.TempDir()
	fl := NewFileLogger(dir)
	defer fl.Close()

	fl.log("INFO", "hello %s", "world")
	fl.Close()

	pattern := filepath.Join(dir, "debforge-*.log")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) != 1 {
		t.Fatalf("expected 1 log file, got %d (err=%v)", len(matches), err)
	}

	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	out := string(data)

	if !strings.Contains(out, "[INFO] hello world") {
		t.Errorf("expected [INFO] hello world, got %q", out)
	}
	_today := time.Now().Format("2006-01-02")
	if !strings.Contains(out, "["+_today+" ") {
		t.Errorf("expected date %s in timestamp, got %q", _today, out)
	}
}

func TestFileLogger_MultipleLevels(t *testing.T) {
	dir := t.TempDir()
	fl := NewFileLogger(dir)
	defer fl.Close()

	fl.log("INFO", "info msg")
	fl.log("WARN", "warn msg")
	fl.log("ERROR", "error msg")
	fl.log("SUCCESS", "ok msg")
	fl.log("PROMPT", "question? -> y")
	fl.Close()

	data, _ := os.ReadFile(filepath.Join(dir, "debforge-"+time.Now().Format("2006-01-02")+".log"))
	out := string(data)

	for _, want := range []string{"[INFO] info msg", "[WARN] warn msg", "[ERROR] error msg", "[SUCCESS] ok msg", "[PROMPT] question? -> y"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in log, got %q", want, out)
		}
	}
}

func TestFileLogger_CreatesDirectoryOnFirstWrite(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "deep")
	fl := NewFileLogger(dir)

	fl.log("INFO", "created")
	fl.Close()

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("expected directory to be created")
	}

	data, _ := os.ReadFile(filepath.Join(dir, "debforge-"+time.Now().Format("2006-01-02")+".log"))
	if !strings.Contains(string(data), "[INFO] created") {
		t.Error("expected log content")
	}
}

func TestFileLogger_GracefulDegradation(t *testing.T) {
	fl := NewFileLogger("/dev/null/impossible-path-that-cannot-exist")
	defer fl.Close()

	fl.log("INFO", "should not panic")
	fl.log("ERROR", "still no panic")
	fl.Close()
}

func TestFileLogger_CloseThenLog(t *testing.T) {
	dir := t.TempDir()
	fl := NewFileLogger(dir)

	fl.log("INFO", "before close")
	fl.Close()
	fl.log("INFO", "after close — should be no-op")
}

func TestSymbolToLevel(t *testing.T) {
	tests := []struct {
		symbol string
		want   string
	}{
		{"i", "INFO"},
		{"*", "SUCCESS"},
		{"!", "WARN"},
		{"x", "ERROR"},
		{"?", "PROMPT"},
		{"z", "UNKNOWN"},
	}
	for _, tc := range tests {
		got := symbolToLevel(tc.symbol)
		if got != tc.want {
			t.Errorf("symbolToLevel(%q) = %q, want %q", tc.symbol, got, tc.want)
		}
	}
}

func TestFileLogger_Retention(t *testing.T) {
	dir := t.TempDir()
	for i := 1; i <= 35; i++ {
		name := filepath.Join(dir, fmt.Sprintf("debforge-2026-01-%02d.log", i))
		os.WriteFile(name, []byte("seed"), 0640)
	}
	fl := NewFileLogger(dir)
	fl.Write("INFO", "trigger prune")
	fl.Close()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	// RetentionDays: prune keeps the last 30 files from the sorted list,
	// which includes today's freshly created file.
	if len(names) != RetentionDays {
		t.Errorf("expected %d files, got %d: %v", RetentionDays, len(names), names)
	}
	// Oldest seeded file should be gone
	for _, n := range names {
		if n == "debforge-2026-01-01.log" {
			t.Error("debforge-2026-01-01.log should have been pruned")
		}
	}
}

func TestFileLogger_Permissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "newlogdir")
	fl := NewFileLogger(dir)
	fl.Write("INFO", "test perms")
	fl.Close()

	di, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if di.Mode().Perm() != 0750 {
		t.Errorf("dir perm = %o, want 0750", di.Mode().Perm())
	}

	logs, _ := filepath.Glob(filepath.Join(dir, "debforge-*.log"))
	if len(logs) != 1 {
		t.Fatalf("expected 1 log file, got %d", len(logs))
	}
	fi, err := os.Stat(logs[0])
	if err != nil {
		t.Fatalf("stat log: %v", err)
	}
	if fi.Mode().Perm() != 0640 {
		t.Errorf("file perm = %o, want 0640", fi.Mode().Perm())
	}
}

func TestFileLogger_NilSafe(t *testing.T) {
	var fl *FileLogger
	fl.Write("INFO", "no panic")
	fl.Close()
}
