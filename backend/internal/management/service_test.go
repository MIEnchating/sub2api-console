package management

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type staticTarget struct {
	value configstore.TargetSettings
	err   error
}

func (target staticTarget) TargetSettings(context.Context) (configstore.TargetSettings, error) {
	return target.value, target.err
}

type captureRepository struct {
	mu            sync.Mutex
	accounts      []map[string]any
	groups        []map[string]any
	actor         string
	result        business.ManagementSyncResult
	maintenance   []business.BoundAccountMaintenance
	verifications []business.BindingVerification
	repairs       []business.AccountNameRepairCommit
	cleanedIDs    []string
	cleanupErr    error
}

func (repository *captureRepository) BoundAccountsForMaintenance(context.Context, []string) ([]business.BoundAccountMaintenance, error) {
	return append([]business.BoundAccountMaintenance{}, repository.maintenance...), nil
}

func (repository *captureRepository) CommitBindingVerification(_ context.Context, values []business.BindingVerification) error {
	repository.verifications = append(repository.verifications, values...)
	return nil
}

func (repository *captureRepository) CommitAccountNameRepairs(_ context.Context, values []business.AccountNameRepairCommit) error {
	repository.repairs = append(repository.repairs, values...)
	return nil
}

func (repository *captureRepository) CleanupMissingBindings(_ context.Context, accountIDs []string, _ string) (business.MissingBindingCleanupResult, error) {
	repository.cleanedIDs = append(repository.cleanedIDs, accountIDs...)
	if repository.cleanupErr != nil {
		return business.MissingBindingCleanupResult{}, repository.cleanupErr
	}
	return business.MissingBindingCleanupResult{Cleaned: len(accountIDs), IDs: append([]string{}, accountIDs...)}, nil
}

func (repository *captureRepository) SyncManagementSnapshot(
	_ context.Context,
	accounts []map[string]any,
	groups []map[string]any,
	actor string,
) (business.ManagementSyncResult, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.accounts, repository.groups, repository.actor = accounts, groups, actor
	return repository.result, nil
}

func TestSyncReadsBothCatalogsBeforeRepositoryWrite(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.URL.Path)
		if request.Header.Get("X-API-Key") != "admin-secret" {
			t.Fatalf("missing admin key")
		}
		w.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/api/v1/admin/groups" {
			_, _ = w.Write([]byte(`{"data":{"items":[{"id":7,"name":"codex"}],"total":1}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"items":[{"id":11,"name":"alpha"}],"total":1}}`))
	}))
	defer server.Close()
	repository := &captureRepository{result: business.ManagementSyncResult{Accounts: 1, Groups: 1, ReadOnly: true}}
	service := New(staticTarget{value: configstore.TargetSettings{
		BaseURL: server.URL, AdminKey: "admin-secret", TimeoutSeconds: 1,
	}}, repository, &memoryTasks{})
	result, err := service.Sync(context.Background(), "operator")
	if err != nil {
		t.Fatal(err)
	}
	if result.Accounts != 1 || len(repository.accounts) != 1 || len(repository.groups) != 1 || repository.actor != "operator" {
		t.Fatalf("result=%#v repository=%#v", result, repository)
	}
	if len(paths) != 2 || paths[0] != "/api/v1/admin/groups" || paths[1] != "/api/v1/admin/accounts" {
		t.Fatalf("paths=%#v", paths)
	}
}

func TestEnqueuedSyncPersistsTerminalFailure(t *testing.T) {
	tasks := &memoryTasks{terminal: make(chan taskstore.Task, 1)}
	service := New(staticTarget{err: errors.New("Admin 目标不可用")}, &captureRepository{}, tasks)
	service.timeout = time.Second
	queued, err := service.EnqueueSync(context.Background(), "operator")
	if err != nil {
		t.Fatal(err)
	}
	if queued.Status != "queued" || queued.Operation != "management-snapshot-sync" {
		t.Fatalf("queued=%#v", queued)
	}
	select {
	case terminal := <-tasks.terminal:
		if terminal.Status != "failed" || terminal.Progress != 100 || terminal.Result["remote_write"] != false {
			t.Fatalf("terminal=%#v", terminal)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("task did not reach terminal state")
	}
}

func TestRevalidationMatchesRemoteAccountsByStableID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"items":[{"id":11,"name":"same name"}],"total":1}}`))
	}))
	defer server.Close()
	repository := &captureRepository{maintenance: []business.BoundAccountMaintenance{
		{AccountID: "11", AccountName: "old", UpstreamHost: "api.example"},
		{AccountID: "12", AccountName: "same name", UpstreamHost: "api.example"},
	}}
	service := New(staticTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 1}}, repository, &memoryTasks{})
	result, err := service.revalidateAccounts(context.Background(), []string{"11", "12"}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if result["verified"] != 1 || result["missing"] != 1 || len(repository.verifications) != 2 {
		t.Fatalf("result=%#v verifications=%#v", result, repository.verifications)
	}
	if !repository.verifications[0].Exists || repository.verifications[1].Exists {
		t.Fatalf("verification must follow stable IDs: %#v", repository.verifications)
	}
}

func TestNameRepairWritesOnlyChangedRemoteNames(t *testing.T) {
	var mu sync.Mutex
	putPaths := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPut {
			mu.Lock()
			putPaths = append(putPaths, request.URL.Path)
			mu.Unlock()
			_, _ = w.Write([]byte(`{"data":{"id":12,"name":"Anc1ent API-1"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"items":[{"id":11,"name":"Anc1ent API-1"},{"id":12,"name":"号池-1"}],"total":2}}`))
	}))
	defer server.Close()
	repository := &captureRepository{maintenance: []business.BoundAccountMaintenance{
		{AccountID: "11", AccountName: "Anc1ent API-1", ExpectedName: "Anc1ent API-1", UpstreamHost: "api.anc1ent.top"},
		{AccountID: "12", AccountName: "号池-1", ExpectedName: "Anc1ent API-1", UpstreamHost: "api.anc1ent.top"},
	}}
	service := New(staticTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 1}}, repository, &memoryTasks{})
	result, err := service.repairAccountNames(context.Background(), []string{"11", "12"}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(putPaths) != 1 || putPaths[0] != "/api/v1/admin/accounts/12" {
		t.Fatalf("PUT paths=%#v", putPaths)
	}
	if result["renamed"] != 1 || result["unchanged"] != 1 || len(repository.repairs) != 1 || repository.repairs[0].AccountID != "12" {
		t.Fatalf("result=%#v repairs=%#v", result, repository.repairs)
	}
}

func TestMissingBindingCleanupRemovesOnlyIDsStillAbsentRemotely(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"items":[{"id":11,"name":"renamed account"}],"total":1}}`))
	}))
	defer server.Close()
	repository := &captureRepository{maintenance: []business.BoundAccountMaintenance{
		{AccountID: "11", AccountName: "old", UpstreamHost: "api.example"},
		{AccountID: "12", AccountName: "renamed account", UpstreamHost: "api.example"},
	}}
	service := New(staticTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 1}}, repository, &memoryTasks{})
	result, err := service.cleanupMissingBindings(context.Background(), []string{"11", "12"}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if len(repository.cleanedIDs) != 1 || repository.cleanedIDs[0] != "12" {
		t.Fatalf("cleanup must follow stable IDs: %#v", repository.cleanedIDs)
	}
	if result["cleaned"] != 1 || result["skipped"] != 1 {
		t.Fatalf("result=%#v", result)
	}
	items := result["items"].([]map[string]any)
	if items[0]["status"] != "账号仍然存在，未清理" || items[1]["status"] != "已清理失效绑定" {
		t.Fatalf("items=%#v", items)
	}
}

func TestMissingBindingCleanupDoesNotReportSuccessWhenCommitFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"items":[],"total":0}}`))
	}))
	defer server.Close()
	repository := &captureRepository{
		maintenance: []business.BoundAccountMaintenance{{AccountID: "12", AccountName: "old", UpstreamHost: "api.example"}},
		cleanupErr:  errors.New("database unavailable"),
	}
	service := New(staticTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 1}}, repository, &memoryTasks{})
	result, err := service.cleanupMissingBindings(context.Background(), []string{"12"}, "operator")
	if err == nil {
		t.Fatal("expected cleanup failure")
	}
	items := result["items"].([]map[string]any)
	if items[0]["status"] != "待清理" || result["cleaned"] != 0 {
		t.Fatalf("failed cleanup must not be reported as successful: %#v", result)
	}
}

type memoryTasks struct {
	mu       sync.Mutex
	values   []taskstore.Task
	terminal chan taskstore.Task
}

func (tasks *memoryTasks) Save(_ context.Context, task taskstore.Task) error {
	tasks.mu.Lock()
	tasks.values = append(tasks.values, task)
	tasks.mu.Unlock()
	if tasks.terminal != nil && (task.Status == "succeeded" || task.Status == "failed") {
		select {
		case tasks.terminal <- task:
		default:
		}
	}
	return nil
}
