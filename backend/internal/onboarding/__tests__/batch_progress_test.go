package onboarding_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/onboarding"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

type progressKeyRequest struct{ reply chan error }

// Only the external Key client is controlled; task and business storage are real.
type progressKeys struct {
	requests      chan progressKeyRequest
	catalog       []business.UpstreamCatalogKey
	skipOnRecheck bool
	listCalls     int
}

func (keys *progressKeys) wait(ctx context.Context) error {
	request := progressKeyRequest{reply: make(chan error, 1)}
	select {
	case keys.requests <- request:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-request.reply:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (keys *progressKeys) CreateKey(ctx context.Context, _ configstore.AuthRecord, name, group string) (upstreamsync.CreatedKey, error) {
	if err := keys.wait(ctx); err != nil {
		return upstreamsync.CreatedKey{}, err
	}
	return upstreamsync.CreatedKey{KeyID: "9" + group, Name: name, GroupID: group, Secret: testKey}, nil
}

func (*progressKeys) RevealKey(context.Context, configstore.AuthRecord, string, string) (upstreamsync.CreatedKey, error) {
	return upstreamsync.CreatedKey{}, errors.New("unexpected key reveal")
}

func (keys *progressKeys) ListKeys(context.Context, configstore.AuthRecord) ([]business.UpstreamCatalogKey, error) {
	keys.listCalls++
	if keys.skipOnRecheck && keys.listCalls == 2 {
		keys.catalog = keys.catalog[1:]
	}
	return append([]business.UpstreamCatalogKey{}, keys.catalog...), nil
}

func (keys *progressKeys) DeleteKey(ctx context.Context, _ configstore.AuthRecord, id string) error {
	if err := keys.wait(ctx); err != nil {
		return err
	}
	for index, key := range keys.catalog {
		if key.KeyID == id {
			keys.catalog = append(keys.catalog[:index], keys.catalog[index+1:]...)
			break
		}
	}
	return nil
}

type progressTaskStore struct {
	*taskstore.Store
	finished chan string
}

func (store *progressTaskStore) Save(ctx context.Context, task taskstore.Task) error {
	if err := store.Store.Save(ctx, task); err != nil {
		return err
	}
	if task.Status == "succeeded" || task.Status == "failed" || task.Status == "cancelled" {
		store.finished <- task.ID
	}
	return nil
}

func progressFixture(t *testing.T) (*onboarding.Service, *progressTaskStore, *taskrunner.Group, *progressKeys, []onboarding.Request) {
	t.Helper()
	var nextID atomic.Int64
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/admin/accounts/models/sync-upstream-preview":
			_, _ = w.Write([]byte(`{"data":{"models":["gemini-2.5-flash"]}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/accounts":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
				return
			}
			body["id"] = 70 + nextID.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"data": body})
		default:
			t.Errorf("unexpected management request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(admin.Close)
	_, repository, _, request := newService(t, admin.URL, admin.URL)
	platform, status, rate := "gemini", "active", "0.2"
	groups := []business.UpstreamCatalogGroup{}
	requests := []onboarding.Request{}
	keys := &progressKeys{requests: make(chan progressKeyRequest, 1)}
	for index := range 3 {
		id := fmt.Sprint(index + 6)
		groups = append(groups, business.UpstreamCatalogGroup{GroupID: id, Name: "group-" + id, Platform: &platform, Status: &status, RawRate: &rate})
		item := request
		item.UpstreamGroupID = id
		requests = append(requests, item)
		keys.catalog = append(keys.catalog, business.UpstreamCatalogKey{KeyID: "9" + id, Name: "key-" + id})
	}
	if _, err := repository.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{Host: "upstream.test", Catalog: &business.UpstreamCatalogSnapshot{Groups: groups}}); err != nil {
		t.Fatal(err)
	}
	private, err := configstore.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = private.Close() })
	if err := private.ConfigureTarget(t.Context(), admin.URL, "isolated-admin-key", 5); err != nil {
		t.Fatal(err)
	}
	token := "isolated-user-token"
	if err := private.SaveAuthRecord(t.Context(), configstore.AuthRecord{Host: request.Host, BaseURL: admin.URL, UpstreamType: "sub2api", AuthMode: "sub2api_user_token", AccessToken: &token}, nil); err != nil {
		t.Fatal(err)
	}
	store, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	tasks := &progressTaskStore{Store: store, finished: make(chan string, 1)}
	runner := taskrunner.NewBounded(t.Context(), 1)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := runner.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	service := onboarding.New(repository, private, keys, tasks)
	service.UseTaskRunner(runner)
	return service, tasks, runner, keys, requests
}

func nextProgressRequest(t *testing.T, keys *progressKeys) progressKeyRequest {
	t.Helper()
	select {
	case request := <-keys.requests:
		return request
	case <-time.After(10 * time.Second):
		t.Fatal("task did not reach isolated key request")
		return progressKeyRequest{}
	}
}

func persistedProgressTask(t *testing.T, tasks *progressTaskStore, id string) taskstore.Task {
	t.Helper()
	task, err := tasks.Get(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func finishedProgressTask(t *testing.T, tasks *progressTaskStore, id string) taskstore.Task {
	t.Helper()
	select {
	case got := <-tasks.finished:
		if got != id {
			t.Fatalf("finished task ID=%s want=%s", got, id)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("task did not finish")
	}
	return persistedProgressTask(t, tasks, id)
}

func TestBatchTasksReportProcessedItemsWhileNextRequestIsPending(t *testing.T) {
	for _, operation := range []string{"onboarding", "key-cleanup"} {
		for _, failFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/first-fails=%t", operation, failFirst), func(t *testing.T) {
				service, tasks, _, keys, requests := progressFixture(t)
				var queued taskstore.Task
				var err error
				if operation == "onboarding" {
					queued, err = service.EnqueueBatch(t.Context(), requests)
				} else {
					queued, err = service.EnqueueKeyCleanup(t.Context(), "upstream.test", []string{"96", "97", "98"}, "test")
				}
				if err != nil {
					t.Fatal(err)
				}
				for index, percent := range []int{0, 33, 66} {
					request := nextProgressRequest(t, keys)
					task := persistedProgressTask(t, tasks, queued.ID)
					if task.Status != "running" || task.Progress != percent {
						t.Errorf("processed=%d task status=%s progress=%d want=%d", index, task.Status, task.Progress, percent)
					}
					if task.Result["completed"] != json.Number(fmt.Sprint(index)) || task.Result["total"] != json.Number("3") {
						t.Errorf("processed=%d result=%v", index, task.Result)
					}
					items, ok := task.Result["items"].([]any)
					if !ok || len(items) != index {
						t.Errorf("processed=%d saved items=%v", index, task.Result["items"])
					}
					wantFailed := 0
					if failFirst && index > 0 {
						wantFailed = 1
					}
					if task.Result["failed"] != json.Number(fmt.Sprint(wantFailed)) {
						t.Errorf("processed=%d failure count=%v", index, task.Result["failed"])
					}
					if task.Result["readback_confirmed"] == true {
						t.Error("unfinished batch was reported fully read back")
					}
					raw, err := json.Marshal(task.Result)
					if err != nil {
						t.Fatal(err)
					}
					for _, secret := range []string{testKey, "isolated-admin-key", "isolated-user-token"} {
						if strings.Contains(string(raw), secret) {
							t.Error("progress included private credentials")
						}
					}
					if index == 0 && failFirst {
						request.reply <- errors.New("isolated key request rejected")
					} else {
						request.reply <- nil
					}
				}
				final := finishedProgressTask(t, tasks, queued.ID)
				wantStatus := "succeeded"
				if failFirst {
					wantStatus = "failed"
				}
				if final.Status != wantStatus || final.Progress != 100 || final.Result["completed"] != json.Number("3") {
					t.Fatalf("unexpected final result: %+v", final)
				}
			})
		}
	}
}

func TestKeyCleanupProgressCountsSkippedKeysBeforeNextDeletion(t *testing.T) {
	service, tasks, _, keys, _ := progressFixture(t)
	keys.skipOnRecheck = true
	queued, err := service.EnqueueKeyCleanup(t.Context(), "upstream.test", []string{"96", "97", "98"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	request := nextProgressRequest(t, keys)
	task := persistedProgressTask(t, tasks, queued.ID)
	if task.Progress != 33 || task.Result["completed"] != json.Number("1") || task.Result["skipped"] != json.Number("1") || task.Result["deleted"] != json.Number("0") {
		t.Fatalf("skipped key was not counted: %+v", task)
	}
	request.reply <- nil
	nextProgressRequest(t, keys).reply <- nil
	final := finishedProgressTask(t, tasks, queued.ID)
	if final.Status != "succeeded" || final.Result["completed"] != json.Number("3") || final.Result["deleted"] != json.Number("2") || final.Result["skipped"] != json.Number("1") {
		t.Fatalf("unexpected cleanup result: %+v", final)
	}
}

func TestBatchTaskCancellationPreservesProcessedResultsAndStopsRemainingRequests(t *testing.T) {
	for _, operation := range []string{"onboarding", "key-cleanup"} {
		t.Run(operation, func(t *testing.T) {
			service, tasks, runner, keys, requests := progressFixture(t)
			var queued taskstore.Task
			var err error
			if operation == "onboarding" {
				queued, err = service.EnqueueBatch(t.Context(), requests)
			} else {
				queued, err = service.EnqueueKeyCleanup(t.Context(), "upstream.test", []string{"96", "97", "98"}, "test")
			}
			if err != nil {
				t.Fatal(err)
			}
			nextProgressRequest(t, keys).reply <- nil
			_ = nextProgressRequest(t, keys)
			if !runner.CancelTask(queued.ID) {
				t.Fatal("running task was not cancellable")
			}
			final := finishedProgressTask(t, tasks, queued.ID)
			if final.Status != "cancelled" || final.Result["completed"] != json.Number("2") || final.Result["total"] != json.Number("3") {
				t.Fatalf("cancelled result lost processed count: %+v", final)
			}
			items, ok := final.Result["items"].([]any)
			if !ok || len(items) != 2 {
				t.Fatalf("cancelled result lost completed and interrupted item: %v", final.Result)
			}
			select {
			case <-keys.requests:
				t.Fatal("cancellation started another remote request")
			default:
			}
		})
	}
}
