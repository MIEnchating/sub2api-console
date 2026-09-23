package business_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"path/filepath"
	"testing"
)

func TestDetectionTaskGroupsUseStableIDsDeduplicateAndPersistSeparately(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "plans.sqlite3")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = store.SyncManagementSnapshot(ctx, []map[string]any{{"id": json.Number("41"), "name": "fixture", "groups": []any{json.Number("7"), json.Number("8")}}}, []map[string]any{{"id": json.Number("7"), "name": "first"}, {"id": json.Number("8"), "name": "second"}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	ids, err := store.DetectionTaskAccountIDs(ctx, []string{"7", "8"})
	if err != nil || fmt.Sprint(ids) != "[41]" {
		t.Fatalf("membership: %v %v", ids, err)
	}
	memberships, err := store.DetectionTaskScope(ctx, []string{"7", "8"})
	if err != nil || fmt.Sprint(memberships.GroupIDsByAccount["41"]) != "[7 8]" {
		t.Fatalf("stable group memberships: %v %v", memberships, err)
	}
	if _, err := store.DetectionTaskAccountIDs(ctx, []string{"99"}); err == nil {
		t.Fatal("missing stable group accepted")
	}
	raw := []byte(`[{"id":"task-1","model":"private-marker"}]`)
	if err := store.SaveDetectionTasks(ctx, raw, "test", "detection.task.saved"); err != nil {
		t.Fatal(err)
	}
	if schedules, err := store.LoadAnimationConfiguration(ctx); err != nil || len(schedules) != 0 {
		t.Fatal("plans overwrite old schedules")
	}
	reopened, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.LoadDetectionTasks(ctx)
	if err != nil || string(got) != string(raw) {
		t.Fatalf("plan lost: %s %v", got, err)
	}
}
