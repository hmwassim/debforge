package service

import (
	"context"
	"os"
	"testing"

	"github.com/hmwassim/debforge/internal/adapters/fs"
	"github.com/hmwassim/debforge/internal/adapters/store"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

type mockSpinner struct{ desc string }

func (m *mockSpinner) Done()            {}
func (m *mockSpinner) Fail()            {}
func (m *mockSpinner) DoneWarn()        {}
func (m *mockSpinner) DoneInfo()        {}
func (m *mockSpinner) Pause()           {}
func (m *mockSpinner) Resume()          {}
func (m *mockSpinner) SetDesc(d string) { m.desc = d }

type variantRecorder struct {
	variants   []string
	forceFlags []bool
}

func (r *variantRecorder) Install(_ context.Context, p *pkg.Package, _ ports.Spinner) error {
	r.forceFlags = append(r.forceFlags, p.ForceInstall)
	if p.Apt != nil {
		r.variants = append(r.variants, p.Apt.Variant)
	}
	return nil
}

func (r *variantRecorder) Remove(_ context.Context, _ *pkg.Package, _ ports.Spinner) error {
	return nil
}

type nopRunner struct{}

func (n *nopRunner) Run(_ context.Context, _ string, _ ...string) (stdout, stderr []byte, err error) {
	return nil, nil, nil
}
func (n *nopRunner) RunWithOptions(_ context.Context, _ ports.RunOptions, _ string, _ ...string) (stdout, stderr []byte, err error) {
	return nil, nil, nil
}

type successRunner struct{}

func (r *successRunner) Run(_ context.Context, _ string, _ ...string) (stdout, stderr []byte, err error) {
	return []byte("installed\n"), nil, nil
}
func (r *successRunner) RunWithOptions(_ context.Context, _ ports.RunOptions, _ string, _ ...string) (stdout, stderr []byte, err error) {
	return []byte("installed\n"), nil, nil
}

func newStateManagerForTest(t *testing.T) (*StateManager, string, func()) {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "debforge-test-*.json")
	if err != nil {
		t.Fatalf("create temp state: %v", err)
	}
	tmpFile.Close()
	stateStore := store.NewStore[State](fs.NewFileSystem(), tmpFile.Name())
	stateSvc := NewStateManager(stateStore)
	return stateSvc, tmpFile.Name(), func() { os.Remove(tmpFile.Name()) }
}
