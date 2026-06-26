package testutil

import (
	"context"

	"github.com/hmwassim/debforge/internal/ports"
)

type RunFunc func(ctx context.Context, name string, args ...string) ([]byte, []byte, error)

type MockRunner struct {
	RunFunc RunFunc
}

func (m *MockRunner) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	return m.RunFunc(ctx, name, args...)
}

func (m *MockRunner) RunWithOptions(ctx context.Context, _ ports.RunOptions, name string, args ...string) ([]byte, []byte, error) {
	return m.RunFunc(ctx, name, args...)
}
