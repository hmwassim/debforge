package testutil

import (
	"context"

	"github.com/hmwassim/debforge/internal/ports"
)

type RunFunc func(ctx context.Context, name string, args ...string) ([]byte, []byte, error)

type RunWithOptionsFunc func(ctx context.Context, opts ports.RunOptions, name string, args ...string) ([]byte, []byte, error)

type MockRunner struct {
	RunFunc          RunFunc
	RunWithOptionsFunc RunWithOptionsFunc
}

func (m *MockRunner) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	return m.RunFunc(ctx, name, args...)
}

func (m *MockRunner) RunWithOptions(ctx context.Context, opts ports.RunOptions, name string, args ...string) ([]byte, []byte, error) {
	if m.RunWithOptionsFunc != nil {
		return m.RunWithOptionsFunc(ctx, opts, name, args...)
	}
	return m.RunFunc(ctx, name, args...)
}

// RunnerReturning creates a MockRunner that returns the given stdout and
// error for every invocation. Tests that need per-command routing should
// still use MockRunner directly with a custom RunFunc.
func RunnerReturning(stdout []byte, err error) *MockRunner {
	return &MockRunner{
		RunFunc: func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
			return stdout, nil, err
		},
	}
}
