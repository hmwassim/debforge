package aptpty

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/hmwassim/debforge/internal/testutil"
)

// delayedReader wraps a mockPtySession and adds a per-Read delay so the
// main loop's ticker branch (case <-ticker.C) gets exercised.
type delayedReader struct {
	mockPtySession
	delay time.Duration
}

func (m *delayedReader) Read(b []byte) (int, error) {
	time.Sleep(m.delay)
	return m.mockPtySession.Read(b)
}

func TestRunWithSession_normal(t *testing.T) {
	spinner := &testutil.MockSpinner{}
	mock := &mockPtySession{
		data: []byte("Need to get 123 kB of archives.\n" +
			" 42% [1 hello 52.0 kB/123 kB 42%]\n" +
			"Fetched 123 kB in 0s (456 kB/s)\n" +
			"Setting up hello (2.36-9)\n"),
		readErr: io.EOF,
	}
	err := runWithSession(context.Background(), testutil.RunnerReturning(nil, nil),
		[]string{"install", "-y", "hello"}, spinner, mockPtyFactory(mock))
	if err != nil {
		t.Fatalf("runWithSession: %v", err)
	}
	if spinner.Desc == "" {
		t.Error("expected spinner.SetDesc to be called")
	}
}

func TestRunWithSession_spinnerNil(t *testing.T) {
	err := runWithSession(context.Background(), nil, []string{"install", "-y", "hello"}, nil, nil)
	if err == nil {
		t.Fatal("expected error for nil spinner")
	}
}

func TestRunWithSession_waitError(t *testing.T) {
	spinner := &testutil.MockSpinner{}
	mock := &mockPtySession{
		data:    []byte("E: Sub-process /usr/bin/dpkg returned an error\n"),
		readErr: io.EOF,
		waitErr: errors.New("exit status 1"),
	}
	err := runWithSession(context.Background(), testutil.RunnerReturning(nil, nil),
		[]string{"install", "-y", "hello"}, spinner, mockPtyFactory(mock))
	if err == nil {
		t.Fatal("expected error from Wait")
	}
}

func TestRunWithSession_waitErrWithMessages(t *testing.T) {
	spinner := &testutil.MockSpinner{}
	mock := &mockPtySession{
		data:    []byte("E: Sub-process /usr/bin/dpkg returned an error (1)\n"),
		readErr: io.EOF,
		waitErr: errors.New("exit status 100"),
	}
	err := runWithSession(context.Background(), testutil.RunnerReturning(nil, nil),
		[]string{"install", "-y", "hello"}, spinner, mockPtyFactory(mock))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRunWithSession_aptErrsCollected(t *testing.T) {
	spinner := &testutil.MockSpinner{}
	mock := &mockPtySession{
		data: []byte("W: Some issues\n" +
			"dpkg: error processing foo\n" +
			"E: Sub-process returned error\n"),
		readErr: io.EOF,
		waitErr: errors.New("exit status 100"),
	}
	err := runWithSession(context.Background(), testutil.RunnerReturning(nil, nil),
		[]string{"install", "-y", "hello"}, spinner, mockPtyFactory(mock))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRunWithSession_contextCancel(t *testing.T) {
	spinner := &testutil.MockSpinner{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	mock := &mockPtySession{
		data:    []byte("some output\n"),
		readErr: io.EOF,
	}
	err := runWithSession(ctx, testutil.RunnerReturning(nil, nil),
		[]string{"install", "-y", "hello"}, spinner, mockPtyFactory(mock))
	if err != nil {
		t.Fatalf("expected nil after cancel, got %v", err)
	}
}

func TestRunWithSession_contextCancelMidRun(t *testing.T) {
	spinner := &testutil.MockSpinner{}
	ctx, cancel := context.WithCancel(context.Background())
	mock := &mockPtySession{
		data:    []byte("Need to get 1,024 kB of archives.\n 10% [1 pkg 100 kB/1,024 kB 10%]\n"),
		readErr: io.EOF,
	}
	cancel()
	err := runWithSession(ctx, testutil.RunnerReturning(nil, nil),
		[]string{"install", "-y", "pkg"}, spinner, mockPtyFactory(mock))
	if err != nil {
		t.Fatalf("expected nil after mid-run cancel, got %v", err)
	}
}

func TestRunWithSession_downloadPhase(t *testing.T) {
	spinner := &testutil.MockSpinner{}
	mock := &mockPtySession{
		data: []byte("Need to get 1,024 kB of archives.\n" +
			" 20% [1 pkg 200 kB/1,024 kB 20%]\n" +
			" 80% [1 pkg 800 kB/1,024 kB 80%]\n" +
			"Fetched 1,024 kB in 1s (1,024 kB/s)\n" +
			"Setting up pkg (1.0-1)\n"),
		readErr: io.EOF,
	}
	err := runWithSession(context.Background(), testutil.RunnerReturning(nil, nil),
		[]string{"install", "-y", "pkg"}, spinner, mockPtyFactory(mock))
	if err != nil {
		t.Fatalf("runWithSession: %v", err)
	}
}

func TestRunWithSession_emptyArgs(t *testing.T) {
	spinner := &testutil.MockSpinner{}
	mock := &mockPtySession{
		data:    []byte("\n"),
		readErr: io.EOF,
	}
	err := runWithSession(context.Background(), testutil.RunnerReturning(nil, nil),
		[]string{}, spinner, mockPtyFactory(mock))
	if err != nil {
		t.Fatalf("runWithSession with empty args: %v", err)
	}
}

func TestRunWithSession_readError(t *testing.T) {
	spinner := &testutil.MockSpinner{}
	mock := &mockPtySession{
		readErr: io.ErrUnexpectedEOF,
	}
	err := runWithSession(context.Background(), testutil.RunnerReturning(nil, nil),
		[]string{"install", "-y", "pkg"}, spinner, mockPtyFactory(mock))
	if err != nil {
		t.Fatalf("expected nil on read error (non-EOF breaks but doesn't error), got %v", err)
	}
}

func TestRunWithSession_multilineProgress(t *testing.T) {
	spinner := &testutil.MockSpinner{}
	data := []byte("Need to get 2,000 kB of archives.\n" +
		" 10% [1 pkg-a 200 kB/1,000 kB 20%]\r" +
		" 50% [1 pkg-a 500 kB/1,000 kB 50%]\n" +
		"Fetched 1,000 kB in 0s (2,000 kB/s)\n" +
		"Setting up pkg-a (2.0-1)\n")
	mock := &mockPtySession{
		data:    data,
		readErr: io.EOF,
	}
	err := runWithSession(context.Background(), testutil.RunnerReturning(nil, nil),
		[]string{"install", "-y", "pkg-a"}, spinner, mockPtyFactory(mock))
	if err != nil {
		t.Fatalf("runWithSession: %v", err)
	}
}

func TestRunWithSession_binaryExistsVerification(t *testing.T) {
	spinner := &testutil.MockSpinner{}
	mock := &mockPtySession{
		data:    []byte("Fetched 100 kB in 0s (1,000 kB/s)\n"),
		readErr: io.EOF,
	}
	err := runWithSession(context.Background(), testutil.RunnerReturning(nil, nil),
		[]string{"install", "-y", "pkg"}, spinner, mockPtyFactory(mock))
	if err != nil {
		t.Fatalf("runWithSession: %v", err)
	}
	if spinner.Desc == "" {
		t.Error("expected spinner SetDesc to be called")
	}
}

func TestRunWithSession_tickerFires(t *testing.T) {
	spinner := &testutil.MockSpinner{}
	mock := &delayedReader{
		mockPtySession: mockPtySession{
			data: []byte("Need to get 1,024 kB of archives.\n" +
				"Fetched 1,024 kB in 1s (1,024 kB/s)\n" +
				"Setting up hello (1.0-1)\n"),
			readErr: io.EOF,
		},
		delay: 200 * time.Millisecond,
	}
	err := runWithSession(context.Background(), testutil.RunnerReturning(nil, nil),
		[]string{"install", "-y", "hello"}, spinner, mockPtyFactory(mock))
	if err != nil {
		t.Fatalf("runWithSession: %v", err)
	}
	if spinner.Desc == "" {
		t.Error("expected spinner SetDesc to be called")
	}
}

func TestRunWithSession_exitsPromptly(t *testing.T) {
	spinner := &testutil.MockSpinner{}
	mock := &mockPtySession{
		data:    []byte("Reading package lists... Done\n"),
		readErr: io.EOF,
	}
	done := make(chan error, 1)
	go func() {
		done <- runWithSession(context.Background(), testutil.RunnerReturning(nil, nil),
			[]string{"full-upgrade", "-y"}, spinner, mockPtyFactory(mock))
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runWithSession: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runWithSession did not exit promptly — likely hanging on wg.Wait()")
	}
}

func TestRunWithSession_lineLogCapturesTranscript(t *testing.T) {
	var captured []string
	LineLog = func(line string) { captured = append(captured, line) }
	defer func() { LineLog = nil }()

	spinner := &testutil.MockSpinner{}
	mock := &mockPtySession{
		data: []byte("Reading package lists... Done\n" +
			" 42% [1 hello 52.0 kB/123 kB 42%]\n" +
			"Setting up hello (2.36-9) ...\n" +
			"dpkg: error processing package hello (--configure):\n" +
			" dependency Problem3\n" +
			"E: Sub-process /usr/bin/dpkg returned an error code (1)\n"),
		readErr: io.EOF,
		waitErr: errors.New("exit status 100"),
	}
	err := runWithSession(context.Background(), testutil.RunnerReturning(nil, nil),
		[]string{"install", "-y", "hello"}, spinner, mockPtyFactory(mock))
	if err == nil {
		t.Fatal("expected error")
	}

	wantContains := []string{
		"Reading package lists... Done",
		"Setting up hello",
		"dpkg: error processing",
		"dependency Problem3",
		"E: Sub-process",
	}
	for _, w := range wantContains {
		found := false
		for _, c := range captured {
			if strings.Contains(c, w) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("captured lines missing %q\ngot: %v", w, captured)
		}
	}

	for _, c := range captured {
		if strings.Contains(c, "kB/") {
			t.Errorf("captured line contains download meter %q", c)
		}
	}
}
