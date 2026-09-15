package authrecovery_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/authrecovery"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamauth"
)

func TestBatchProgressPreservesNewerHostStatusAndCompleteSnapshot(t *testing.T) {
	ctx := t.Context()
	databasePath := filepath.Join(t.TempDir(), "business.sqlite3")
	repository, err := business.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	if err := repository.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	private, err := configstore.Open(filepath.Join(t.TempDir(), "private.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = private.Close() })
	var refreshes atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/auth/refresh":
			if refreshes.Add(1) == 2 {
				reason := "credentials revoked after the first recovery"
				if _, err := repository.PersistAuthRecoveryOutcomes(r.Context(), []business.AuthRecoveryOutcome{{Host: "one.example.test", Attempted: true, Reason: &reason}}, "newer-check"); err != nil {
					t.Error(err)
					http.Error(w, "fixture persistence failed", http.StatusInternalServerError)
					return
				}
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"access_token":"rotated-access","refresh_token":"rotated-refresh"}}`))
		case "/api/v1/user/profile":
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(upstream.Close)
	hosts := []string{"one.example.test", "two.example.test"}
	for _, host := range hosts {
		if _, err := repository.CreateUpstreamConfiguration(ctx, business.UpstreamConfigurationWrite{
			Host: host, BaseURL: "https://" + host, UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1",
		}); err != nil {
			t.Fatal(err)
		}
		refresh := "isolated-refresh"
		if err := private.SaveAuthRecord(ctx, configstore.AuthRecord{
			Host: host, BaseURL: upstream.URL, UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RefreshToken: &refresh,
		}, nil); err != nil {
			t.Fatal(err)
		}
	}
	service := authrecovery.New(repository, private, upstreamauth.New(upstream.Client()), nil, nil, nil)
	summary, err := service.RecoverInvalid(ctx, hosts, "batch")
	if err != nil || summary.Hosts != 2 || summary.Recovered != 2 || refreshes.Load() != 2 {
		t.Fatalf("batch result = %+v, refreshes = %d, error = %v", summary, refreshes.Load(), err)
	}
	reader, err := sql.Open("sqlite", "file:"+databasePath+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	var status, snapshot string
	if err := reader.QueryRowContext(ctx, `SELECT auth_status FROM upstreams WHERE host='one.example.test'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != business.UpstreamAuthStatusInvalid {
		t.Errorf("completing another host overwrote the newer first-host failure: %s", status)
	}
	if err := reader.QueryRowContext(ctx, `SELECT value_json FROM operational_snapshots WHERE state_key='auth-recovery-runtime-snapshot'`).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	var persisted struct {
		Results []business.AuthRecoveryOutcome `json:"results"`
	}
	if err := json.Unmarshal([]byte(snapshot), &persisted); err != nil {
		t.Fatal(err)
	}
	if len(persisted.Results) != 2 || persisted.Results[0].Host != hosts[0] || persisted.Results[1].Host != hosts[1] {
		t.Fatalf("batch snapshot lost its completed prefix: %+v", persisted.Results)
	}
}
