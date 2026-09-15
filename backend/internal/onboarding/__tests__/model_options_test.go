package onboarding_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/onboarding"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

type modelOptionsKeys struct {
	byMarker  map[string]string
	created   []string
	deleted   []string
	createErr error
	deleteErr error
}

func (keys *modelOptionsKeys) CreateKey(_ context.Context, _ configstore.AuthRecord, marker, group string) (upstreamsync.CreatedKey, error) {
	if keys.createErr != nil {
		return upstreamsync.CreatedKey{}, &upstreamsync.CommitUnknownError{Marker: marker, Cause: keys.createErr}
	}
	if id := keys.byMarker[marker]; id != "" {
		return upstreamsync.CreatedKey{KeyID: id, Secret: testKey, GroupID: group}, nil
	}
	id := fmt.Sprint(100 + len(keys.created))
	keys.byMarker[marker] = id
	keys.created = append(keys.created, id)
	return upstreamsync.CreatedKey{KeyID: id, Secret: testKey, GroupID: group}, nil
}
func (keys *modelOptionsKeys) RevealKey(_ context.Context, _ configstore.AuthRecord, id, group string) (upstreamsync.CreatedKey, error) {
	return upstreamsync.CreatedKey{KeyID: id, Secret: testKey, GroupID: group}, nil
}
func (keys *modelOptionsKeys) DeleteKey(ctx context.Context, _ configstore.AuthRecord, id string) error {
	if ctx.Err() != nil {
		return errors.New("cleanup received cancelled context")
	}
	keys.deleted = append(keys.deleted, id)
	if keys.deleteErr != nil {
		return keys.deleteErr
	}
	for marker, existing := range keys.byMarker {
		if existing == id {
			delete(keys.byMarker, marker)
		}
	}
	return nil
}

type modelOptionsTasks struct{ final taskstore.Task }

func (store *modelOptionsTasks) Save(_ context.Context, task taskstore.Task) error {
	if task.Status == "succeeded" || task.Status == "failed" || task.Status == "cancelled" {
		store.final = task
	}
	return nil
}

type modelOptionsRunner struct{ run func(context.Context) }

func (runner *modelOptionsRunner) Go(run func(context.Context)) error { runner.run = run; return nil }
func (runner *modelOptionsRunner) GoTask(_ string, run func(context.Context)) error {
	return runner.Go(run)
}
func (*modelOptionsRunner) CancelTask(string) bool { return false }

func modelOptionsFixture(t *testing.T, gateway string, existingKey bool) (*onboarding.Service, *modelOptionsKeys, *modelOptionsTasks, *modelOptionsRunner) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "business.sqlite3")
	repo, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if err := repo.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{Host: "options.test", BaseURL: gateway, UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1"}); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO upstream_groups(host,group_id,name,platform,status,raw_rate,effective_rate,updated_at) VALUES('options.test','6','OpenAI','openai','active','1','1','now')`); err != nil {
		t.Fatal(err)
	}
	if existingKey {
		if _, err := db.Exec(`INSERT INTO upstream_keys(host,key_id,name,upstream_group,status,updated_at) VALUES('options.test','existing-77','user-owned','6','active','now')`); err != nil {
			t.Fatal(err)
		}
	}
	private, err := configstore.Open(filepath.Join(dir, "private.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = private.Close() })
	token := "isolated-user"
	if err := private.SaveAuthRecord(t.Context(), configstore.AuthRecord{Host: "options.test", BaseURL: gateway, UpstreamType: "sub2api", AuthMode: "sub2api_user_token", AccessToken: &token}, nil); err != nil {
		t.Fatal(err)
	}
	keys := &modelOptionsKeys{byMarker: map[string]string{}}
	tasks, runner := &modelOptionsTasks{}, &modelOptionsRunner{}
	service := onboarding.New(repo, private, keys, tasks)
	service.UseTaskRunner(runner)
	return service, keys, tasks, runner
}

func runModelOptions(t *testing.T, service *onboarding.Service, tasks *modelOptionsTasks, runner *modelOptionsRunner, ctx context.Context) taskstore.Task {
	t.Helper()
	if _, err := service.EnqueueProbe(t.Context(), "model-options", "options.test", "6", "", ""); err != nil {
		t.Fatal(err)
	}
	runner.run(ctx)
	return tasks.final
}

func TestModelOptionsReturnsModelsAfterCleaningOnlyItsTemporaryKey(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer "+testKey {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", 400)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"model-b"},{"id":"model-a"},{"id":"model-b"}]}`))
	}))
	t.Cleanup(gateway.Close)
	service, keys, tasks, runner := modelOptionsFixture(t, gateway.URL, false)
	if _, err := service.ProbeModels(t.Context(), "options.test", "6"); err != nil {
		t.Fatal(err)
	}
	completed := runModelOptions(t, service, tasks, runner, t.Context())
	if completed.Status != "succeeded" || !reflect.DeepEqual(completed.Result["models"], []string{"model-a", "model-b"}) {
		t.Fatalf("task=%+v", completed)
	}
	if !reflect.DeepEqual(keys.created, []string{"100", "101"}) || !reflect.DeepEqual(keys.deleted, []string{"101"}) {
		t.Fatalf("independent key not cleaned: created=%v deleted=%v", keys.created, keys.deleted)
	}
	if err := service.CancelProbe(t.Context(), "options.test", "6"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(keys.deleted, []string{"101", "100"}) {
		t.Fatalf("existing probe session was consumed or deleted: %v", keys.deleted)
	}
}

func TestModelOptionsDoesNotDeleteExistingUserKey(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"data":[{"id":"model-a"}]}`)) }))
	t.Cleanup(gateway.Close)
	service, keys, tasks, runner := modelOptionsFixture(t, gateway.URL, true)
	completed := runModelOptions(t, service, tasks, runner, t.Context())
	if completed.Status != "succeeded" || len(keys.created) != 0 || len(keys.deleted) != 0 {
		t.Fatalf("task=%+v created=%v deleted=%v", completed, keys.created, keys.deleted)
	}
}

func TestModelOptionsFailureAndCancellationCleanCreatedKeyWithoutPublishingModels(t *testing.T) {
	for _, scenario := range []string{"invalid response", "empty models", "cancelled", "cleanup failure", "cancelled cleanup failure"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				switch scenario {
				case "invalid response":
					_, _ = w.Write([]byte(`<html>invalid</html>`))
				case "empty models":
					_, _ = w.Write([]byte(`{"data":[]}`))
				case "cancelled", "cancelled cleanup failure":
					cancel()
				default:
					_, _ = w.Write([]byte(`{"data":[{"id":"model-a"}]}`))
				}
			}))
			t.Cleanup(gateway.Close)
			service, keys, tasks, runner := modelOptionsFixture(t, gateway.URL, false)
			if strings.Contains(scenario, "cleanup failure") {
				keys.deleteErr = errors.New("isolated delete failure")
			}
			completed := runModelOptions(t, service, tasks, runner, ctx)
			want := "failed"
			if scenario == "cancelled" {
				want = "cancelled"
			}
			if completed.Status != want || len(keys.deleted) != 1 || completed.Result["models"] != nil {
				t.Fatalf("task=%+v deleted=%v", completed, keys.deleted)
			}
			if strings.Contains(scenario, "cleanup failure") && !strings.Contains(completed.Message, "清理") {
				t.Fatalf("cleanup failure missing: %+v", completed)
			}
		})
	}
}

func TestModelOptionsCleanupRetryDoesNotConsumeOpenProbeCredential(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"model-a"}]}`))
	}))
	t.Cleanup(gateway.Close)
	service, keys, tasks, runner := modelOptionsFixture(t, gateway.URL, false)
	if _, err := service.ProbeModels(t.Context(), "options.test", "6"); err != nil {
		t.Fatal(err)
	}
	keys.deleteErr = errors.New("isolated delete failure")
	if task := runModelOptions(t, service, tasks, runner, t.Context()); task.Status != "failed" {
		t.Fatalf("task=%+v", task)
	}
	keys.deleteErr = nil
	if task := runModelOptions(t, service, tasks, runner, t.Context()); task.Status != "succeeded" {
		t.Fatalf("task=%+v", task)
	}
	if !reflect.DeepEqual(keys.deleted, []string{"101", "101", "102"}) {
		t.Fatalf("wrong retry cleanup keys: %v", keys.deleted)
	}
	if err := service.CancelProbe(t.Context(), "options.test", "6"); err != nil {
		t.Fatal(err)
	}
	if keys.deleted[len(keys.deleted)-1] != "100" {
		t.Fatalf("open probe session lost: %v", keys.deleted)
	}
}
