package management_test

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountops"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/management"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

type rateCollectionTarget struct {
	mu       sync.Mutex
	settings configstore.TargetSettings
}

func (target *rateCollectionTarget) TargetSettings(context.Context) (configstore.TargetSettings, error) {
	target.mu.Lock()
	defer target.mu.Unlock()
	return target.settings, nil
}

func (*rateCollectionTarget) AccountDefaults(context.Context) (configstore.AccountDefaultsSettings, error) {
	return configstore.AccountDefaultsSettings{Concurrency: 10, Priority: 1}, nil
}

func (target *rateCollectionTarget) changeKey() {
	target.mu.Lock()
	defer target.mu.Unlock()
	target.settings.AdminKey = "changed-test-key"
}

func rateCollectionStore(t *testing.T) (*business.Store, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rate-collection.sqlite3")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := store.SetMode(context.Background(), runtimepolicy.Full); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		INSERT INTO accounts(id,name,multiplier,metadata_json,updated_at)
		VALUES('11','Relay-1','1','{"platform":"openai"}','now');
		INSERT INTO upstreams(host,base_url,upstream_type,auth_status,metadata_json,updated_at)
		VALUES('upstream.example','https://upstream.example','sub2api','authenticated','{"site_name":"Relay"}','now');
		INSERT INTO upstream_identities(upstream_id,created_at,updated_at) VALUES('test-upstream','now','now');
		INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at)
		VALUES('upstream.example','test-upstream',1,'now');
		INSERT INTO bindings(id,local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,updated_at)
		VALUES(1,'11','upstream.example','key-1','test','codex','now');
		INSERT INTO binding_identities(binding_id,upstream_id,upstream_key_id,updated_at)
		VALUES(1,'test-upstream','key-1','now');
	`)
	if err != nil {
		t.Fatal(err)
	}
	return store, db
}

func TestRateCollectionAllowsManagementMutationAndRevalidatesTargetBeforeCommit(t *testing.T) {
	for _, changedTarget := range []bool{false, true} {
		name := "unchanged target preserves the successful observation"
		if changedTarget {
			name = "changed target rejects the old observation before writing"
		}
		t.Run(name, func(t *testing.T) {
			store, db := rateCollectionStore(t)
			probeStarted := make(chan struct{}, 1)
			finishProbe := make(chan struct{})
			var finishOnce sync.Once
			unblock := func() { finishOnce.Do(func() { close(finishProbe) }) }
			var catalogRequests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch request.URL.Path {
				case "/api/v1/admin/accounts/upstream-billing-probe/batch":
					probeStarted <- struct{}{}
					select {
					case <-finishProbe:
					case <-request.Context().Done():
						return
					}
					_, _ = w.Write([]byte(`{"data":{"results":[{"account_id":11,"snapshot":{"status":"ok","data":{"resolved_rate_multiplier":1}}}]}}`))
				case "/api/v1/admin/accounts":
					catalogRequests.Add(1)
					_, _ = w.Write([]byte(`{"data":{"items":[{"id":11,"name":"Relay-1","rate_multiplier":1}],"total":1}}`))
				default:
					t.Errorf("unexpected remote request: %s %s", request.Method, request.URL.Path)
					http.Error(w, "unexpected request", http.StatusBadRequest)
				}
			}))
			defer server.Close()
			defer unblock()
			target := &rateCollectionTarget{settings: configstore.TargetSettings{
				BaseURL: server.URL, AdminKey: "test-key", TimeoutSeconds: 5,
			}}
			service := management.New(target, store, nil, accountops.New(target, store, nil))
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			type outcome struct {
				result map[string]any
				err    error
			}
			done := make(chan outcome, 1)
			go func() {
				result, err := service.SyncAllAccountRates(mutationguard.WithAutomaticInspection(ctx), "test")
				done <- outcome{result: result, err: err}
			}()
			select {
			case <-probeStarted:
			case <-ctx.Done():
				t.Fatal("rate collection did not reach the isolated upstream")
			}
			mutationCtx, cancelMutation := context.WithTimeout(ctx, time.Second)
			_, release, err := mutationguard.Acquire(mutationguard.WithAutomaticInspection(mutationCtx), store, mutationguard.ManagementTarget())
			cancelMutation()
			if err != nil {
				unblock()
				<-done
				t.Fatalf("read-only rate collection blocked another management mutation: %v", err)
			}
			if changedTarget {
				target.changeKey()
			}
			if err := release(); err != nil {
				t.Fatal(err)
			}
			unblock()
			finished := <-done
			var observedRate sql.NullString
			if err := db.QueryRow(`SELECT upstream_rate FROM bindings WHERE local_account_id='11'`).Scan(&observedRate); err != nil {
				t.Fatal(err)
			}
			if changedTarget {
				if !errors.Is(finished.err, targetguard.ErrChanged) || observedRate.Valid || catalogRequests.Load() != 0 {
					t.Fatalf("old target observation escaped validation: result=%#v err=%v rate=%v catalog requests=%d", finished.result, finished.err, observedRate, catalogRequests.Load())
				}
				return
			}
			if finished.err != nil || finished.result["unchanged"] != 1 || !observedRate.Valid || observedRate.String != "1" {
				t.Fatalf("unchanged target lost the rate observation: result=%#v err=%v rate=%v", finished.result, finished.err, observedRate)
			}
		})
	}
}
