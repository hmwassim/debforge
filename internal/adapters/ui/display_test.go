package ui

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"golang.org/x/term"
)

func TestDisplay_NonTTY_Done(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(context.Background(), &buf, "working", nil)
	d.Done()
	out := buf.String()

	if !strings.Contains(out, "[*] Working") {
		t.Errorf("expected [*] output, got %q", out)
	}
}

func TestDisplay_NonTTY_Fail(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(context.Background(), &buf, "processing", nil)
	d.Fail()
	out := buf.String()

	if !strings.Contains(out, "[x] Processing") {
		t.Errorf("expected [x] output, got %q", out)
	}
}

func TestDisplay_NonTTY_DoneWarn(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(context.Background(), &buf, "checking", nil)
	d.DoneWarn()
	out := buf.String()

	if !strings.Contains(out, "[!] Checking") {
		t.Errorf("expected [!] output, got %q", out)
	}
}

func TestDisplay_NonTTY_DoneInfo(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(context.Background(), &buf, "syncing", nil)
	d.DoneInfo()
	out := buf.String()

	if !strings.Contains(out, "[i] Syncing") {
		t.Errorf("expected [i] output, got %q", out)
	}
}

func TestDisplay_NonTTY_Stop(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(context.Background(), &buf, "working", nil)
	d.Stop()
	out := buf.String()

	if out != "[i] Working\n" {
		t.Errorf("expected only init line, got %q", out)
	}
}

func TestDisplay_NonTTY_SetDesc(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(context.Background(), &buf, "working", nil)
	d.SetDesc("updated task")
	d.Done()
	out := buf.String()

	if !strings.Contains(out, "Updated task") {
		t.Errorf("expected updated desc, got %q", out)
	}
}

func TestDisplay_DoneMultipleCalls(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(context.Background(), &buf, "working", nil)
	d.Done()
	first := buf.String()
	d.Done()
	second := buf.String()

	if first != second {
		t.Error("second Done should not produce additional output")
	}
}

func TestDisplay_StopMultipleCalls(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(context.Background(), &buf, "working", nil)
	d.Stop()
	first := buf.String()
	d.Stop()
	second := buf.String()

	if first != second {
		t.Error("second Stop should not produce additional output")
	}
}

func TestDisplay_NonTTY_StripTrailingDots(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(context.Background(), &buf, "loading...", nil)
	d.Done()
	out := buf.String()

	// Initial line preserves dots, final doneWith line strips them
	if !strings.Contains(out, "[i] Loading...\n") {
		t.Errorf("expected initial line with dots, got %q", out)
	}
	if !strings.Contains(out, "[*] Loading\n") {
		t.Errorf("expected final line without dots, got %q", out)
	}
}

func TestDisplay_PauseResumeNonTTY(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(context.Background(), &buf, "working", nil)
	d.Pause()
	d.Resume()
	d.Done()
	out := buf.String()

	if !strings.Contains(out, "[*] Working") {
		t.Errorf("expected final output, got %q", out)
	}
}

func TestDisplay_DoneAfterStop(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(context.Background(), &buf, "working", nil)
	d.Stop()
	buf.Reset()
	d.Done()
	if buf.Len() > 0 {
		t.Error("Done after Stop should not produce output")
	}
}

func TestDisplay_StopAfterDone(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(context.Background(), &buf, "working", nil)
	d.Done()
	buf.Reset()
	d.Stop()
	if buf.Len() > 0 {
		t.Error("Stop after Done should not produce output")
	}
}

func TestDisplay_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var buf bytes.Buffer
	d := NewDisplay(ctx, &buf, "working", nil)
	cancel()
	d.Done()

	if !strings.Contains(buf.String(), "[*] Working") {
		t.Errorf("expected final output after ctx cancel, got %q", buf.String())
	}
}

func TestDisplay_PauseAfterDone(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(context.Background(), &buf, "working", nil)
	d.Done()
	d.Pause()
}

func TestDisplay_ResumeAfterDone(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(context.Background(), &buf, "working", nil)
	d.Done()
	d.Resume()
}

func restoreIsTerminal() {
	isTerminal = func(w io.Writer) bool {
		if f, ok := w.(*os.File); ok {
			return term.IsTerminal(int(f.Fd()))
		}
		return false
	}
}

func TestDisplay_TTY_Done(t *testing.T) {
	isTerminal = func(io.Writer) bool { return true }
	defer restoreIsTerminal()
	var buf bytes.Buffer
	d := NewDisplay(context.Background(), &buf, "working", nil)
	d.Done()
	out := buf.String()
	if !strings.Contains(out, "[*]") {
		t.Errorf("expected [*] in output, got %q", out)
	}
	if !strings.Contains(out, "Working") {
		t.Errorf("expected Working in output, got %q", out)
	}
}

func TestDisplay_TTY_Fail(t *testing.T) {
	isTerminal = func(io.Writer) bool { return true }
	defer restoreIsTerminal()
	var buf bytes.Buffer
	d := NewDisplay(context.Background(), &buf, "processing", nil)
	d.Fail()
	out := buf.String()
	if !strings.Contains(out, "[x]") {
		t.Errorf("expected [x] in output, got %q", out)
	}
	if !strings.Contains(out, "Processing") {
		t.Errorf("expected Processing in output, got %q", out)
	}
}

func TestDisplay_TTY_DoneWarn(t *testing.T) {
	isTerminal = func(io.Writer) bool { return true }
	defer restoreIsTerminal()
	var buf bytes.Buffer
	d := NewDisplay(context.Background(), &buf, "checking", nil)
	d.DoneWarn()
	out := buf.String()
	if !strings.Contains(out, "[!]") {
		t.Errorf("expected [!] in output, got %q", out)
	}
	if !strings.Contains(out, "Checking") {
		t.Errorf("expected Checking in output, got %q", out)
	}
}

func TestDisplay_TTY_DoneInfo(t *testing.T) {
	isTerminal = func(io.Writer) bool { return true }
	defer restoreIsTerminal()
	var buf bytes.Buffer
	d := NewDisplay(context.Background(), &buf, "syncing", nil)
	d.DoneInfo()
	out := buf.String()
	if !strings.Contains(out, "[i]") {
		t.Errorf("expected [i] in output, got %q", out)
	}
	if !strings.Contains(out, "Syncing") {
		t.Errorf("expected Syncing in output, got %q", out)
	}
}

func TestDisplay_TTY_Stop(t *testing.T) {
	isTerminal = func(io.Writer) bool { return true }
	defer restoreIsTerminal()
	var buf bytes.Buffer
	d := NewDisplay(context.Background(), &buf, "working", nil)
	d.Stop()
	out := buf.String()
	// TTY Stop doesn't print a final marker, just removes the animation
	if !strings.Contains(out, "Working") {
		t.Errorf("expected Working in output, got %q", out)
	}
}

func TestDisplay_TTY_PauseResume(t *testing.T) {
	isTerminal = func(io.Writer) bool { return true }
	defer restoreIsTerminal()
	var buf bytes.Buffer
	d := NewDisplay(context.Background(), &buf, "working", nil)
	d.Pause()
	d.Resume()
	d.Done()
	out := buf.String()
	if !strings.Contains(out, "[*]") {
		t.Errorf("expected [*] in output, got %q", out)
	}
	if !strings.Contains(out, "Working") {
		t.Errorf("expected Working in output, got %q", out)
	}
}

func TestDisplay_TTY_ContextCancelled(t *testing.T) {
	isTerminal = func(io.Writer) bool { return true }
	defer restoreIsTerminal()
	ctx, cancel := context.WithCancel(context.Background())
	var buf bytes.Buffer
	d := NewDisplay(ctx, &buf, "working", nil)
	cancel()
	d.Done()
	out := buf.String()
	if !strings.Contains(out, "[*]") {
		t.Errorf("expected [*] in output, got %q", out)
	}
	if !strings.Contains(out, "Working") {
		t.Errorf("expected Working in output, got %q", out)
	}
}

func TestDisplay_TTY_SetDesc(t *testing.T) {
	isTerminal = func(io.Writer) bool { return true }
	defer restoreIsTerminal()
	var buf bytes.Buffer
	d := NewDisplay(context.Background(), &buf, "working", nil)
	d.SetDesc("updated task")
	d.Done()
	out := buf.String()
	if !strings.Contains(out, "Updated task") {
		t.Errorf("expected Updated task in output, got %q", out)
	}
}

func TestDisplay_StripTrailingDots(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"working", "working"},
		{"loading...", "loading"},
		{"testing.....", "testing"},
		{"...", ""},
	}
	for _, tc := range tests {
		got := stripTrailingDots(tc.input)
		if got != tc.want {
			t.Errorf("stripTrailingDots(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
