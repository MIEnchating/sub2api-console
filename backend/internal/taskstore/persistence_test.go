package taskstore

import (
	"context"
	"errors"
	"testing"
	"time"
)

type retrySaver struct {
	failures int
	calls    int
}

func (s *retrySaver) Save(context.Context, Task) error {
	s.calls++
	if s.calls <= s.failures {
		return errors.New("database busy")
	}
	return nil
}

func TestSaveFinalRetriesTransientPersistenceFailures(t *testing.T) {
	saver := &retrySaver{failures: 2}
	if err := SaveFinal(context.Background(), saver, Task{ID: "task-1"}); err != nil {
		t.Fatal(err)
	}
	if saver.calls != 3 {
		t.Fatalf("calls=%d", saver.calls)
	}
}

func TestSaveFinalStopsAtThreeAttempts(t *testing.T) {
	saver := &retrySaver{failures: 10}
	err := SaveFinal(context.Background(), saver, Task{ID: "task-1"})
	if err == nil || saver.calls != 3 {
		t.Fatalf("calls=%d err=%v", saver.calls, err)
	}
}

func TestSaveFinalHonorsCallerCancellation(t *testing.T) {
	saver := &retrySaver{failures: 10}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	err := SaveFinal(ctx, saver, Task{ID: "task-1"})
	if err == nil || time.Since(started) > time.Second {
		t.Fatalf("elapsed=%s err=%v", time.Since(started), err)
	}
}
