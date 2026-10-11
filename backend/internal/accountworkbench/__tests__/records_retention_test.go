package accountworkbench_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func saveRetentionRecord(t *testing.T, store *configstore.Store, owner, id, action string, created time.Time, items any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"owner":  owner,
		"public": accountworkbench.Run{ID: id, Action: action, Status: "completed", CreatedAt: created},
		"items":  items,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveWorkbenchDocument(t.Context(), "run:"+owner+":"+id, 0, raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestRecordRetentionKeepsNewestHundredAcrossPagesAndOtherNamespaces(t *testing.T) {
	service, store := fixture(t, `{}`)
	owner, _ := previewOwner(t, store)
	other, _ := previewOwner(t, store)
	for index := range 103 {
		id := fmt.Sprintf("%032x", 200-index)
		saveRetentionRecord(t, store, owner, id, "export", time.Unix(int64(index), 0).UTC(), []any{})
	}
	maintenance := fmt.Sprintf("%032x", 300)
	otherID := fmt.Sprintf("%032x", 400)
	saveRetentionRecord(t, store, owner, maintenance, "maintenance", time.Unix(0, 0).UTC(), []any{})
	saveRetentionRecord(t, store, other, otherID, "export", time.Unix(0, 0).UTC(), []any{})
	if err := service.PruneRecords(t.Context(), owner); err != nil {
		t.Fatal(err)
	}
	for index := range 103 {
		id := fmt.Sprintf("%032x", 200-index)
		raw, revision, err := store.WorkbenchDocument(t.Context(), "run:"+owner+":"+id)
		if err != nil {
			t.Fatal(err)
		}
		if index < 3 {
			if len(raw) != 0 {
				t.Fatalf("old record %s was retained", id)
			}
		} else if len(raw) == 0 || revision != 1 {
			t.Fatalf("recent record %s was deleted or rewritten", id)
		}
	}
	for _, key := range []string{"run:" + owner + ":" + maintenance, "run:" + other + ":" + otherID} {
		if raw, revision, err := store.WorkbenchDocument(t.Context(), key); err != nil || len(raw) == 0 || revision != 1 {
			t.Fatalf("excluded record %s changed: revision=%d err=%v", key, revision, err)
		}
	}
}

func TestRecordRetentionDoesNotDecodePrivateFieldsOfRetainedRecords(t *testing.T) {
	service, store := fixture(t, `{}`)
	owner, _ := previewOwner(t, store)
	id := fmt.Sprintf("%032x", 1)
	want := saveRetentionRecord(t, store, owner, id, "export", time.Unix(1, 0).UTC(), "private fields are not retention metadata")
	if err := service.PruneRecords(t.Context(), owner); err != nil {
		t.Fatal(err)
	}
	raw, revision, err := store.WorkbenchDocument(t.Context(), "run:"+owner+":"+id)
	if err != nil || revision != 1 || string(raw) != string(want) {
		t.Fatalf("retained private record changed: revision=%d err=%v", revision, err)
	}
}

func TestRecordRetentionStillValidatesPrivateFieldsBeforeDeletingSelectedRecord(t *testing.T) {
	service, store := fixture(t, `{}`)
	owner, _ := previewOwner(t, store)
	for index := range 101 {
		var items any = []any{}
		if index == 0 {
			items = "invalid execution data"
		}
		saveRetentionRecord(t, store, owner, fmt.Sprintf("%032x", index+1), "export", time.Unix(int64(index), 0).UTC(), items)
	}
	if err := service.PruneRecords(t.Context(), owner); err == nil {
		t.Fatal("invalid selected record was accepted")
	}
	rows, err := store.WorkbenchDocuments(t.Context(), "run:"+owner+":", 501)
	if err != nil || len(rows) != 101 {
		t.Fatalf("invalid selected record caused deletion: count=%d err=%v", len(rows), err)
	}
}

func TestRecordRetentionCancelledReadDoesNotDeleteRecords(t *testing.T) {
	service, store := fixture(t, `{}`)
	owner, _ := previewOwner(t, store)
	id := fmt.Sprintf("%032x", 1)
	want := saveRetentionRecord(t, store, owner, id, "export", time.Unix(1, 0).UTC(), []any{})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := service.PruneRecords(ctx, owner); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled read error = %v", err)
	}
	raw, revision, err := store.WorkbenchDocument(t.Context(), "run:"+owner+":"+id)
	if err != nil || revision != 1 || string(raw) != string(want) {
		t.Fatalf("cancelled retention changed record: revision=%d err=%v", revision, err)
	}
}

func TestRecordRetentionRejectsInvalidSelectionMetadataWithoutDeletingRecords(t *testing.T) {
	for _, tc := range []struct{ name, raw string }{
		{"invalid time", `{"public":{"created_at":"invalid"}}`},
		{"numeric identity", `{"public":{"id":17}}`},
		{"boolean action", `{"public":{"action":true}}`},
		{"array public", `{"public":[]}`},
		{"string public", `{"public":"invalid"}`},
		{"array document", `[]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, store := fixture(t, `{}`)
			owner, _ := previewOwner(t, store)
			key := "run:" + owner + ":11111111111111111111111111111111"
			if _, err := store.SaveWorkbenchDocument(t.Context(), key, 0, json.RawMessage(tc.raw)); err != nil {
				t.Fatal(err)
			}
			if err := service.PruneRecords(t.Context(), owner); err == nil {
				t.Fatal("invalid retention metadata accepted")
			}
			if raw, revision, err := store.WorkbenchDocument(t.Context(), key); err != nil || revision != 1 || string(raw) != tc.raw {
				t.Fatalf("invalid retention changed document: revision=%d err=%v", revision, err)
			}
		})
	}
}

func TestRecordRetentionDeletesAssociatedTerminalTaskAndPrivateArtifact(t *testing.T) {
	service, store := fixture(t, `{}`)
	owner, _ := previewOwner(t, store)
	runner, tasks := runTasks(t, service)
	directory := t.TempDir()
	service.UseExecution(tasks, runner, nil, directory)
	const id = "11111111111111111111111111111111"
	const artifactID = "22222222222222222222222222222222"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{ID: "33333333333333333333333333333333", Skill: accountworkbench.Skill, Operation: "workbench-run", Status: "succeeded", Message: "处理完成", Result: map[string]any{"run_id": id}, CreatedAt: now, UpdatedAt: now}
	if err := tasks.Save(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]any{
		"owner":    owner,
		"public":   accountworkbench.Run{ID: id, Action: "export", CreatedAt: time.Unix(0, 0).UTC()},
		"task_ids": []string{task.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveWorkbenchDocument(t.Context(), "run:"+owner+":"+id, 0, raw); err != nil {
		t.Fatal(err)
	}
	artifactKey := "artifact:" + owner + ":" + id + ":" + artifactID
	raw, err = json.Marshal(map[string]any{"id": artifactID, "owner": owner, "run_id": id, "file": artifactID + ".json"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveWorkbenchDocument(t.Context(), artifactKey, 0, raw); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(directory, artifactID+".json")
	if err := os.WriteFile(filename, []byte(`{"private":"export"}`), 0600); err != nil {
		t.Fatal(err)
	}
	for index := range 100 {
		saveRetentionRecord(t, store, owner, fmt.Sprintf("%032x", index+1), "export", time.Unix(int64(index+1), 0).UTC(), []any{})
	}
	if err := service.PruneRecords(t.Context(), owner); err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.Get(t.Context(), task.ID); !errors.Is(err, taskstore.ErrNotFound) {
		t.Fatalf("evicted run task remained: %v", err)
	}
	for _, key := range []string{"run:" + owner + ":" + id, artifactKey} {
		if raw, _, err := store.WorkbenchDocument(t.Context(), key); err != nil || len(raw) != 0 {
			t.Fatalf("evicted run document remained: %s err=%v", key, err)
		}
	}
	if _, err := os.Stat(filename); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("evicted private file remained: %v", err)
	}
}

func TestRecordRetentionProtectsActiveRunOlderThanHundredCompletedRecords(t *testing.T) {
	service, store := fixture(t, `{}`)
	owner, _ := previewOwner(t, store)
	runner, _ := runTasks(t, service)
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	entered := make(chan struct{}, 1)
	service.UseOfficialTransport(transportFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://auth.openai.com/oauth/token" {
			return nil, fmt.Errorf("unexpected official endpoint")
		}
		entered <- struct{}{}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-request.Context().Done():
			return nil, request.Context().Err()
		}
	}))
	preview, err := service.Preview(t.Context(), owner, accountworkbench.PreviewInput{Action: "export", Content: "rt_retention-active"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.Start(t.Context(), owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("active run did not reach isolated request")
	}
	for index := range 100 {
		saveRetentionRecord(t, store, owner, fmt.Sprintf("%032x", index+1), "export", run.CreatedAt.Add(time.Duration(index+1)*time.Hour), []any{})
	}
	if err := service.PruneRecords(t.Context(), owner); err != nil {
		t.Fatal(err)
	}
	rows, err := store.WorkbenchDocuments(t.Context(), "run:"+owner+":", 501)
	if err != nil || len(rows) != 101 {
		t.Fatalf("active run consumed retained history allowance: count=%d err=%v", len(rows), err)
	}
	if raw, _, err := store.WorkbenchDocument(t.Context(), "run:"+owner+":"+run.ID); err != nil || len(raw) == 0 {
		t.Fatalf("active run was deleted: %v", err)
	}
	cancel()
	awaitRun(t, runner)
}
