package notify

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSendWithRetryStopsAfterSuccess(t *testing.T) {
	attempts := 0
	err := SendWithRetry(context.Background(), 3, func(context.Context) error {
		attempts++
		if attempts < 2 {
			return errors.New("temporary")
		}
		return nil
	})
	if err != nil || attempts != 2 {
		t.Fatalf("retry failed: attempts=%d err=%v", attempts, err)
	}
}

func TestSendWithRetryHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err := SendWithRetry(ctx, 3, func(context.Context) error { return errors.New("temporary") })
	if err == nil {
		t.Fatal("cancelled retry returned success")
	}
}
