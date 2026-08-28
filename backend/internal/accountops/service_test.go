package accountops

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type testTarget struct {
	value configstore.TargetSettings
	calls atomic.Int32
}

func (target *testTarget) TargetSettings(context.Context) (configstore.TargetSettings, error) {
	target.calls.Add(1)
	return target.value, nil
}

func TestAccountFieldsRequireMatchingReadbackBeforeLocalCommit(t *testing.T) {
	repository, db, _ := accountRepository(t)
	if _, err := repository.UpdatePolicy(context.Background(), map[string]any{
		"advanced_policy": map[string]any{"writeback": map[string]any{"verification": true}},
	}, "test"); err != nil {
		t.Fatal(err)
	}
	var written atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPut {
			var body map[string]any
			decoder := json.NewDecoder(request.Body)
			decoder.UseNumber()
			if err := decoder.Decode(&body); err != nil || body["rate_multiplier"] != json.Number("0.2") ||
				body["priority"] != json.Number("7") || body["load_factor"] != json.Number("3") ||
				body["concurrency"] != json.Number("25") {
				t.Fatalf("body=%#v err=%v", body, err)
			}
			written.Store(true)
			_, _ = io.WriteString(w, `{"success":true}`)
			return
		}
		name, multiplier, notes := "alpha", "0.1", "old"
		priority, loadFactor, concurrency := 10, "2", 1
		if written.Load() {
			name, multiplier, notes = "renamed", "0.2", "new"
			priority, loadFactor, concurrency = 7, "3", 25
		}
		_, _ = io.WriteString(w, `{"data":{"id":41,"name":"`+name+`","priority":`+strconv.Itoa(priority)+`,"load_factor":`+loadFactor+`,"concurrency":`+strconv.Itoa(concurrency)+`,"rate_multiplier":`+multiplier+`,"notes":"`+notes+`"}}`)
	}))
	defer server.Close()
	target := &testTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 2}}
	service := New(target, repository, nil)
	name, multiplier, notes := "renamed", "0.2", "new"
	priority, loadFactor, concurrency := int64(7), "3", int64(25)
	result, err := service.SyncFields(context.Background(), "41", FieldPatch{
		NamePresent: true, Name: &name, PriorityPresent: true, Priority: &priority,
		LoadFactorPresent: true, LoadFactor: &loadFactor, ConcurrencyPresent: true, Concurrency: &concurrency,
		MultiplierPresent: true, Multiplier: &multiplier,
		NotesPresent: true, Notes: &notes,
	}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if result["remote_write"] != true {
		t.Fatalf("result=%#v", result)
	}
	var storedName, storedLoadFactor, storedMultiplier, metadata string
	var storedPriority, storedConcurrency int64
	if err := db.QueryRow(`SELECT name,priority,load_factor,concurrency,multiplier,metadata_json FROM accounts WHERE id='41'`).Scan(&storedName, &storedPriority, &storedLoadFactor, &storedConcurrency, &storedMultiplier, &metadata); err != nil {
		t.Fatal(err)
	}
	if storedName != "renamed" || storedPriority != 7 || storedLoadFactor != "3" || storedConcurrency != 25 || storedMultiplier != "0.2" || !strings.Contains(metadata, `"notes":"new"`) {
		t.Fatalf("name=%q priority=%d load=%q concurrency=%d multiplier=%q metadata=%s", storedName, storedPriority, storedLoadFactor, storedConcurrency, storedMultiplier, metadata)
	}
}

func TestAccountFieldsSkipReadbackByDefault(t *testing.T) {
	repository, _, _ := accountRepository(t)
	var reads, writes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPut {
			writes.Add(1)
			_, _ = io.WriteString(w, `{"success":true}`)
			return
		}
		reads.Add(1)
		_, _ = io.WriteString(w, `{"data":{"id":41,"name":"alpha","priority":10,"load_factor":2,"concurrency":1,"rate_multiplier":0.1,"notes":"old"}}`)
	}))
	defer server.Close()
	service := New(&testTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 2}}, repository, nil)
	priority := int64(7)
	result, err := service.SyncFields(context.Background(), "41", FieldPatch{PriorityPresent: true, Priority: &priority}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if reads.Load() != 1 || writes.Load() != 1 || result["readback_confirmed"] != false {
		t.Fatalf("reads=%d writes=%d result=%#v", reads.Load(), writes.Load(), result)
	}
}

func accountRepository(t *testing.T) (*business.Store, *sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "account-ops.sqlite3")
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	if err := repository.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`INSERT INTO accounts(
		id,name,schedulable,priority,load_factor,multiplier,metadata_json,updated_at
	) VALUES('41','alpha',1,10,'2','0.1','{"notes":"old"}','now')`); err != nil {
		t.Fatal(err)
	}
	return repository, db, path
}

func TestModelsPersistsNormalizedAccountModelCache(t *testing.T) {
	repository, db, _ := accountRepository(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/admin/accounts/41/models/sync-upstream" {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		_, _ = io.WriteString(w, `{"data":[{"id":"model-a"},{"id":"model-a"},{"id":"model-b"}]}`)
	}))
	defer server.Close()
	service := New(&testTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 2}}, repository, nil)
	models, err := service.Models(context.Background(), "41")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0] != "model-a" || models[1] != "model-b" {
		t.Fatalf("models=%#v", models)
	}
	var metadata string
	if err := db.QueryRow(`SELECT metadata_json FROM accounts WHERE id='41'`).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(metadata, `"known_models":["model-a","model-b"]`) || !strings.Contains(metadata, `"notes":"old"`) {
		t.Fatalf("metadata=%s", metadata)
	}
}
