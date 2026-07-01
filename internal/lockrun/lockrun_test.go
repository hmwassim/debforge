package lockrun

import (
	"context"
	"errors"
	"testing"

	"github.com/hmwassim/debforge/internal/testutil"
)

func TestWithLock_runsFnBetweenAcquireAndRelease(t *testing.T) {
	locker := &testutil.MockLocker{}
	ran := false

	err := WithLock(context.Background(), locker, "/tmp/whatever", func() error {
		ran = true
		if locker.AcquireCount != 1 || locker.ReleaseCount != 0 {
			t.Errorf("expected fn to run after acquire but before release, got acquire=%d release=%d", locker.AcquireCount, locker.ReleaseCount)
		}
		return nil
	})

	if err != nil {
		t.Fatalf("WithLock: %v", err)
	}
	if !ran {
		t.Error("expected fn to run")
	}
	if locker.ReleaseCount != 1 {
		t.Errorf("expected release to be called exactly once, got %d", locker.ReleaseCount)
	}
}

func TestWithLock_releasesEvenWhenFnFails(t *testing.T) {
	locker := &testutil.MockLocker{}
	wantErr := errors.New("fn failed")

	err := WithLock(context.Background(), locker, "/tmp/whatever", func() error {
		return wantErr
	})

	if !errors.Is(err, wantErr) {
		t.Errorf("expected WithLock to propagate fn's error, got %v", err)
	}
	if locker.ReleaseCount != 1 {
		t.Errorf("expected release to still be called once on fn failure, got %d", locker.ReleaseCount)
	}
}

func TestWithLock_acquireFailurePreventsFnAndWrapsError(t *testing.T) {
	wantErr := errors.New("acquire failed")
	locker := &testutil.MockLocker{AcquireErr: wantErr}
	called := false

	err := WithLock(context.Background(), locker, "/tmp/whatever", func() error {
		called = true
		return nil
	})

	if called {
		t.Error("expected fn not to run when Acquire fails")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("expected error to wrap the acquire error, got %v", err)
	}
}

func TestWithLock_cancelledContext(t *testing.T) {
	locker := &testutil.MockLocker{AcquireErr: context.Canceled}
	called := false

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := WithLock(ctx, locker, "/tmp/whatever", func() error {
		called = true
		return nil
	})

	if called {
		t.Error("expected fn not to run when context is cancelled")
	}
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}
