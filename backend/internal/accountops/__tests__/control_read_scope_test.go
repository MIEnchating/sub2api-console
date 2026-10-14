package accountops_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountops"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

func TestAccountControlDoesNotRequireDisplayEvidenceHistory(t *testing.T) {
	store, db := accountWithoutDisplayHistory(t)
	schedulable := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/accounts/41/schedulable" {
			var body struct {
				Schedulable bool `json:"schedulable"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			schedulable = body.Schedulable
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": 41, "name": "control-account", "schedulable": schedulable}})
	}))
	t.Cleanup(server.Close)
	tasks, runner := &settingsTasks{}, &settingsRunner{}
	service := accountops.New(settingsTarget{endpoint: server.URL}, store, tasks)
	service.UseTaskRunner(runner)
	for _, action := range []string{"fuse", "recover", "exclude", "include"} {
		t.Run(action, func(t *testing.T) {
			if _, err := service.EnqueueControl(t.Context(), "41", action, "test"); err != nil {
				t.Fatal(err)
			}
			runner.run(t.Context())
			if tasks.last.Status != "succeeded" {
				t.Fatalf("control did not finish: %+v", tasks.last)
			}
			if action == "fuse" || action == "recover" {
				var confirmed bool
				if err := db.QueryRow(`SELECT schedulable FROM accounts WHERE id='41'`).Scan(&confirmed); err != nil {
					t.Fatal(err)
				}
				if confirmed != (action == "recover") {
					t.Fatalf("incorrect confirmed state after %s: %t", action, confirmed)
				}
			}
		})
	}
}

func accountWithoutDisplayHistory(t *testing.T) (*business.Store, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "control.sqlite3")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetMode(t.Context(), runtimepolicy.Full); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`INSERT INTO accounts(id,name,schedulable,updated_at) VALUES('41','control-account',1,'now');
		DROP TABLE account_stability_samples; DROP TABLE health_samples`); err != nil {
		t.Fatal(err)
	}
	return store, db
}

func TestSettingsSubmissionDoesNotReadDisplayHistory(t *testing.T) {
	store, _ := accountWithoutDisplayHistory(t)
	service := accountops.New(settingsTarget{endpoint: "https://unused.invalid"}, store, &settingsTasks{})
	service.UseTaskRunner(&settingsRunner{})
	task, err := service.EnqueueSettings(t.Context(), "41", accountops.SettingsInput{Priority: 20, LoadFactor: "1", Concurrency: 3}, "test")
	if err != nil || task.Status != "queued" {
		t.Fatalf("settings submission=%+v error=%v", task, err)
	}
}

func TestModelDiscoverySubmissionDoesNotReadDisplayHistory(t *testing.T) {
	store, _ := accountWithoutDisplayHistory(t)
	service := accountops.New(settingsTarget{endpoint: "https://unused.invalid"}, store, &settingsTasks{})
	service.UseTaskRunner(&settingsRunner{})
	task, err := service.EnqueueModelDiscovery(t.Context(), []string{"41"}, "test")
	if err != nil || task.Status != "queued" {
		t.Fatalf("model discovery submission=%+v error=%v", task, err)
	}
}
