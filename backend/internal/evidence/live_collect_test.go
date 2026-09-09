package evidence

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type liveTarget struct{ url string }

func (s liveTarget) TargetSettings(context.Context) (configstore.TargetSettings, error) {
	return configstore.TargetSettings{BaseURL: s.url, AdminKey: "isolated-test", TimeoutSeconds: 1}, nil
}
func TestCollectLivePersistsAndDeduplicatesRequestWithoutProbeOrInspection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "live.sqlite3")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','test','{}','now'); INSERT INTO account_groups(account_id,group_name) VALUES('41','codex')`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/admin/ops/requests" {
			t.Errorf("unexpected upstream operation: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("account_id") != "41" {
			t.Error("missing stable account filter")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"items": []any{
			map[string]any{"account_id": 41, "request_id": "request-1", "kind": "error", "created_at": now, "status_code": 503, "message": "overloaded"},
		}, "total": 1}})
	}))
	defer upstream.Close()
	service := New(store, nil)
	policy := map[string]any{"traffic": map[string]any{"enabled": true}}
	for range 2 {
		if err := service.CollectLive(context.Background(), policy, liveTarget{url: upstream.URL}, "41"); err != nil {
			t.Fatal(err)
		}
	}
	results, err := store.RecentAccountResults(context.Background(), "41", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ID == "" || results[0].Result == nil || *results[0].Result != "失败" {
		t.Fatalf("live evidence not persisted exactly once: %#v", results)
	}
}

func TestCollectLiveReturnsUpstreamFailureWithoutPersistingFalseSuccess(t *testing.T) {
	repository := &lateTrafficRepository{targets: []business.EvidenceTarget{{AccountID: "41", GroupName: "codex"}}}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"upstream unavailable"}`, http.StatusServiceUnavailable)
	}))
	defer upstream.Close()
	err := New(repository, nil).CollectLive(context.Background(), map[string]any{}, liveTarget{url: upstream.URL}, "41")
	if err == nil {
		t.Fatal("upstream failure was reported as successful collection")
	}
	if len(repository.samples) != 0 {
		t.Fatalf("failed collection fabricated samples: %#v", repository.samples)
	}
}

func TestCollectLiveHonorsDisabledTrafficWithoutCallingUpstream(t *testing.T) {
	repository := &lateTrafficRepository{targets: []business.EvidenceTarget{{AccountID: "41", GroupName: "codex"}}}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("disabled traffic contacted upstream")
		http.NotFound(w, r)
	}))
	defer upstream.Close()
	err := New(repository, nil).CollectLive(context.Background(), map[string]any{"traffic": map[string]any{"enabled": false}}, liveTarget{url: upstream.URL}, "41")
	if err != nil {
		t.Fatal(err)
	}
	if len(repository.samples) != 0 {
		t.Fatal("disabled traffic created evidence")
	}
}
