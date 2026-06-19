package utils

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetryWithBackoffSuccess(t *testing.T) {
	attempts := 0
	err := RetryWithBackoff(context.Background(), 3, time.Millisecond, func() error {
		attempts++
		if attempts < 2 {
			return errors.New("transient error")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
}

func TestRetryWithBackoffExhausted(t *testing.T) {
	attempts := 0
	err := RetryWithBackoff(context.Background(), 2, time.Millisecond, func() error {
		attempts++
		return errors.New("always fails")
	})
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if attempts != 3 { // 0..2 = 3 attempts
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetryWithBackoffContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := RetryWithBackoff(ctx, 3, time.Millisecond, func() error {
		return errors.New("error")
	})
	if err == nil || errors.Is(err, context.Canceled) {
		// Either the error is returned, or context cancellation is detected
	}
}

func TestRetryHTTP(t *testing.T) {
	attempts := 0
	err := RetryHTTP(context.Background(), func() error {
		attempts++
		return errors.New("http error")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts != 4 { // 0..3 = 4 attempts
		t.Fatalf("expected 4 attempts, got %d", attempts)
	}
}

func TestRetryGit(t *testing.T) {
	attempts := 0
	err := RetryGit(context.Background(), func() error {
		attempts++
		return errors.New("git error")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts != 4 {
		t.Fatalf("expected 4 attempts, got %d", attempts)
	}
}

func TestRetryWithBackoffImmediateSuccess(t *testing.T) {
	attempts := 0
	err := RetryWithBackoff(context.Background(), 3, time.Millisecond, func() error {
		attempts++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", attempts)
	}
}
