package ui

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func TestNewConsoleSpinner(t *testing.T) {
	var buf bytes.Buffer
	ctx := context.Background()

	s := NewConsoleSpinner(ctx, &buf, "testing spinner")
	if s == nil {
		t.Fatal("expected spinner to be created")
	}
}

func TestSpinnerDone(t *testing.T) {
	var buf bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := NewConsoleSpinner(ctx, &buf, "test")
	if s == nil {
		t.Fatal("expected spinner")
	}
	time.Sleep(50 * time.Millisecond)
	s.Done()
}

func TestSpinnerFail(t *testing.T) {
	var buf bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := NewConsoleSpinner(ctx, &buf, "test")
	if s == nil {
		t.Fatal("expected spinner")
	}
	time.Sleep(50 * time.Millisecond)
	s.Fail()
}

func TestSpinnerPauseResume(t *testing.T) {
	var buf bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := NewConsoleSpinner(ctx, &buf, "test")

	s.Pause()
	s.Resume()
	s.Done()
}

func TestSpinnerContextCancelled(t *testing.T) {
	var buf bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())

	s := NewConsoleSpinner(ctx, &buf, "test")
	cancel()

	// Give it time to process cancellation
	time.Sleep(200 * time.Millisecond)
	s.Done()
}

func TestSpinnerNoTTY(t *testing.T) {
	var buf bytes.Buffer
	ctx := context.Background()

	s := NewConsoleSpinner(ctx, &buf, "test")
	s.Done()
	// Should work without color/terminal
}

func TestSpinnerPauseResumeFlag(t *testing.T) {
	var buf bytes.Buffer
	ctx := context.Background()

	s := NewConsoleSpinner(ctx, &buf, "test")

	s.Pause()
	if !s.paused {
		t.Error("expected paused after Pause()")
	}

	s.Resume()
	if s.paused {
		t.Error("expected not paused after Resume()")
	}

	s.Pause()
	s.Done()
	if s.paused {
		t.Error("expected paused cleared after Done()")
	}
}

func TestSpinnerPauseResumeOutput(t *testing.T) {
	var buf bytes.Buffer
	ctx := context.Background()

	s := NewConsoleSpinner(ctx, &buf, "pause-output")

	s.Pause()
	s.Resume()
	s.Done()

	// Non-TTY output: Done writes "[*] description\n"
	want := "[*] pause-output\n"
	if got := buf.String(); got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}
