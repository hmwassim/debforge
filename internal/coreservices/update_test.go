package services

import (
	"context"
	"errors"
	"testing"

	"github.com/hmwassim/debforge/internal/services/self"
)

type mockUpdater struct {
	updateErr error
}

func (m *mockUpdater) Update(ctx context.Context) error {
	return m.updateErr
}

var _ self.SelfUpdater = (*mockUpdater)(nil)

func TestUpdateServiceRun(t *testing.T) {
	updater := &mockUpdater{}
	svc := NewUpdateService(updater, &mockUI{})
	ctx := context.Background()

	err := svc.Run(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateServiceRunError(t *testing.T) {
	updater := &mockUpdater{updateErr: errors.New("update failed")}
	svc := NewUpdateService(updater, &mockUI{})
	ctx := context.Background()

	err := svc.Run(ctx)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "update failed" {
		t.Fatalf("expected 'update failed', got %v", err)
	}
}

type trackingUpdater struct {
	*mockUpdater
	called    bool
	passedCtx context.Context
}

func (t *trackingUpdater) Update(ctx context.Context) error {
	t.called = true
	t.passedCtx = ctx
	return t.mockUpdater.Update(ctx)
}

func TestUpdateServicePassesContext(t *testing.T) {
	tracker := &trackingUpdater{mockUpdater: &mockUpdater{}}
	svc := NewUpdateService(tracker, &mockUI{})
	ctx := context.Background()

	err := svc.Run(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !tracker.called {
		t.Fatal("Update not called")
	}
	if tracker.passedCtx != ctx {
		t.Fatal("context not passed correctly")
	}
}
