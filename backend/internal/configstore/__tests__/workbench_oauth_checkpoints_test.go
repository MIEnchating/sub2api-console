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

func checkpointRecord() configstore.WorkbenchOAuthCheckpoint {
	now := time.Now().UTC()
	return configstore.WorkbenchOAuthCheckpoint{ID: strings.Repeat("a", 32), OwnerHash: strings.Repeat("b", 64), TargetFingerprint: strings.Repeat("c", 64), TargetURL: "https://target.example", SourceTaskID: strings.Repeat("d", 32), Status: "saving", CreatedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(15 * time.Minute).Format(time.RFC3339Nano), Payload: json.RawMessage(`{"state":"private-state","verifier":"private-verifier","proxy":"private-proxy"}`)}
}

func persistReadyCheckpoint(t *testing.T, store *configstore.Store) configstore.WorkbenchOAuthCheckpoint {
	t.Helper()
	input := checkpointRecord()
	saved, err := store.SaveWorkbenchOAuthCheckpoint(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	saved.Status, saved.Stage, saved.WorkerID, saved.WorkerLease, saved.Payload = "ready", "email_code", strings.Repeat("e", 48), strings.Repeat("f", 32), input.Payload
	ready, err := store.SaveWorkbenchOAuthCheckpoint(context.Background(), saved)
	if err != nil {
		t.Fatal(err)
	}
	ready.Payload = input.Payload
	return ready
}

func TestOAuthCheckpointRestartPreservesPrivateBindingAndListsOnlyMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.sqlite3")
	store, err := configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ready := persistReadyCheckpoint(t, store)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	list, err := store.WorkbenchOAuthCheckpoints(context.Background(), ready.OwnerHash, ready.TargetFingerprint)
	if err != nil || len(list) != 1 || list[0].Revision != ready.Revision || len(list[0].Payload) != 0 {
		t.Fatalf("restored summaries = %+v, %v", list, err)
	}
	private, err := store.WorkbenchOAuthCheckpoint(context.Background(), ready.OwnerHash, ready.TargetFingerprint, ready.ID)
	if err != nil || string(private.Payload) != string(ready.Payload) {
		t.Fatal("private authorization binding did not survive restart")
	}
	public, _ := json.Marshal(private)
	if strings.Contains(string(public), "private-verifier") || strings.Contains(string(public), "private-proxy") {
		t.Fatal("marshaling checkpoint metadata exposed private binding")
	}
	for _, scope := range [][2]string{{strings.Repeat("0", 64), ready.TargetFingerprint}, {ready.OwnerHash, strings.Repeat("0", 64)}} {
		if _, err := store.WorkbenchOAuthCheckpoint(context.Background(), scope[0], scope[1], ready.ID); !errors.Is(err, configstore.ErrWorkbenchOAuthCheckpoint) {
			t.Fatalf("another owner or target accessed private checkpoint = %v", err)
		}
	}
}

func TestOAuthCheckpointConcurrentRestoreClaimsOnlyOneRevisionAndCannotReturnToReady(t *testing.T) {
	store, err := configstore.Open(filepath.Join(t.TempDir(), "private.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ready := persistReadyCheckpoint(t, store)
	ready.Status, ready.TaskID = "restoring", strings.Repeat("1", 32)
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Go(func() { _, err := store.SaveWorkbenchOAuthCheckpoint(context.Background(), ready); results <- err })
	}
	wait.Wait()
	close(results)
	succeeded, conflict := 0, 0
	for err := range results {
		if err == nil {
			succeeded++
		} else if errors.Is(err, configstore.ErrWorkbenchOAuthCheckpoint) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if succeeded != 1 || conflict != 1 {
		t.Fatalf("concurrent restoration = success %d conflicts %d", succeeded, conflict)
	}
	claimed, err := store.WorkbenchOAuthCheckpoint(context.Background(), ready.OwnerHash, ready.TargetFingerprint, ready.ID)
	if err != nil {
		t.Fatal(err)
	}
	claimed.Status = "ready"
	if _, err := store.SaveWorkbenchOAuthCheckpoint(context.Background(), claimed); !errors.Is(err, configstore.ErrWorkbenchOAuthCheckpoint) {
		t.Fatal("claimed checkpoint became reusable")
	}
	claimed.Status, claimed.Payload = "restored", nil
	if _, err := store.SaveWorkbenchOAuthCheckpoint(context.Background(), claimed); err != nil {
		t.Fatal(err)
	}
	stored, err := store.WorkbenchOAuthCheckpoint(context.Background(), ready.OwnerHash, ready.TargetFingerprint, ready.ID)
	if err != nil || len(stored.Payload) != 0 || stored.Status != "restored" {
		t.Fatalf("consumed checkpoint payload bytes=%d status=%s error=%v", len(stored.Payload), stored.Status, err)
	}
}

func TestOAuthCheckpointRejectsIdentityRebindingAndExpiryExtensionThenPurgesPrivateData(t *testing.T) {
	store, err := configstore.Open(filepath.Join(t.TempDir(), "private.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ready := persistReadyCheckpoint(t, store)
	for _, field := range []string{"owner", "target", "source", "expiry"} {
		changed := ready
		changed.Status = "restoring"
		switch field {
		case "owner":
			changed.OwnerHash = strings.Repeat("1", 64)
		case "target":
			changed.TargetFingerprint = strings.Repeat("1", 64)
		case "source":
			changed.SourceTaskID = strings.Repeat("1", 32)
		case "expiry":
			changed.ExpiresAt = time.Now().Add(time.Hour).Format(time.RFC3339Nano)
		}
		if _, err := store.SaveWorkbenchOAuthCheckpoint(context.Background(), changed); !errors.Is(err, configstore.ErrWorkbenchOAuthCheckpoint) {
			t.Fatalf("changed %s accepted: %v", field, err)
		}
	}
	if err := store.PurgeExpiredWorkbenchOAuthCheckpoints(context.Background(), time.Now().Add(16*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.WorkbenchOAuthCheckpoint(context.Background(), ready.OwnerHash, ready.TargetFingerprint, ready.ID); !errors.Is(err, configstore.ErrWorkbenchOAuthCheckpoint) {
		t.Fatal("expired checkpoint retained private binding")
	}
}

func TestOAuthCheckpointRestartMarksInterruptedClaimsFailedAndClearsPrivatePayload(t *testing.T) {
	for _, status := range []string{"saving", "restoring"} {
		t.Run(status, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "private.sqlite3")
			store, err := configstore.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			var record configstore.WorkbenchOAuthCheckpoint
			if status == "saving" {
				record, err = store.SaveWorkbenchOAuthCheckpoint(context.Background(), checkpointRecord())
			} else {
				record = persistReadyCheckpoint(t, store)
				record.Status, record.TaskID = "restoring", strings.Repeat("1", 32)
				record, err = store.SaveWorkbenchOAuthCheckpoint(context.Background(), record)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			store, err = configstore.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			stored, err := store.WorkbenchOAuthCheckpoint(context.Background(), record.OwnerHash, record.TargetFingerprint, record.ID)
			if err != nil || stored.Status != "failed" || stored.Revision != record.Revision+1 || len(stored.Payload) != 0 {
				t.Fatalf("interrupted checkpoint status=%s revision=%d payload=%d err=%v", stored.Status, stored.Revision, len(stored.Payload), err)
			}
		})
	}
}
