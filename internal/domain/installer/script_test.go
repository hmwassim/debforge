package installer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/testutil"
)

func TestRunScript_surfacesStderr(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, []byte("configure: error: missing libfoo\n"), errors.New("exit status 1")
		},
	}

	spinner := &mockSpinner{}
	err := RunScript(context.Background(), runner, spinner, "test-pkg", "./configure", "configuring")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "missing libfoo") {
		t.Errorf("error should contain stderr output, got: %v", err)
	}
}

func TestRunScript_noStderr(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, nil, errors.New("exit status 1")
		},
	}

	spinner := &mockSpinner{}
	err := RunScript(context.Background(), runner, spinner, "test-pkg", "./fail", "testing")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if strings.Contains(err.Error(), ": :") {
		t.Errorf("error should not have empty stderr suffix: %v", err)
	}
}

func TestRunScript_truncatesLongStderr(t *testing.T) {
	long := strings.Repeat("x", 1000)
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, []byte(long), errors.New("exit status 1")
		},
	}

	spinner := &mockSpinner{}
	err := RunScript(context.Background(), runner, spinner, "test-pkg", "./fail", "testing")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !strings.HasSuffix(msg, "...") {
		t.Errorf("long stderr should be truncated ending with ..., got: %v", msg)
	}
	if len(msg) > 600 {
		t.Errorf("error message too long (%d chars), should be truncated", len(msg))
	}
}

type mockSpinner struct {
	desc string
}

func (m *mockSpinner) Done()                  {}
func (m *mockSpinner) Fail()                  {}
func (m *mockSpinner) DoneWarn()              {}
func (m *mockSpinner) DoneInfo()              {}
func (m *mockSpinner) Pause()                 {}
func (m *mockSpinner) Resume()                {}
func (m *mockSpinner) SetDesc(d string)       { m.desc = d }
var _ interface{ SetDesc(string) } = (*mockSpinner)(nil)
