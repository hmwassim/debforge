package utils

import (
	"context"
	"fmt"
	"time"
)

func RetryWithBackoff(ctx context.Context, maxRetries int, baseDelay time.Duration, fn func() error) error {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := fn(); err != nil {
			lastErr = err
			if attempt == maxRetries {
				break
			}
			delay := baseDelay * time.Duration(1<<attempt)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
			continue
		}
		return nil
	}
	return fmt.Errorf("after %d retries: %w", maxRetries, lastErr)
}

func RetryHTTP(ctx context.Context, fn func() error) error {
	return RetryWithBackoff(ctx, 3, 500*time.Millisecond, fn)
}

func RetryGit(ctx context.Context, fn func() error) error {
	return RetryWithBackoff(ctx, 3, 1*time.Second, fn)
}
