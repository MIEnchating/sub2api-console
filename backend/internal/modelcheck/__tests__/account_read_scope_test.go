package modelcheck_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestAnimationModelsWithoutHealthHistoryRetainsBoundCredential(t *testing.T) {
	service := detectionWithoutDisplayHistory(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer "+fixtureSecret {
			t.Error("model list did not use the bound credential")
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"isolated-model"}]}`))
	})
	models, err := service.AccountAnimationModels(t.Context(), "41")
	if err != nil || !reflect.DeepEqual(models, []string{"isolated-model"}) {
		t.Fatalf("model list depended on display history: models=%v error=%v", models, err)
	}
}

func TestAnimationScheduleWithoutHealthHistoryCanBeSaved(t *testing.T) {
	service := detectionWithoutDisplayHistory(t, func(http.ResponseWriter, *http.Request) {
		t.Error("saving a schedule must not contact the upstream")
	})
	values, err := service.SaveAnimationSchedules(t.Context(), []modelcheck.AnimationSchedule{
		{AccountID: "41", Model: "gpt-6-astra", Enabled: true, IntervalMinutes: 5, TimeoutSeconds: 5},
	}, "test")
	if err != nil || len(values) != 1 {
		t.Fatalf("schedule depended on display history: values=%v error=%v", values, err)
	}
}

func detectionWithoutDisplayHistory(t *testing.T, handler http.HandlerFunc) *modelcheck.Service {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	path := filepath.Join(t.TempDir(), "accounts.sqlite3")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	metadata, err := json.Marshal(map[string]string{"base_url": server.URL, "platform": "openai", "type": "apikey"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','isolated-model-account',?,'now');
		INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,upstream_group_id,local_group,status,updated_at)
		VALUES('41','isolated.example','91','isolated key','7','isolated group','active','now');
		DROP TABLE health_samples; DROP TABLE account_stability_samples`, string(metadata)); err != nil {
		t.Fatal(err)
	}
	tasks, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tasks.Close() })
	service, err := modelcheck.New(tasks, credentials{}, store, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	return service
}
