package ui

import (
	"bytes"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
)

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	old := os.Stderr
	os.Stderr = w

	var buf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		_, _ = io.Copy(&buf, r)
		wg.Done()
	}()

	fn()

	w.Close()
	os.Stderr = old
	wg.Wait()
	return buf.String()
}

func TestConsoleLogger_Info(t *testing.T) {
	out := captureStderr(t, func() {
		NewConsoleLogger().Info("test %s", "message")
	})
	if !strings.Contains(out, "[i]") {
		t.Errorf("expected [i] marker, got %q", out)
	}
	if !strings.Contains(out, "test message") {
		t.Errorf("expected message content, got %q", out)
	}
}

func TestConsoleLogger_Success(t *testing.T) {
	out := captureStderr(t, func() {
		NewConsoleLogger().Success("done")
	})
	if !strings.Contains(out, "[*]") {
		t.Errorf("expected [*] marker, got %q", out)
	}
	if !strings.Contains(out, "done") {
		t.Errorf("expected message, got %q", out)
	}
}

func TestConsoleLogger_Warn(t *testing.T) {
	out := captureStderr(t, func() {
		NewConsoleLogger().Warn("caution: %s", "hot")
	})
	if !strings.Contains(out, "[!]") {
		t.Errorf("expected [!] marker, got %q", out)
	}
	if !strings.Contains(out, "caution: hot") {
		t.Errorf("expected message, got %q", out)
	}
}

func TestConsoleLogger_Error(t *testing.T) {
	out := captureStderr(t, func() {
		NewConsoleLogger().Error("fail: %v", "boom")
	})
	if !strings.Contains(out, "[x]") {
		t.Errorf("expected [x] marker, got %q", out)
	}
	if !strings.Contains(out, "fail: boom") {
		t.Errorf("expected message, got %q", out)
	}
}

func TestConsoleLogger_NonTerminal(t *testing.T) {
	out := captureStderr(t, func() {
		NewConsoleLogger().Info("plain")
	})
	if strings.Contains(out, "\033[") {
		t.Errorf("expected no ANSI codes on pipe, got %q", out)
	}
}

func TestIsTerminal(t *testing.T) {
	if isTerminal(&bytes.Buffer{}) {
		t.Error("bytes.Buffer should not be a terminal")
	}
	if isTerminal(os.Stderr) {
		t.Log("stderr is a terminal in this environment")
	}
}

func captureStderrAndStdin(t *testing.T, stdinInput string, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	old := os.Stderr
	os.Stderr = w

	oldStdin := os.Stdin
	if stdinInput != "" {
		sin, sw, err := os.Pipe()
		if err != nil {
			t.Fatalf("stdin pipe: %v", err)
		}
		os.Stdin = sin
		_, _ = sw.WriteString(stdinInput)
		sw.Close()
		defer func() { os.Stdin = oldStdin }()
	}

	var buf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		_, _ = io.Copy(&buf, r)
		wg.Done()
	}()

	fn()

	w.Close()
	os.Stderr = old
	wg.Wait()
	return buf.String()
}

func TestConsoleLogger_Prompt_yes(t *testing.T) {
	out := captureStderrAndStdin(t, "y\n", func() {
		NewConsoleLogger().Prompt("continue?")
	})
	if !strings.Contains(out, "[?] continue? [y/N]") {
		t.Errorf("expected prompt output, got %q", out)
	}
}

func TestConsoleLogger_Prompt_no(t *testing.T) {
	out := captureStderrAndStdin(t, "n\n", func() {
		if NewConsoleLogger().Prompt("continue?") {
			t.Error("expected false for n")
		}
	})
	if !strings.Contains(out, "[?] continue? [y/N]") {
		t.Errorf("expected prompt output, got %q", out)
	}
}

func TestConsoleLogger_Prompt_defaultNo(t *testing.T) {
	out := captureStderrAndStdin(t, "\n", func() {
		if NewConsoleLogger().Prompt("continue?") {
			t.Error("expected false for empty input")
		}
	})
	if !strings.Contains(out, "[?] continue? [y/N]") {
		t.Errorf("expected prompt output, got %q", out)
	}
}

func TestConsoleLogger_TTY_Info(t *testing.T) {
	isTerminal = func(io.Writer) bool { return true }
	defer restoreIsTerminal()
	out := captureStderr(t, func() {
		NewConsoleLogger().Info("hello %s", "tty")
	})
	if !strings.Contains(out, "[i]") || !strings.Contains(out, "hello tty") {
		t.Errorf("expected [i] and hello tty in output, got %q", out)
	}
}

func TestConsoleLogger_TTY_Success(t *testing.T) {
	isTerminal = func(io.Writer) bool { return true }
	defer restoreIsTerminal()
	out := captureStderr(t, func() {
		NewConsoleLogger().Success("done")
	})
	if !strings.Contains(out, "[*]") || !strings.Contains(out, "done") {
		t.Errorf("expected [*] and done in output, got %q", out)
	}
}

func TestConsoleLogger_TTY_Error(t *testing.T) {
	isTerminal = func(io.Writer) bool { return true }
	defer restoreIsTerminal()
	out := captureStderr(t, func() {
		NewConsoleLogger().Error("fail")
	})
	if !strings.Contains(out, "[x]") || !strings.Contains(out, "fail") {
		t.Errorf("expected [x] and fail in output, got %q", out)
	}
}

func TestConsoleLogger_TTY_Warn(t *testing.T) {
	isTerminal = func(io.Writer) bool { return true }
	defer restoreIsTerminal()
	out := captureStderr(t, func() {
		NewConsoleLogger().Warn("caution")
	})
	if !strings.Contains(out, "[!]") || !strings.Contains(out, "caution") {
		t.Errorf("expected [!] and caution in output, got %q", out)
	}
}

func TestConsoleLogger_TTY_Prompt(t *testing.T) {
	isTerminal = func(io.Writer) bool { return true }
	defer restoreIsTerminal()
	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdin = r
	_, _ = w.WriteString("y\n")
	w.Close()

	out := captureStderr(t, func() {
		NewConsoleLogger().Prompt("continue?")
	})
	os.Stdin = oldStdin

	if !strings.Contains(out, "[?]") || !strings.Contains(out, "continue?") {
		t.Errorf("expected [?] and continue? in output, got %q", out)
	}
}

func TestConsoleLogger_ConcurrentSafety(t *testing.T) {
	logger := NewConsoleLogger()
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			logger.Info("goroutine %d", n)
		}(i)
	}
	wg.Wait()
}
