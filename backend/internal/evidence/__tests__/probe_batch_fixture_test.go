package evidence_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/evidence"
	"github.com/MIEnchating/sub2api-console/backend/internal/probe"
)

type probeBatchFixture struct {
	store   *business.Store
	service *evidence.Service
	admin   *adminclient.Client
	policy  map[string]any
	now     time.Time
	path    string
	mu      sync.Mutex
	probed  []string
}

func newProbeBatchFixture(t *testing.T, accountIDs []string, freshIDs []string) *probeBatchFixture {
	t.Helper()
	fixture := &probeBatchFixture{now: time.Now().UTC()}
	path := filepath.Join(t.TempDir(), "evidence.sqlite3")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	fixture.store = store
	fixture.path = path
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, accountID := range accountIDs {
		if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES(?,?,?,?)`,
			accountID, "account-"+accountID, `{"known_models":["model-a","model-b"]}`, fixture.now.Format(time.RFC3339Nano)); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO account_groups(account_id,group_name) VALUES(?,'codex')`, accountID); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = store.UpdatePolicy(context.Background(), map[string]any{
		"traffic_enabled": freshIDs != nil,
		"advanced_policy": map[string]any{"probe": map[string]any{
			"concurrency": 2, "retry_enabled": false, "timeout_seconds": 5,
		}},
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/test") {
			accountID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/accounts/"), "/test")
			fixture.mu.Lock()
			fixture.probed = append(fixture.probed, accountID)
			fixture.mu.Unlock()
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"pong\"}}]}\n\ndata: {\"type\":\"test_complete\",\"success\":true}\n\n")
			return
		}
		if r.URL.Path == "/api/v1/admin/ops/requests" || r.URL.Path == "/api/v1/admin/usage" {
			rows := []map[string]any{}
			accountID := r.URL.Query().Get("account_id")
			if r.URL.Path == "/api/v1/admin/ops/requests" && slices.Contains(freshIDs, accountID) {
				rows = append(rows, map[string]any{
					"account_id": accountID, "request_id": "fresh-" + accountID, "kind": "success",
					"created_at": fixture.now.Format(time.RFC3339Nano), "duration_ms": 500,
					"first_token_ms": 200, "input_tokens": 1, "output_tokens": 1,
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"items": rows, "total": len(rows)}})
			return
		}
		t.Errorf("unexpected test endpoint: %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	settings, err := configstore.Open(filepath.Join(t.TempDir(), "settings.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = settings.Close() })
	if err := settings.ConfigureTarget(context.Background(), server.URL, "isolated-test", 5); err != nil {
		t.Fatal(err)
	}
	fixture.admin, err = adminclient.New(adminclient.Config{BaseURL: server.URL, AdminKey: "isolated-test", Attempts: 1, Timeout: time.Second}, nil)
	if err != nil {
		t.Fatal(err)
	}
	fixture.policy, err = store.ControlPolicy(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	fixture.service = evidence.New(store, probe.New(store, settings, nil))
	return fixture
}

func (f *probeBatchFixture) probeIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := slices.Clone(f.probed)
	slices.Sort(result)
	return result
}

func (f *probeBatchFixture) seedProbe(t *testing.T, accountID string, at time.Time) {
	t.Helper()
	_, err := f.store.PersistProbeSamples(context.Background(), []business.ProbeSample{{
		AccountID: accountID, GroupName: "codex", Result: "通过", SampleCount: 1, Attempts: 1,
		ObservedAt: at.Format(time.RFC3339Nano), RequestModel: "model-a",
	}})
	if err != nil {
		t.Fatal(err)
	}
}

func (f *probeBatchFixture) removeModels(t *testing.T, accountIDs []string) {
	t.Helper()
	db, err := sql.Open("sqlite", f.path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, accountID := range accountIDs {
		if _, err := db.Exec(`UPDATE accounts SET metadata_json='{}' WHERE id=?`, accountID); err != nil {
			t.Fatal(err)
		}
	}
}
