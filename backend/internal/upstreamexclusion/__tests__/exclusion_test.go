package upstreamexclusion_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/MIEnchating/sub2api-console/backend/internal/accountops"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamexclusion"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type target struct{ value configstore.TargetSettings }

func (t target) TargetSettings(context.Context) (configstore.TargetSettings, error) {
	return t.value, nil
}

type fixture struct {
	service *upstreamexclusion.Service
	store   *business.Store
	db      *sql.DB
	request upstreamexclusion.Request
	mu      sync.Mutex
	states  map[string]bool
	writes  []string
	fail    string
}

func setup(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{states: map[string]bool{"41": true, "42": true}}
	path := filepath.Join(t.TempDir(), "test.db")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	f.store = store
	if err = store.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err = store.SetMode(t.Context(), runtimepolicy.Full); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{Host: "fixture.example", BaseURL: "https://fixture.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1"}); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	f.db = db
	if _, err = db.Exec(`INSERT INTO accounts(id,name,upstream_host,schedulable,updated_at) VALUES('41','first','fixture.example',1,'now'),('42','second','fixture.example',1,'now')`); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		path := strings.TrimPrefix(r.URL.Path, "/api/v1")
		for accountID, enabled := range f.states {
			if path == "/admin/accounts/"+accountID {
				json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": accountID, "name": "account", "schedulable": enabled}})
				return
			}
			if path == "/admin/accounts/"+accountID+"/schedulable" {
				if f.fail == accountID {
					w.WriteHeader(400)
					json.NewEncoder(w).Encode(map[string]any{"message": "fixture stop rejected"})
					return
				}
				var body struct {
					Schedulable bool `json:"schedulable"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				f.states[accountID] = body.Schedulable
				f.writes = append(f.writes, accountID)
				json.NewEncoder(w).Encode(map[string]any{"success": true})
				return
			}
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(404)
	}))
	t.Cleanup(server.Close)
	private := target{configstore.TargetSettings{BaseURL: server.URL, AdminKey: "fixture", TimeoutSeconds: 2}}
	f.service = upstreamexclusion.New(store, private, nil, accountops.New(private, store, nil))
	preview, err := f.service.Preview(t.Context(), "fixture.example")
	if err != nil {
		t.Fatal(err)
	}
	f.request = upstreamexclusion.Request{UpstreamID: preview.UpstreamID, Excluded: true, ExpectedAccountIDs: preview.AccountIDs}
	return f
}
func TestExclusionWithoutStopPreservesRemoteScheduling(t *testing.T) {
	f := setup(t)
	_, err := f.service.Apply(t.Context(), "fixture.example", f.request, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.writes) != 0 {
		t.Fatalf("unexpected writes: %v", f.writes)
	}
	excluded, err := f.store.UpstreamExcluded(t.Context(), "fixture.example")
	if err != nil || !excluded {
		t.Fatalf("exclusion: %v %v", excluded, err)
	}
}
func TestExclusionWithStopConfirmsEveryAccountAndRestoreDoesNotEnable(t *testing.T) {
	f := setup(t)
	f.request.StopScheduling = true
	if _, err := f.service.Apply(t.Context(), "fixture.example", f.request, "test", nil); err != nil {
		t.Fatal(err)
	}
	if f.states["41"] || f.states["42"] || len(f.writes) != 2 {
		t.Fatalf("states=%v writes=%v", f.states, f.writes)
	}
	f.request.Excluded = false
	f.request.StopScheduling = false
	if _, err := f.service.Apply(t.Context(), "fixture.example", f.request, "test", nil); err != nil {
		t.Fatal(err)
	}
	if len(f.writes) != 2 {
		t.Fatalf("restore wrote scheduling: %v", f.writes)
	}
}
func TestPartialStopKeepsExclusionAndSuccessfulStops(t *testing.T) {
	f := setup(t)
	f.fail = "42"
	f.request.StopScheduling = true
	result, err := f.service.Apply(t.Context(), "fixture.example", f.request, "test", nil)
	if err == nil || result["excluded"] != true || f.states["41"] || !f.states["42"] {
		t.Fatalf("result=%v states=%v err=%v", result, f.states, err)
	}
	excluded, _ := f.store.UpstreamExcluded(t.Context(), "fixture.example")
	if !excluded {
		t.Fatal("partial failure removed protection")
	}
}
func TestChangedPreviewRejectsBeforeExclusionOrRemoteWrite(t *testing.T) {
	f := setup(t)
	f.request.ExpectedAccountIDs = []string{"41"}
	f.request.StopScheduling = true
	if _, err := f.service.Apply(t.Context(), "fixture.example", f.request, "test", nil); err == nil {
		t.Fatal("stale preview accepted")
	}
	excluded, _ := f.store.UpstreamExcluded(t.Context(), "fixture.example")
	if excluded || len(f.writes) != 0 {
		t.Fatal("stale preview changed state")
	}
}
func TestMonitoringRejectsStopButAllowsExclusion(t *testing.T) {
	f := setup(t)
	if _, err := f.store.SetMode(t.Context(), runtimepolicy.Monitoring); err != nil {
		t.Fatal(err)
	}
	f.request.StopScheduling = true
	if _, err := f.service.Apply(t.Context(), "fixture.example", f.request, "test", nil); err == nil {
		t.Fatal("monitoring stop accepted")
	}
	f.request.StopScheduling = false
	if _, err := f.service.Apply(t.Context(), "fixture.example", f.request, "test", nil); err != nil {
		t.Fatal(err)
	}
}

func TestRetryAfterPartialStopDoesNotRewriteConfirmedStoppedAccount(t *testing.T) {
	f := setup(t)
	f.fail = "42"
	f.request.StopScheduling = true
	if _, err := f.service.Apply(t.Context(), "fixture.example", f.request, "test", nil); err == nil {
		t.Fatal("expected partial failure")
	}
	f.fail = ""
	if _, err := f.service.Apply(t.Context(), "fixture.example", f.request, "test", nil); err != nil {
		t.Fatal(err)
	}
	if len(f.writes) != 2 || f.states["41"] || f.states["42"] {
		t.Fatalf("retry states=%v writes=%v", f.states, f.writes)
	}
}

func TestExcludedAccountRejectsOrdinarySchedulingChangesBeforeRemoteAccess(t *testing.T) {
	f := setup(t)
	if _, err := f.service.Apply(t.Context(), "fixture.example", f.request, "test", nil); err != nil {
		t.Fatal(err)
	}
	service := accountops.New(target{}, f.store, nil)
	if _, err := service.Control(t.Context(), "41", "resume", "test"); err != business.ErrUpstreamExcluded {
		t.Fatalf("unexpected guard: %v", err)
	}
}
