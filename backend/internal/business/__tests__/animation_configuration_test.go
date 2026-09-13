package business_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestAnimationSchedulesPersistSeparatelyFromProfilesAndAuditExcludesPayload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "configuration.sqlite3")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	profiles := []byte(`{"active":{"id":"profile-1"}}`)
	schedules := []byte(`[{"account_id":"41","model":"private-model-marker","enabled":true}]`)
	if err := store.SaveModelCheckConfiguration(ctx, profiles, "operator", "draft.saved"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveAnimationConfiguration(ctx, schedules, "operator"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	loadedProfiles, err := reopened.LoadModelCheckConfiguration(ctx)
	if err != nil || string(loadedProfiles) != string(profiles) {
		t.Fatalf("profiles changed: %s %v", loadedProfiles, err)
	}
	loadedSchedules, err := reopened.LoadAnimationConfiguration(ctx)
	if err != nil || string(loadedSchedules) != string(schedules) {
		t.Fatalf("schedules not restored: %s %v", loadedSchedules, err)
	}
	audit, err := reopened.AuditEvents(ctx, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(audit)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "animation.schedule.saved") || strings.Contains(string(raw), "private-model-marker") {
		t.Fatalf("unexpected audit: %s", raw)
	}
}
