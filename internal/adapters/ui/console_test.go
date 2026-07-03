package ui

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
)

func TestConsoleUI_SetYes_Prompt(t *testing.T) {
	u := NewConsoleUI()
	u.SetYes(true)
	if !u.Prompt("Continue?") {
		t.Error("expected yes-mode Prompt to return true immediately")
	}
}

func TestConsoleUI_SetYes_PromptInput(t *testing.T) {
	u := NewConsoleUI()
	u.SetYes(true)
	got := u.PromptInput("default", "Enter value")
	if got != "default" {
		t.Errorf("expected %q, got %q", "default", got)
	}
}

func TestConsoleUI_PromptInput_yesModeIgnoresSpinner(t *testing.T) {
	u := NewConsoleUI()
	u.SetYes(true)
	u.currentSpinner = &Display{}
	got := u.PromptInput("default", "prompt")
	if got != "default" {
		t.Errorf("expected %q, got %q", "default", got)
	}
}

func TestConsoleUI_Prompt_yesModeIgnoresSpinner(t *testing.T) {
	u := NewConsoleUI()
	u.SetYes(true)
	u.currentSpinner = &Display{}
	if !u.Prompt("Continue?") {
		t.Error("expected yes-mode Prompt to return true")
	}
}

func TestConsoleUI_Spinner(t *testing.T) {
	u := NewConsoleUI()
	s := u.Spinner(context.Background(), "testing")
	if s == nil {
		t.Fatal("Spinner should not return nil")
	}
	if u.currentSpinner == nil {
		t.Error("currentSpinner should be set")
	}
}

func TestConsoleUI_Info(t *testing.T) {
	out := captureStderr(t, func() {
		u := NewConsoleUI()
		u.Info("hello %s", "world")
	})
	if !strings.Contains(out, "[i] hello world") {
		t.Errorf("expected info output, got %q", out)
	}
}

func TestConsoleUI_Success(t *testing.T) {
	out := captureStderr(t, func() {
		u := NewConsoleUI()
		u.Success("done %s", "ok")
	})
	if !strings.Contains(out, "[*] done ok") {
		t.Errorf("expected success output, got %q", out)
	}
}

func TestConsoleUI_Warn(t *testing.T) {
	out := captureStderr(t, func() {
		u := NewConsoleUI()
		u.Warn("warning: %s", "low disk")
	})
	if !strings.Contains(out, "[!] warning: low disk") {
		t.Errorf("expected warn output, got %q", out)
	}
}

func TestConsoleUI_Error(t *testing.T) {
	out := captureStderr(t, func() {
		u := NewConsoleUI()
		u.Error("error: %v", "boom")
	})
	if !strings.Contains(out, "[x] error: boom") {
		t.Errorf("expected error output, got %q", out)
	}
}

func TestConsoleUI_InfoWithSpinner(t *testing.T) {
	var buf bytes.Buffer
	u := NewConsoleUI()
	u.Spinner(context.Background(), "working")
	out := captureStderr(t, func() {
		u.Info("message")
	})
	if !strings.Contains(out, "[i] message") {
		t.Errorf("expected info output, got %q", out)
	}
	_ = buf
}

func TestConsoleUI_PromptInput_nonYes(t *testing.T) {
	old := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdin = r

	w.WriteString("user_input\n")
	w.Close()

	u := NewConsoleUI()
	got := u.PromptInput("default", "Enter name")
	os.Stdin = old

	if got != "user_input" {
		t.Errorf("expected %q, got %q", "user_input", got)
	}
}

func TestConsoleUI_Prompt_nonYes(t *testing.T) {
	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdin = r
	w.WriteString("y\n")
	w.Close()

	out := captureStderr(t, func() {
		u := NewConsoleUI()
		if !u.Prompt("continue?") {
			t.Error("expected true for y")
		}
	})
	os.Stdin = oldStdin

	if !strings.Contains(out, "[?] continue? [y/N]") {
		t.Errorf("expected prompt output, got %q", out)
	}
}

func TestConsoleUI_Prompt_nonYesWithSpinner(t *testing.T) {
	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdin = r
	w.WriteString("n\n")
	w.Close()

	out := captureStderr(t, func() {
		u := NewConsoleUI()
		u.Spinner(context.Background(), "working")
		if u.Prompt("continue?") {
			t.Error("expected false for n")
		}
	})
	os.Stdin = oldStdin

	if !strings.Contains(out, "[?] continue? [y/N]") {
		t.Errorf("expected prompt output, got %q", out)
	}
}

func TestNewConsoleUI(t *testing.T) {
	u := NewConsoleUI()
	if u.logger == nil {
		t.Error("logger should be initialized")
	}
	if u.yes {
		t.Error("yes should default to false")
	}
	if u.currentSpinner != nil {
		t.Error("currentSpinner should default to nil")
	}
}

func TestConsoleUI_withSpinnerPaused_noSpinner(t *testing.T) {
	u := NewConsoleUI()
	called := false
	u.withSpinnerPaused(func() {
		called = true
	})
	if !called {
		t.Error("function should be called even without a spinner")
	}
}

func TestConsoleUI_withSpinnerPaused_withSpinner(t *testing.T) {
	var buf bytes.Buffer
	u := NewConsoleUI()
	u.Spinner(context.Background(), "working")
	called := false
	u.withSpinnerPaused(func() {
		called = true
	})
	if !called {
		t.Error("function should be called with spinner paused")
	}
	_ = buf
}
