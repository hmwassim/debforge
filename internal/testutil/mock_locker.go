package testutil

import "context"

// MockLocker is a ports.Locker test double that never actually locks
// anything but records whether Acquire/the release func were called, so
// tests can assert lock/unlock sequencing without real file locking.
type MockLocker struct {
	AcquireErr   error
	AcquireCount int
	ReleaseCount int
}

func (m *MockLocker) Acquire(_ context.Context, _ string) (func(), error) {
	m.AcquireCount++
	if m.AcquireErr != nil {
		return nil, m.AcquireErr
	}
	return func() { m.ReleaseCount++ }, nil
}
