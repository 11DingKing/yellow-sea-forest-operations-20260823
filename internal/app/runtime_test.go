package app

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWaitForComponentsDoesNotReturnBeforeAllWorkersStop(t *testing.T) {
	workerDone := make(chan struct{})
	cleanupDone := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- waitForComponents(context.Background(), map[string]<-chan struct{}{
			"outbox worker":   workerDone,
			"session cleanup": cleanupDone,
		})
	}()
	close(workerDone)
	select {
	case err := <-result:
		t.Fatalf("wait returned before cleanup stopped: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(cleanupDone)
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("waitForComponents() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("waitForComponents() did not return after all components stopped")
	}
}

func TestWaitForComponentsHonorsShutdownDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err := waitForComponents(ctx, map[string]<-chan struct{}{"outbox worker": make(chan struct{})})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waitForComponents() error = %v", err)
	}
}
