package lock

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireContended(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "debforge.lock")
	l1, err := Acquire(context.Background(), p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if Holder(p) == "" {
		t.Fatal("pid not recorded")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	waited := false
	if _, err := Acquire(ctx, p, func() { waited = true }); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second acquire err = %v", err)
	}
	if !waited {
		t.Fatal("onWait not called")
	}

	if err := l1.Release(); err != nil {
		t.Fatal(err)
	}
	l2, err := Acquire(context.Background(), p, nil)
	if err != nil {
		t.Fatalf("after release: %v", err)
	}
	_ = l2.Release()
}
