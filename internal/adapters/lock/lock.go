// Package lock provides adapters for ports.Locker. Every other port
// (FileSystem, CommandRunner, UI) has its concrete implementation under
// internal/adapters/...; Locker previously did not - its only
// implementation was a "noopLocker" struct defined and instantiated
// directly inside cmd/debforge/main.go. It now lives here instead, beside
// its sibling adapters.
package lock

import "context"

// Noop is a ports.Locker that never actually locks anything. debforge does
// not yet have a real cross-process lock (e.g. flock-based) implementation;
// Noop exists so the rest of the codebase can depend on ports.Locker today
// and swap in a real implementation later without any other call site
// changing.
type Noop struct{}

func NewNoop() *Noop {
	return &Noop{}
}

func (l *Noop) Acquire(ctx context.Context, path string) (func(), error) {
	return func() {}, nil
}
