package configstore_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func queueFixture() configstore.WorkbenchQueue {
	now := time.Now().UTC()
	return configstore.WorkbenchQueue{ID: "queue-1", Owner: strings.Repeat("a", 64), Target: strings.Repeat("b", 64), Kind: "oauth-batch", TaskID: "task-1", Status: "running", CreatedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(time.Hour).Format(time.RFC3339Nano), Payload: json.RawMessage(`{"password":"private-queue-password"}`)}
}

func TestWorkbenchQueueRestartPreservesPrivatePayloadAndMarksInterrupted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.sqlite3")
	store, err := configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	value := queueFixture()
	saved, err := store.SaveWorkbenchQueue(ctx, value)
	if err != nil || saved.Revision != 1 || len(saved.Payload) != 0 {
		t.Fatalf("save metadata: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	private, err := store.WorkbenchQueue(ctx, value.Owner, value.Target, value.ID)
	if err != nil || private.Status != "interrupted" || private.Revision != 2 || !strings.Contains(string(private.Payload), "private-queue-password") {
		t.Fatalf("private recovery lost: %v", err)
	}
	list, err := store.WorkbenchQueues(ctx, value.Owner, value.Target)
	if err != nil || len(list) != 1 || len(list[0].Payload) != 0 {
		t.Fatal("list loaded credentials")
	}
	raw, _ := json.Marshal(private)
	if strings.Contains(string(raw), "private-queue-password") {
		t.Fatal("private record JSON leaked payload")
	}
	if _, err := store.WorkbenchQueue(ctx, strings.Repeat("c", 64), value.Target, value.ID); !errors.Is(err, configstore.ErrWorkbenchQueue) {
		t.Fatal("cross-owner queue access accepted")
	}
	if _, err := store.WorkbenchQueue(ctx, value.Owner, strings.Repeat("c", 64), value.ID); !errors.Is(err, configstore.ErrWorkbenchQueue) {
		t.Fatal("cross-target queue access accepted")
	}
}

func TestWorkbenchQueueConcurrentRestoreHasOneOwnerAndCannotExtendExpiry(t *testing.T) {
	store, err := configstore.Open(filepath.Join(t.TempDir(), "config.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	value := queueFixture()
	saved, err := store.SaveWorkbenchQueue(ctx, value)
	if err != nil {
		t.Fatal(err)
	}
	value.Revision, value.Status = saved.Revision, "interrupted"
	saved, err = store.SaveWorkbenchQueue(ctx, value)
	if err != nil {
		t.Fatal(err)
	}
	value.Revision, value.Status = saved.Revision, "running"
	var workers sync.WaitGroup
	results := make(chan error, 2)
	for _, taskID := range []string{"resume-1", "resume-2"} {
		workers.Go(func() {
			attempt := value
			attempt.TaskID = taskID
			_, err := store.SaveWorkbenchQueue(ctx, attempt)
			results <- err
		})
	}
	workers.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, configstore.ErrWorkbenchQueue) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("resume ownership: %d success, %d conflict", success, conflict)
	}
	current, err := store.WorkbenchQueue(ctx, value.Owner, value.Target, value.ID)
	if err != nil {
		t.Fatal(err)
	}
	current.ExpiresAt = time.Now().Add(90 * time.Minute).Format(time.RFC3339Nano)
	if _, err := store.SaveWorkbenchQueue(ctx, current); !errors.Is(err, configstore.ErrWorkbenchQueue) {
		t.Fatal("resume extended original lifetime")
	}
}

func TestWorkbenchQueueExpiredPayloadIsPurgedAndLateSaveCannotResurrect(t *testing.T) {
	store, err := configstore.Open(filepath.Join(t.TempDir(), "config.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	value := queueFixture()
	saved, err := store.SaveWorkbenchQueue(ctx, value)
	if err != nil {
		t.Fatal(err)
	}
	value.Revision = saved.Revision
	if err := store.PurgeExpiredWorkbenchQueues(ctx, time.Now().Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveWorkbenchQueue(ctx, value); !errors.Is(err, configstore.ErrWorkbenchQueue) {
		t.Fatal("late runner recreated expired credentials")
	}
	list, err := store.WorkbenchQueues(ctx, value.Owner, value.Target)
	if err != nil || len(list) != 0 {
		t.Fatal("expired recovery retained")
	}
}
