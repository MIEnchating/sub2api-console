package management

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type staticTarget struct {
	value    configstore.TargetSettings
	err      error
	defaults configstore.AccountDefaultsSettings
}

type targetWithAuth struct {
	staticTarget
	record configstore.AuthRecord
}

type emptyAuthTarget struct{ staticTarget }

func (target emptyAuthTarget) AuthRecord(context.Context, string) (*configstore.AuthRecord, error) {
	return nil, nil
}

type captureAuthResolver struct {
	host   string
	record configstore.AuthRecord
}

func (resolver *captureAuthResolver) ResolveAuth(_ context.Context, host, _ string) (*configstore.AuthRecord, error) {
	resolver.host = host
	copy := resolver.record
	return &copy, nil
}

func (target targetWithAuth) AuthRecord(context.Context, string) (*configstore.AuthRecord, error) {
	copy := target.record
	return &copy, nil
}

type captureCatalogReader struct {
	calls    int
	snapshot business.UpstreamCatalogSnapshot
	err      error
}

func (reader *captureCatalogReader) ReadCatalog(context.Context, configstore.AuthRecord) (business.UpstreamCatalogSnapshot, error) {
	reader.calls++
	return reader.snapshot, reader.err
}

func (target staticTarget) TargetSettings(context.Context) (configstore.TargetSettings, error) {
	return target.value, target.err
}

func (target staticTarget) AccountDefaults(context.Context) (configstore.AccountDefaultsSettings, error) {
	if target.defaults.Concurrency == 0 {
		return configstore.AccountDefaultsSettings{Concurrency: 10, Priority: 1}, nil
	}
	return target.defaults, nil
}

type captureRepository struct {
	mu             sync.Mutex
	accounts       []map[string]any
	groups         []map[string]any
	actor          string
	result         business.ManagementSyncResult
	maintenance    []business.BoundAccountMaintenance
	accountNames   map[string]string
	verifications  []business.BindingVerification
	repairs        []business.AccountNameRepairCommit
	cleanedIDs     []string
	cleanupErr     error
	observations   []business.AccountRateObservation
	baseURLs       []business.AccountBaseURLObservation
	hostRepair     business.AccountUpstreamHostRepairResult
	defaultRepairs []business.AccountDefaultsRepairCommit
	manualControls map[string]business.ManualPriorityControl
}

func TestAccountIDsForHostMatchesPrimaryAndSourceAuthHostsWithoutDuplicates(t *testing.T) {
	bound := []business.BoundAccountMaintenance{
		{AccountID: "1", UpstreamHost: "PRIMARY.EXAMPLE"},
		{AccountID: "2", UpstreamHost: "alias.example", SourceAuthHost: "auth.example"},
		{AccountID: "2", UpstreamHost: "auth.example"},
		{AccountID: "3", UpstreamHost: "other.example"},
		{AccountID: " ", UpstreamHost: "auth.example"},
	}
	if got := accountIDsForHost(bound, "https://primary.example/"); len(got) != 1 || got[0] != "1" {
		t.Fatalf("primary host matches = %#v", got)
	}
	if got := accountIDsForHost(bound, "AUTH.EXAMPLE"); len(got) != 1 || got[0] != "2" {
		t.Fatalf("source auth host matches = %#v", got)
	}
}

func TestEnqueueAllAccountRateSyncQueuesEveryBoundAccount(t *testing.T) {
	repository := &captureRepository{maintenance: []business.BoundAccountMaintenance{
		{AccountID: "11", UpstreamHost: "first.example"},
		{AccountID: "12", UpstreamHost: "second.example"},
		{AccountID: "11", UpstreamHost: "first.example"},
	}}
	tasks := &memoryTasks{}
	service := New(staticTarget{}, repository, tasks)

	taskID, err := service.EnqueueAllAccountRateSync(context.Background(), "auto-inspection")
	if err != nil {
		t.Fatal(err)
	}
	if taskID == "" {
		t.Fatal("all-account rate sync did not return a task ID")
	}
	tasks.mu.Lock()
	defer tasks.mu.Unlock()
	if len(tasks.values) == 0 || tasks.values[0].Operation != "account-rate-sync" || tasks.values[0].Result["requested"] != 2 {
		t.Fatalf("queued tasks = %#v", tasks.values)
	}
}

func (repository *captureRepository) ManualPriorityControls(_ context.Context, accountIDs []string) (map[string]business.ManualPriorityControl, error) {
	result := make(map[string]business.ManualPriorityControl)
	for _, accountID := range accountIDs {
		if control, found := repository.manualControls[accountID]; found {
			result[accountID] = control
		}
	}
	return result, nil
}

func (repository *captureRepository) CommitAccountBaseURLObservations(_ context.Context, values []business.AccountBaseURLObservation) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.baseURLs = append(repository.baseURLs, values...)
	return nil
}

func (repository *captureRepository) RepairAccountUpstreamHosts(_ context.Context, _ []string, _ string) (business.AccountUpstreamHostRepairResult, error) {
	return repository.hostRepair, nil
}

func (repository *captureRepository) CommitAccountRateObservations(_ context.Context, values []business.AccountRateObservation) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.observations = append(repository.observations, values...)
	return nil
}

func (repository *captureRepository) AccountNamesForMaintenance(_ context.Context, accountIDs []string) (map[string]string, error) {
	result := make(map[string]string, len(accountIDs))
	for _, accountID := range accountIDs {
		if name, found := repository.accountNames[accountID]; found {
			result[accountID] = name
		}
	}
	return result, nil
}

type captureRateWriter struct {
	mu                  sync.Mutex
	values              map[string]string
	names               map[string]string
	errors              map[string]error
	calls               int
	multiplierOnlyCalls int
}

func (writer *captureRateWriter) SyncAccountRate(_ context.Context, accountID, name, multiplier, _ string) (map[string]any, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	writer.calls++
	if err := writer.errors[accountID]; err != nil {
		return nil, err
	}
	if writer.values == nil {
		writer.values = map[string]string{}
	}
	if writer.names == nil {
		writer.names = map[string]string{}
	}
	writer.values[accountID] = multiplier
	writer.names[accountID] = name
	return map[string]any{"remote_write": true, "readback_confirmed": true}, nil
}

func (writer *captureRateWriter) SyncAccountMultiplier(_ context.Context, accountID, multiplier, _ string) (map[string]any, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	writer.calls++
	writer.multiplierOnlyCalls++
	if err := writer.errors[accountID]; err != nil {
		return nil, err
	}
	if writer.values == nil {
		writer.values = map[string]string{}
	}
	writer.values[accountID] = multiplier
	return map[string]any{"remote_write": true, "readback_confirmed": true}, nil
}

func (repository *captureRepository) BoundAccountsForMaintenance(_ context.Context, accountIDs []string) ([]business.BoundAccountMaintenance, error) {
	requested := make(map[string]struct{}, len(accountIDs))
	for _, accountID := range accountIDs {
		requested[accountID] = struct{}{}
	}
	result := make([]business.BoundAccountMaintenance, 0, len(repository.maintenance))
	for _, account := range repository.maintenance {
		if len(requested) > 0 {
			if _, found := requested[account.AccountID]; !found {
				continue
			}
		}
		result = append(result, account)
	}
	return result, nil
}

func (repository *captureRepository) CommitBindingVerification(_ context.Context, values []business.BindingVerification) error {
	repository.verifications = append(repository.verifications, values...)
	return nil
}

func (repository *captureRepository) CommitAccountNameRepairs(_ context.Context, values []business.AccountNameRepairCommit) error {
	repository.repairs = append(repository.repairs, values...)
	return nil
}

func (repository *captureRepository) CommitAccountDefaultsRepairs(_ context.Context, values []business.AccountDefaultsRepairCommit, _ string) error {
	repository.defaultRepairs = append(repository.defaultRepairs, values...)
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
	var pathsMu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		pathsMu.Lock()
		paths = append(paths, request.URL.Path)
		pathsMu.Unlock()
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
	pathsMu.Lock()
	recordedPaths := append([]string{}, paths...)
	pathsMu.Unlock()
	if len(recordedPaths) != 2 || recordedPaths[0] != "/api/v1/admin/groups" || recordedPaths[1] != "/api/v1/admin/accounts" {
		t.Fatalf("paths=%#v", recordedPaths)
	}
}

func TestMaintenanceSkipsManualPriorityAccountsInMixedBatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"items":[{"id":12,"name":"automatic"}],"total":1}}`))
	}))
	defer server.Close()
	repository := &captureRepository{
		maintenance: []business.BoundAccountMaintenance{
			{AccountID: "11", AccountName: "manual", UpstreamHost: "api.example"},
			{AccountID: "12", AccountName: "automatic", UpstreamHost: "api.example"},
		},
		manualControls: map[string]business.ManualPriorityControl{
			"11": {AccountID: "11"},
		},
	}
	tasks := &memoryTasks{terminal: make(chan taskstore.Task, 1)}
	service := New(staticTarget{value: configstore.TargetSettings{
		BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 1,
	}}, repository, tasks)
	task := taskstore.Task{ID: "maintenance-manual-protection", Status: "queued", Result: map[string]any{}}
	service.executeMaintenance(task, "account-binding-revalidation", []string{"11", "12"}, "operator")

	select {
	case terminal := <-tasks.terminal:
		if terminal.Status != "succeeded" || terminal.Result["requested"] != 2 || terminal.Result["verified"] != 1 || terminal.Result["skipped"] != 1 {
			t.Fatalf("terminal=%#v", terminal)
		}
		if len(repository.verifications) != 1 || repository.verifications[0].AccountID != "12" {
			t.Fatalf("manual account entered maintenance commits: %#v", repository.verifications)
		}
	case <-time.After(time.Second):
		t.Fatal("maintenance task did not finish")
	}
}

func TestBaseURLValidationReadsAccountDetailsAndPersistsOnlyReturnedRows(t *testing.T) {
	var paths []string
	var pathsMu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		pathsMu.Lock()
		paths = append(paths, request.URL.Path)
		pathsMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/admin/accounts/11":
			_, _ = w.Write([]byte(`{"data":{"id":11,"name":"alpha","credentials":{"base_url":"https://relay.example.test/v1"}}}`))
		case "/api/v1/admin/accounts/12":
			_, _ = w.Write([]byte(`{"data":{"id":12,"name":"beta","credentials":{}}}`))
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()
	repository := &captureRepository{result: business.ManagementSyncResult{Accounts: 2, ReadOnly: true}}
	service := New(staticTarget{value: configstore.TargetSettings{
		BaseURL: server.URL, AdminKey: "admin-secret", TimeoutSeconds: 1,
	}}, repository, &memoryTasks{})
	result, err := service.validateAccountBaseURLs(context.Background(), []string{"11", "12"}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if result["resolved"] != 1 || result["unavailable"] != 1 || result["failed"] != 0 {
		t.Fatalf("result=%#v", result)
	}
	if len(repository.baseURLs) != 2 {
		t.Fatalf("repository=%#v", repository)
	}
	if repository.baseURLs[0].AccountID != "11" || repository.baseURLs[0].BaseURL == nil ||
		*repository.baseURLs[0].BaseURL != "https://relay.example.test/v1" || repository.baseURLs[1].BaseURL != nil {
		t.Fatalf("baseURLs=%#v", repository.baseURLs)
	}
	pathsMu.Lock()
	recordedPaths := append([]string{}, paths...)
	pathsMu.Unlock()
	if len(recordedPaths) != 2 {
		t.Fatalf("paths=%#v", recordedPaths)
	}
}

func TestBaseURLValidationReturnsCanceledContextInsteadOfSuccessfulFailureCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("canceled validation unexpectedly reached the management API")
	}))
	defer server.Close()
	service := New(staticTarget{value: configstore.TargetSettings{
		BaseURL: server.URL, AdminKey: "admin-secret", TimeoutSeconds: 1,
	}}, &captureRepository{}, &memoryTasks{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := service.validateAccountBaseURLs(ctx, []string{"11"}, "operator")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("validateAccountBaseURLs error = %v, want context.Canceled; result=%#v", err, result)
	}
}

func TestBaseURLValidationUsesSub2APIDefaultForAPIKeyAccounts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":750,"name":"relay","platform":"openai","type":"apikey","credentials":{}}}`))
	}))
	defer server.Close()
	repository := &captureRepository{}
	service := New(staticTarget{value: configstore.TargetSettings{
		BaseURL: server.URL, AdminKey: "admin-secret", TimeoutSeconds: 1,
	}}, repository, &memoryTasks{})
	result, err := service.validateAccountBaseURLs(context.Background(), []string{"750"}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if result["resolved"] != 1 || result["unavailable"] != 0 || len(repository.baseURLs) != 1 ||
		repository.baseURLs[0].BaseURL == nil || *repository.baseURLs[0].BaseURL != "https://api.openai.com" ||
		repository.baseURLs[0].Source != "platform_default" {
		t.Fatalf("result=%#v observations=%#v", result, repository.baseURLs)
	}
}

func TestBaseURLRepairWritesUpstreamAddressAndConfirmsExplicitValue(t *testing.T) {
	var mu sync.Mutex
	getCalls := 0
	var update map[string]any
	recovered, schedulingEnabled := false, false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/admin/accounts/750":
			switch request.Method {
			case http.MethodGet:
				mu.Lock()
				getCalls++
				currentCall := getCalls
				mu.Unlock()
				if currentCall == 1 {
					_, _ = w.Write([]byte(`{"data":{"id":750,"name":"relay","platform":"openai","type":"apikey","status":"error","schedulable":false,"credentials":{}}}`))
					return
				}
				_, _ = w.Write([]byte(`{"data":{"id":750,"name":"relay","platform":"openai","type":"apikey","status":"active","schedulable":true,"credentials":{"base_url":"https://relay.example.test/v1"}}}`))
			case http.MethodPut:
				if err := json.NewDecoder(request.Body).Decode(&update); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				_, _ = w.Write([]byte(`{"data":{"id":750,"status":"error","schedulable":false,"credentials":{"base_url":"https://relay.example.test/v1"}}}`))
			default:
				http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			}
		case "/api/v1/admin/accounts/750/recover-state":
			recovered = true
			_, _ = w.Write([]byte(`{"data":{"id":750,"status":"active","schedulable":false}}`))
		case "/api/v1/admin/accounts/750/schedulable":
			var body map[string]any
			_ = json.NewDecoder(request.Body).Decode(&body)
			schedulingEnabled, _ = body["schedulable"].(bool)
			_, _ = w.Write([]byte(`{"data":{"id":750,"status":"active","schedulable":true}}`))
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()
	repository := &captureRepository{maintenance: []business.BoundAccountMaintenance{{
		AccountID: "750", AccountName: "relay", UpstreamHost: "relay.example.test", NamingBaseURL: "https://relay.example.test/v1",
	}}}
	service := New(staticTarget{value: configstore.TargetSettings{
		BaseURL: server.URL, AdminKey: "admin-secret", TimeoutSeconds: 1,
	}}, repository, &memoryTasks{})

	result, err := service.repairAccountBaseURLs(context.Background(), []string{"750"}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	credentials, _ := update["credentials"].(map[string]any)
	if credentials["base_url"] != "https://relay.example.test/v1" {
		t.Fatalf("update=%#v", update)
	}
	if result["repaired"] != 1 || result["failed"] != 0 || result["remote_write"] != true {
		t.Fatalf("result=%#v", result)
	}
	if !recovered || !schedulingEnabled || getCalls != 2 || len(repository.baseURLs) != 1 || repository.baseURLs[0].Source != "explicit" ||
		repository.baseURLs[0].BaseURL == nil || *repository.baseURLs[0].BaseURL != "https://relay.example.test/v1" {
		t.Fatalf("recovered=%v scheduling=%v getCalls=%d observations=%#v", recovered, schedulingEnabled, getCalls, repository.baseURLs)
	}
	items := result["items"].([]map[string]any)
	if items[0]["status"] != "已修复并恢复调度" || items[0]["readback_confirmed"] != true {
		t.Fatalf("items=%#v", items)
	}
}

func TestBaseURLRepairDoesNotOverwriteExistingExplicitAddress(t *testing.T) {
	putCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPut {
			putCalls++
		}
		_, _ = w.Write([]byte(`{"data":{"id":750,"name":"relay","platform":"openai","type":"apikey","credentials":{"base_url":"https://cdn.example.test/v1"}}}`))
	}))
	defer server.Close()
	repository := &captureRepository{maintenance: []business.BoundAccountMaintenance{{
		AccountID: "750", AccountName: "relay", UpstreamHost: "relay.example.test", NamingBaseURL: "https://relay.example.test/v1",
	}}}
	service := New(staticTarget{value: configstore.TargetSettings{
		BaseURL: server.URL, AdminKey: "admin-secret", TimeoutSeconds: 1,
	}}, repository, &memoryTasks{})

	result, err := service.repairAccountBaseURLs(context.Background(), []string{"750"}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if putCalls != 0 || result["skipped"] != 1 || result["remote_write"] != false {
		t.Fatalf("putCalls=%d result=%#v", putCalls, result)
	}
	items := result["items"].([]map[string]any)
	if items[0]["status"] != "已有显式 Base URL，未覆盖" {
		t.Fatalf("items=%#v", items)
	}
}

func TestBaseURLRepairReactivatesAndSchedulesAccountWhenAddressAlreadyMatches(t *testing.T) {
	getCalls, statusUpdates := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/admin/accounts/750":
			switch request.Method {
			case http.MethodGet:
				getCalls++
				status, schedulable := "inactive", false
				if getCalls > 1 {
					status, schedulable = "active", true
				}
				_, _ = fmt.Fprintf(w, `{"data":{"id":750,"platform":"openai","type":"apikey","status":%q,"schedulable":%v,"credentials":{"base_url":"https://relay.example.test/v1"}}}`, status, schedulable)
			case http.MethodPut:
				var body map[string]any
				_ = json.NewDecoder(request.Body).Decode(&body)
				if body["status"] == "active" {
					statusUpdates++
				}
				_, _ = w.Write([]byte(`{"data":{"id":750,"status":"active","schedulable":false}}`))
			}
		case "/api/v1/admin/accounts/750/recover-state":
			_, _ = w.Write([]byte(`{"data":{"id":750,"status":"inactive","schedulable":false}}`))
		case "/api/v1/admin/accounts/750/schedulable":
			_, _ = w.Write([]byte(`{"data":{"id":750,"status":"active","schedulable":true}}`))
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()
	repository := &captureRepository{maintenance: []business.BoundAccountMaintenance{{
		AccountID: "750", AccountName: "relay", UpstreamHost: "relay.example.test", NamingBaseURL: "https://relay.example.test/v1",
	}}}
	service := New(staticTarget{value: configstore.TargetSettings{
		BaseURL: server.URL, AdminKey: "admin-secret", TimeoutSeconds: 1,
	}}, repository, &memoryTasks{})

	result, err := service.repairAccountBaseURLs(context.Background(), []string{"750"}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if result["repaired"] != 1 || statusUpdates != 1 || getCalls != 2 {
		t.Fatalf("result=%#v statusUpdates=%d getCalls=%d", result, statusUpdates, getCalls)
	}
}

func TestAccountUpstreamHostRepairReturnsRepositoryResult(t *testing.T) {
	repository := &captureRepository{hostRepair: business.AccountUpstreamHostRepairResult{
		Requested: 1, Repaired: 1, Items: []business.AccountUpstreamHostRepairItem{{AccountID: "24", Status: "已修复"}}, EventID: -3,
	}}
	service := New(staticTarget{}, repository, &memoryTasks{})
	result, err := service.repairAccountUpstreamHosts(context.Background(), []string{"24"}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if result["repaired"] != 1 || result["event_id"] != int64(-3) || result["remote_write"] != false {
		t.Fatalf("result=%#v", result)
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

func TestAccountRateSyncAppliesRechargeRatioToSub2APIMultiplier(t *testing.T) {
	paths := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/admin/accounts/upstream-billing-probe/batch":
			var body struct {
				AccountIDs []int64 `json:"account_ids"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil || len(body.AccountIDs) != 1 || body.AccountIDs[0] != 11 {
				t.Fatalf("body=%#v err=%v", body, err)
			}
			_, _ = w.Write([]byte(`{"data":{"results":[{"account_id":11,"snapshot":{"status":"ok","data":{"group_rate_multiplier":1.98,"user_rate_multiplier":1.5,"resolved_rate_multiplier":1.5,"effective_rate_multiplier":3}}}]}}`))
		case "/api/v1/admin/accounts":
			_, _ = w.Write([]byte(`{"data":{"items":[{"id":11,"name":"HX｜Relay-1.5","rate_multiplier":1.5}],"total":1}}`))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()
	repository := &captureRepository{maintenance: []business.BoundAccountMaintenance{{
		AccountID: "11", AccountName: "HX｜Relay-1.5", UpstreamHost: "upstream.example", CurrentMultiplier: "1.5", RechargeRate: "10",
		NamingSiteName: "HX｜Relay", NamingBaseURL: "https://upstream.example",
	}}}
	writer := &captureRateWriter{}
	service := New(staticTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 1}}, repository, &memoryTasks{}, writer)

	result, err := service.syncAccountRates(context.Background(), []string{"11"}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if result["updated"] != 1 || len(paths) != 2 || paths[0] != "/api/v1/admin/accounts/upstream-billing-probe/batch" || paths[1] != "/api/v1/admin/accounts" || writer.calls != 1 || writer.values["11"] != "0.15" || writer.names["11"] != "HX｜Relay-0.15" || len(repository.observations) != 1 || repository.observations[0].Rate != "1.5" {
		t.Fatalf("result=%#v paths=%#v calls=%d values=%#v names=%#v", result, paths, writer.calls, writer.values, writer.names)
	}
	items, ok := result["items"].([]map[string]any)
	if !ok || len(items) != 1 || items[0]["upstream_raw_multiplier"] != "1.5" || items[0]["recharge_rate"] != "10" || items[0]["account_multiplier"] != "0.15" {
		t.Fatalf("rate audit fields=%#v", result["items"])
	}
}

func TestSyncAllAccountRatesDiscoversEveryBoundAccount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/admin/accounts/upstream-billing-probe/batch":
			_, _ = w.Write([]byte(`{"data":{"results":[{"account_id":11,"snapshot":{"status":"ok","data":{"resolved_rate_multiplier":0.2}}},{"account_id":12,"snapshot":{"status":"ok","data":{"resolved_rate_multiplier":0.3}}}]}}`))
		case "/api/v1/admin/accounts":
			_, _ = w.Write([]byte(`{"data":{"items":[{"id":11,"name":"Example-0.1","rate_multiplier":0.1},{"id":12,"name":"Example-0.1","rate_multiplier":0.1}],"total":2}}`))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()
	repository := &captureRepository{maintenance: []business.BoundAccountMaintenance{
		{AccountID: "11", AccountName: "Example-0.1", UpstreamHost: "upstream.example", NamingSiteName: "Example", NamingBaseURL: "https://upstream.example"},
		{AccountID: "12", AccountName: "Example-0.1", UpstreamHost: "upstream.example", NamingSiteName: "Example", NamingBaseURL: "https://upstream.example"},
	}}
	writer := &captureRateWriter{}
	service := New(staticTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 1}}, repository, &memoryTasks{}, writer)

	result, err := service.SyncAllAccountRates(context.Background(), "auto-inspection")
	if err != nil {
		t.Fatal(err)
	}
	if result["requested"] != 2 || result["updated"] != 2 || writer.calls != 2 ||
		writer.values["11"] != "0.2" || writer.names["11"] != "Example-0.2" ||
		writer.values["12"] != "0.3" || writer.names["12"] != "Example-0.3" || len(repository.observations) != 2 {
		t.Fatalf("result=%#v calls=%d values=%#v names=%#v", result, writer.calls, writer.values, writer.names)
	}
}

func TestAccountRateSyncHonorsManualControlSyncChoice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/admin/accounts/upstream-billing-probe/batch":
			var body struct {
				AccountIDs []int64 `json:"account_ids"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil || !reflect.DeepEqual(body.AccountIDs, []int64{12}) {
				t.Fatalf("manual sync probe body=%#v err=%v", body, err)
			}
			_, _ = w.Write([]byte(`{"data":{"results":[{"account_id":12,"snapshot":{"status":"ok","data":{"resolved_rate_multiplier":0.3}}}]}}`))
		case "/api/v1/admin/accounts":
			_, _ = w.Write([]byte(`{"data":{"items":[{"id":11,"name":"manual-off","rate_multiplier":0.1},{"id":12,"name":"manual-on","rate_multiplier":0.1}],"total":2}}`))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()
	repository := &captureRepository{maintenance: []business.BoundAccountMaintenance{
		{AccountID: "11", AccountName: "manual-off", UpstreamHost: "upstream.example", ManualPriority: true},
		{AccountID: "12", AccountName: "manual-on", UpstreamHost: "upstream.example", ManualPriority: true, SyncBalanceMultiplier: true},
	}}
	writer := &captureRateWriter{}
	service := New(staticTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 1}}, repository, &memoryTasks{}, writer)

	result, err := service.syncAccountRates(context.Background(), []string{"11", "12"}, "auto-inspection")
	if err != nil {
		t.Fatal(err)
	}
	if result["skipped"] != 1 || result["updated"] != 1 || writer.multiplierOnlyCalls != 1 ||
		writer.values["12"] != "0.3" || writer.names["12"] != "" {
		t.Fatalf("result=%#v calls=%d multiplierOnly=%d values=%#v names=%#v", result, writer.calls, writer.multiplierOnlyCalls, writer.values, writer.names)
	}
}

func TestAccountRateSyncUsesLastSuccessfulObservationOnlyForReadOnlyFallbackWhenLiveProbeFails(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/admin/accounts/upstream-billing-probe/batch":
			_, _ = w.Write([]byte(`{"data":{"results":[{"account_id":107,"snapshot":{"status":"failed","last_error":"upstream unavailable"}}]}}`))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()
	repository := &captureRepository{maintenance: []business.BoundAccountMaintenance{{
		AccountID: "107", AccountName: "KKAPI-0.12", UpstreamHost: "kkgait.com",
		CurrentMultiplier: "0.12", KnownRawRate: "1.3", KnownRawRateSource: "account_observation", RechargeRate: "10",
		NamingSiteName: "KKAPI", NamingBaseURL: "https://kkgait.com",
	}}}
	writer := &captureRateWriter{}
	service := New(staticTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 1}}, repository, &memoryTasks{}, writer)

	result, err := service.syncAccountRates(context.Background(), []string{"107"}, "auto-inspection")
	if err != nil {
		t.Fatal(err)
	}
	items, ok := result["items"].([]map[string]any)
	if !ok || len(items) != 1 {
		t.Fatalf("items=%#v", result["items"])
	}
	if result["updated"] != 0 || result["failed"] != 0 || result["skipped"] != 1 || result["fallback"] != 1 || result["remote_write"] != false ||
		requests != 1 || writer.calls != 0 || items[0]["observation_source"] != "last_successful" || items[0]["read_only"] != true ||
		items[0]["status"] != "只读降级，已跳过写回" || items[0]["account_multiplier"] != "0.13" || len(repository.observations) != 0 {
		t.Fatalf("result=%#v requests=%d values=%#v names=%#v observations=%#v", result, requests, writer.values, writer.names, repository.observations)
	}
}

func TestAccountRateSyncUsesFixedGroupCatalogOnlyForReadOnlyFallbackWhenLiveProbeFails(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/admin/accounts/upstream-billing-probe/batch":
			_, _ = w.Write([]byte(`{"data":{"results":[{"account_id":818,"snapshot":{"status":"failed","last_error":"upstream unavailable"}}]}}`))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()
	repository := &captureRepository{maintenance: []business.BoundAccountMaintenance{{
		AccountID: "818", AccountName: "Pixel API-0.1", UpstreamHost: "pixel.example",
		CurrentMultiplier: "0.1", KnownRawRate: "0.2", KnownRawRateSource: "group_catalog", RechargeRate: "10",
		NamingSiteName: "Pixel API", NamingBaseURL: "https://pixel.example",
	}}}
	writer := &captureRateWriter{}
	service := New(staticTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 1}}, repository, &memoryTasks{}, writer)

	result, err := service.syncAccountRates(context.Background(), []string{"818"}, "auto-inspection")
	if err != nil {
		t.Fatal(err)
	}
	items, ok := result["items"].([]map[string]any)
	if !ok || len(items) != 1 {
		t.Fatalf("items=%#v", result["items"])
	}
	if result["updated"] != 0 || result["failed"] != 0 || result["skipped"] != 1 || result["fallback"] != 1 || result["remote_write"] != false ||
		requests != 1 || writer.calls != 0 || items[0]["observation_source"] != "group_catalog" || items[0]["read_only"] != true ||
		items[0]["status"] != "只读降级，已跳过写回" || items[0]["account_multiplier"] != "0.02" || len(repository.observations) != 0 {
		t.Fatalf("result=%#v requests=%d values=%#v names=%#v observations=%#v", result, requests, writer.values, writer.names, repository.observations)
	}
}

func TestAccountRateSyncTreatsMissingNewAPIAuthAsPermanentFailure(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests++
		t.Fatalf("unexpected management request %s", request.URL.Path)
	}))
	defer server.Close()
	repository := &captureRepository{maintenance: []business.BoundAccountMaintenance{{
		AccountID: "919", AccountName: "Auth API-0.1", UpstreamHost: "auth.example", UpstreamType: "newapi",
		UpstreamKeyID: "101", CurrentMultiplier: "0.1", KnownRawRate: "0.2", KnownRawRateSource: "group_catalog", RechargeRate: "10",
		NamingSiteName: "Auth API", NamingBaseURL: "https://auth.example",
	}}}
	writer := &captureRateWriter{}
	service := New(emptyAuthTarget{staticTarget{value: configstore.TargetSettings{
		BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 1,
	}}}, repository, &memoryTasks{}, writer)
	service.UseUpstreamCatalogReader(&captureCatalogReader{})

	result, err := service.syncAccountRates(context.Background(), []string{"919"}, "auto-inspection")
	if err != nil {
		t.Fatal(err)
	}
	items, ok := result["items"].([]map[string]any)
	if !ok || len(items) != 1 {
		t.Fatalf("items=%#v", result["items"])
	}
	if result["updated"] != 0 || result["failed"] != 1 || result["skipped"] != 0 || result["fallback"] != 0 ||
		result["remote_write"] != false || requests != 0 || writer.calls != 0 || items[0]["status"] != "上游探测失败" ||
		!strings.Contains(fmt.Sprint(items[0]["error"]), "私有授权记录") {
		t.Fatalf("result=%#v requests=%d calls=%d", result, requests, writer.calls)
	}
}

func TestAccountRateSyncUsesStoredRateOnlyWhenNewAPICatalogReadFails(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests++
		t.Fatal("read-only fallback unexpectedly read the management account catalog")
	}))
	defer server.Close()
	repository := &captureRepository{maintenance: []business.BoundAccountMaintenance{{
		AccountID: "920", AccountName: "Catalog API-0.1", UpstreamHost: "catalog.example", UpstreamType: "newapi",
		UpstreamKeyID: "101", UpstreamGroupID: "pro", CurrentMultiplier: "0.1", KnownRawRate: "0.2",
		KnownRawRateSource: "group_catalog", RechargeRate: "2",
	}}}
	writer := &captureRateWriter{}
	service := New(targetWithAuth{
		staticTarget: staticTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 1}},
		record:       configstore.AuthRecord{Host: "catalog.example", BaseURL: "https://catalog.example", UpstreamType: "newapi"},
	}, repository, &memoryTasks{}, writer)
	service.UseUpstreamCatalogReader(&captureCatalogReader{err: errors.New("catalog network unavailable")})

	result, err := service.syncAccountRates(context.Background(), []string{"920"}, "auto-inspection")
	if err != nil {
		t.Fatal(err)
	}
	items := result["items"].([]map[string]any)
	if result["failed"] != 0 || result["skipped"] != 1 || result["fallback"] != 1 || requests != 0 || writer.calls != 0 ||
		len(items) != 1 || items[0]["status"] != "只读降级，已跳过写回" || items[0]["account_multiplier"] != "0.1" {
		t.Fatalf("result=%#v requests=%d calls=%d", result, requests, writer.calls)
	}
}

func TestAccountRateSyncDoesNotFallbackForPermanentNewAPIErrors(t *testing.T) {
	group, validRate := "pro", "0.2"
	tests := []struct {
		name     string
		account  business.BoundAccountMaintenance
		catalog  business.UpstreamCatalogSnapshot
		wantText string
	}{
		{
			name:    "duplicate token",
			account: business.BoundAccountMaintenance{UpstreamKeyID: "101", UpstreamGroupID: group, RechargeRate: "2"},
			catalog: business.UpstreamCatalogSnapshot{Groups: []business.UpstreamCatalogGroup{{GroupID: group, RawRate: &validRate}},
				Keys: []business.UpstreamCatalogKey{{KeyID: "101", UpstreamGroup: &group}, {KeyID: "101", UpstreamGroup: &group}}},
			wantText: "重复",
		},
		{
			name:     "ambiguous token groups",
			account:  business.BoundAccountMaintenance{UpstreamKeyID: "101", RechargeRate: "2"},
			catalog:  business.UpstreamCatalogSnapshot{Keys: []business.UpstreamCatalogKey{{KeyID: "101", RateAmbiguous: true}}},
			wantText: "多分组",
		},
		{
			name:     "missing fixed group",
			account:  business.BoundAccountMaintenance{UpstreamKeyID: "101", RechargeRate: "2"},
			catalog:  business.UpstreamCatalogSnapshot{Keys: []business.UpstreamCatalogKey{{KeyID: "101"}}},
			wantText: "固定分组",
		},
		{
			name:    "invalid recharge rate",
			account: business.BoundAccountMaintenance{UpstreamKeyID: "101", UpstreamGroupID: group, RechargeRate: "invalid"},
			catalog: business.UpstreamCatalogSnapshot{Groups: []business.UpstreamCatalogGroup{{GroupID: group, RawRate: &validRate}},
				Keys: []business.UpstreamCatalogKey{{KeyID: "101", UpstreamGroup: &group}}},
			wantText: "折算倍率无效",
		},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				requests++
				t.Fatal("permanent upstream error unexpectedly read the management account catalog")
			}))
			defer server.Close()
			test.account.AccountID = fmt.Sprint(930 + index)
			test.account.AccountName = "Permanent API-0.1"
			test.account.UpstreamHost = "permanent.example"
			test.account.UpstreamType = "newapi"
			test.account.KnownRawRate = "0.2"
			test.account.KnownRawRateSource = "group_catalog"
			repository := &captureRepository{maintenance: []business.BoundAccountMaintenance{test.account}}
			writer := &captureRateWriter{}
			service := New(targetWithAuth{
				staticTarget: staticTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 1}},
				record:       configstore.AuthRecord{Host: "permanent.example", BaseURL: "https://permanent.example", UpstreamType: "newapi"},
			}, repository, &memoryTasks{}, writer)
			service.UseUpstreamCatalogReader(&captureCatalogReader{snapshot: test.catalog})

			result, err := service.syncAccountRates(context.Background(), []string{test.account.AccountID}, "auto-inspection")
			if err != nil {
				t.Fatal(err)
			}
			items := result["items"].([]map[string]any)
			if result["failed"] != 1 || result["skipped"] != 0 || result["fallback"] != 0 || result["remote_write"] != false ||
				requests != 0 || writer.calls != 0 || len(items) != 1 || items[0]["status"] != "上游探测失败" ||
				!strings.Contains(fmt.Sprint(items[0]["error"]), test.wantText) {
				t.Fatalf("result=%#v requests=%d calls=%d", result, requests, writer.calls)
			}
		})
	}
}

func TestAccountRateSyncAllManualSkipsDoNotInitializeManagementClientAndExplainTaskCounts(t *testing.T) {
	repository := &captureRepository{maintenance: []business.BoundAccountMaintenance{{
		AccountID: "940", AccountName: "Manual API-0.1", UpstreamHost: "manual.example", ManualPriority: true,
	}}}
	tasks := &memoryTasks{terminal: make(chan taskstore.Task, 1)}
	service := New(staticTarget{err: errors.New("management target must not be read")}, repository, tasks, &captureRateWriter{})
	task := taskstore.Task{ID: "manual-rate-sync", Status: "queued", Result: map[string]any{}}

	service.executeMaintenance(task, "account-rate-sync", []string{"940"}, "auto-inspection")

	select {
	case terminal := <-tasks.terminal:
		if terminal.Status != "succeeded" || terminal.Result["skipped"] != 1 || terminal.Result["fallback"] != 0 ||
			!strings.Contains(terminal.Message, "跳过 1 个") || !strings.Contains(terminal.Message, "只读降级 0 个") {
			t.Fatalf("terminal=%#v", terminal)
		}
	case <-time.After(time.Second):
		t.Fatal("rate sync task did not finish")
	}
}

func TestAccountRateSyncSkipsWriteWhenManagementMultiplierAlreadyMatches(t *testing.T) {
	paths := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/admin/accounts/upstream-billing-probe/batch":
			_, _ = w.Write([]byte(`{"data":{"results":[{"account_id":11,"snapshot":{"status":"ok","data":{"resolved_rate_multiplier":0.15}}}]}}`))
		case "/api/v1/admin/accounts":
			_, _ = w.Write([]byte(`{"data":{"items":[{"id":11,"name":"Example-0.15","rate_multiplier":0.1500}],"total":1}}`))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()
	repository := &captureRepository{maintenance: []business.BoundAccountMaintenance{{
		AccountID: "11", AccountName: "Example-0.15", CurrentMultiplier: "0.15", UpstreamHost: "upstream.example",
		NamingSiteName: "Example", NamingBaseURL: "https://upstream.example",
	}}}
	writer := &captureRateWriter{}
	service := New(staticTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 1}}, repository, &memoryTasks{}, writer)

	result, err := service.syncAccountRates(context.Background(), []string{"11"}, "operator")
	if err != nil || result["unchanged"] != 1 || result["updated"] != 0 || writer.calls != 0 || len(paths) != 2 {
		t.Fatalf("result=%#v paths=%#v calls=%d err=%v", result, paths, writer.calls, err)
	}
}

func TestAccountRateSyncReportsUnboundAccountWithItsStoredNameWithoutReadingRemoteCatalog(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"items":[],"total":0}}`))
	}))
	defer server.Close()
	service := New(staticTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 1}}, &captureRepository{accountNames: map[string]string{"11": "Existing Account-0.15"}}, &memoryTasks{}, &captureRateWriter{})

	result, err := service.syncAccountRates(context.Background(), []string{"11"}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if result["failed"] != 1 || requests != 0 {
		t.Fatalf("result=%#v requests=%d", result, requests)
	}
	items := result["items"].([]map[string]any)
	if len(items) != 1 || items[0]["account_name"] != "Existing Account-0.15" {
		t.Fatalf("items=%#v", items)
	}
}

func TestAccountRateSyncReadsNewAPIStableTokensAndLoadsEachHostOnce(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"items":[{"id":11,"name":"alpha","rate_multiplier":0.2},{"id":12,"name":"beta","rate_multiplier":0.2}],"total":2}}`))
	}))
	defer server.Close()
	repository := &captureRepository{maintenance: []business.BoundAccountMaintenance{
		{AccountID: "11", AccountName: "alpha", UpstreamHost: "api.example", UpstreamType: "newapi", UpstreamKeyID: "101", UpstreamGroupID: "pro", RechargeRate: "2", CurrentMultiplier: "0.2"},
		{AccountID: "12", AccountName: "beta", UpstreamHost: "api.example", UpstreamType: "newapi", UpstreamKeyID: "102", UpstreamGroupID: "pro", RechargeRate: "2", CurrentMultiplier: "0.2"},
	}}
	groupRate := "0.3"
	group := "pro"
	reader := &captureCatalogReader{snapshot: business.UpstreamCatalogSnapshot{
		Groups: []business.UpstreamCatalogGroup{{GroupID: "pro", Name: "pro", RawRate: &groupRate}},
		Keys:   []business.UpstreamCatalogKey{{KeyID: "101", UpstreamGroup: &group}, {KeyID: "102", UpstreamGroup: &group}},
	}}
	writer := &captureRateWriter{}
	target := targetWithAuth{
		staticTarget: staticTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 1}},
		record:       configstore.AuthRecord{Host: "api.example", BaseURL: "https://api.example", UpstreamType: "newapi"},
	}
	service := New(target, repository, &memoryTasks{}, writer)
	service.UseUpstreamCatalogReader(reader)

	result, err := service.syncAccountRates(context.Background(), []string{"11", "12"}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if result["updated"] != 2 || result["failed"] != 0 || reader.calls != 1 || writer.values["11"] != "0.15" || writer.values["12"] != "0.15" {
		t.Fatalf("result=%#v calls=%d values=%#v", result, reader.calls, writer.values)
	}
}

func TestAccountRateSyncUsesExplicitAuthResolverForNewAPIHost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"items":[{"id":11,"name":"edge","rate_multiplier":0.15}],"total":1}}`))
	}))
	defer server.Close()
	repository := &captureRepository{maintenance: []business.BoundAccountMaintenance{{
		AccountID: "11", UpstreamHost: "primary.example", SourceAuthHost: "edge.example:8443", UpstreamType: "newapi", UpstreamKeyID: "101", RechargeRate: "1", CurrentMultiplier: "0.15",
	}}}
	group, rate := "pro", "0.15"
	reader := &captureCatalogReader{snapshot: business.UpstreamCatalogSnapshot{
		Groups: []business.UpstreamCatalogGroup{{GroupID: group, RawRate: &rate}},
		Keys:   []business.UpstreamCatalogKey{{KeyID: "101", UpstreamGroup: &group}},
	}}
	resolver := &captureAuthResolver{record: configstore.AuthRecord{Host: "edge.example:8443", BaseURL: "https://cdn.example", UpstreamType: "newapi"}}
	service := New(emptyAuthTarget{staticTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 1}}}, repository, &memoryTasks{}, &captureRateWriter{})
	service.UseUpstreamCatalogReader(reader)
	service.UseUpstreamAuthResolver(resolver)

	result, err := service.syncAccountRates(context.Background(), []string{"11"}, "operator")
	if err != nil || result["failed"] != 0 || resolver.host != "edge.example:8443" {
		t.Fatalf("result=%#v host=%q err=%v", result, resolver.host, err)
	}
}

func TestNewAPIAccountMultiplierUsesLiveTokenGroupOverLegacyBindingGroup(t *testing.T) {
	rate := "0.3"
	otherGroup := "other"
	catalog := business.UpstreamCatalogSnapshot{
		Groups: []business.UpstreamCatalogGroup{{GroupID: "other", RawRate: &rate}},
		Keys:   []business.UpstreamCatalogKey{{KeyID: "101", UpstreamGroup: &otherGroup}},
	}
	value, err := newAPIAccountMultiplier(business.BoundAccountMaintenance{
		UpstreamKeyID: "101", UpstreamGroupID: "pro", RechargeRate: "1",
	}, catalog)
	if err != nil || value != "0.3" {
		t.Fatalf("value=%q err=%v", value, err)
	}
}

func newAPIAccountMultiplier(account business.BoundAccountMaintenance, catalog business.UpstreamCatalogSnapshot) (string, error) {
	_, multiplier, err := newAPIAccountRates(account, catalog)
	return multiplier, err
}

func TestNewAPIAccountMultiplierRoundsConvertedRate(t *testing.T) {
	rate := "1.1000000000000001"
	group := "pro"
	value, err := newAPIAccountMultiplier(business.BoundAccountMaintenance{
		UpstreamKeyID: "101", UpstreamGroupID: group, RechargeRate: "10",
	}, business.UpstreamCatalogSnapshot{
		Groups: []business.UpstreamCatalogGroup{{GroupID: group, RawRate: &rate}},
		Keys:   []business.UpstreamCatalogKey{{KeyID: "101", UpstreamGroup: &group}},
	})
	if err != nil || value != "0.11" {
		t.Fatalf("value=%q err=%v", value, err)
	}
}

func TestNewAPIAccountMultiplierRejectsMultiGroupToken(t *testing.T) {
	_, err := newAPIAccountMultiplier(business.BoundAccountMaintenance{UpstreamKeyID: "101", RechargeRate: "1"}, business.UpstreamCatalogSnapshot{
		Keys: []business.UpstreamCatalogKey{{KeyID: "101", RateAmbiguous: true}},
	})
	if err == nil || !strings.Contains(err.Error(), "多分组") {
		t.Fatalf("err=%v", err)
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
	repaired := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPut {
			mu.Lock()
			putPaths = append(putPaths, request.URL.Path)
			repaired = true
			mu.Unlock()
			_, _ = w.Write([]byte(`{"success":true}`))
			return
		}
		if request.URL.Path == "/api/v1/admin/accounts/12" {
			mu.Lock()
			name := "号池-1"
			if repaired {
				name = "Anc1ent API-1"
			}
			mu.Unlock()
			_, _ = w.Write([]byte(`{"data":{"id":12,"name":"` + name + `"}}`))
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

func TestNameRepairDoesNotCommitMismatchedRemoteReadback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPut {
			_, _ = w.Write([]byte(`{"success":true}`))
			return
		}
		if request.URL.Path == "/api/v1/admin/accounts/12" {
			_, _ = w.Write([]byte(`{"data":{"id":12,"name":"号池-1"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"items":[{"id":12,"name":"号池-1"}],"total":1}}`))
	}))
	defer server.Close()
	repository := &captureRepository{maintenance: []business.BoundAccountMaintenance{
		{AccountID: "12", AccountName: "号池-1", ExpectedName: "Anc1ent API-1", UpstreamHost: "api.example"},
	}}
	service := New(staticTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 1}}, repository, &memoryTasks{})

	result, err := service.repairAccountNames(context.Background(), []string{"12"}, "operator")
	if err == nil || result["failed"] != 1 || result["remote_write"] != true {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if len(repository.repairs) != 0 {
		t.Fatalf("mismatched readback committed locally: %#v", repository.repairs)
	}
}

func TestAccountDefaultsRepairUsesSharedDefaultsForEveryPlatform(t *testing.T) {
	var mu sync.Mutex
	updates := map[string]map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPut {
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			accountID := strings.TrimPrefix(request.URL.Path, "/api/v1/admin/accounts/")
			mu.Lock()
			updates[accountID] = body
			mu.Unlock()
			_, _ = fmt.Fprintf(w, `{"data":{"id":%s,"name":"account-%s","platform":"grok","concurrency":24,"priority":7,"load_factor":null}}`, accountID, accountID)
			return
		}
		_, _ = w.Write([]byte(`{"data":{"items":[
			{"id":11,"name":"account-11","platform":"openai","concurrency":0,"priority":0,"load_factor":null},
			{"id":12,"name":"account-12","platform":"grok","concurrency":-1,"priority":-1,"load_factor":null},
			{"id":13,"name":"custom","platform":"openai","concurrency":20,"priority":3,"load_factor":8},
			{"id":14,"name":"external","platform":"openai","concurrency":0,"priority":0,"load_factor":null}
		],"total":4}}`))
	}))
	defer server.Close()
	repository := &captureRepository{maintenance: []business.BoundAccountMaintenance{
		{AccountID: "11", AccountName: "account-11", UpstreamHost: "api.example", ConsoleOnboarded: true},
		{AccountID: "12", AccountName: "account-12", UpstreamHost: "api.example", ConsoleOnboarded: true},
		{AccountID: "13", AccountName: "custom", UpstreamHost: "api.example", ConsoleOnboarded: true},
		{AccountID: "14", AccountName: "external", UpstreamHost: "api.example", ConsoleOnboarded: false},
	}}
	service := New(staticTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 1}, defaults: configstore.AccountDefaultsSettings{Concurrency: 24, Priority: 7}}, repository, &memoryTasks{})

	result, err := service.repairAccountDefaults(context.Background(), []string{"11", "12", "13", "14"}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(updates) != 2 || updates["11"]["concurrency"] != float64(24) || updates["12"]["concurrency"] != float64(24) {
		t.Fatalf("updates=%#v", updates)
	}
	for accountID, update := range updates {
		if update["priority"] != float64(7) {
			t.Errorf("account %s priority update=%#v", accountID, update["priority"])
		}
		if _, present := update["load_factor"]; present {
			t.Errorf("account %s must keep load factor unset: %#v", accountID, update)
		}
	}
	if result["repaired"] != 2 || result["unchanged"] != 1 || result["skipped"] != 1 || result["failed"] != 0 {
		t.Fatalf("result=%#v", result)
	}
	if len(repository.defaultRepairs) != 3 {
		t.Fatalf("commits=%#v", repository.defaultRepairs)
	}
	if repository.defaultRepairs[2].AccountID != "13" || repository.defaultRepairs[2].Concurrency == nil ||
		*repository.defaultRepairs[2].Concurrency != 20 || repository.defaultRepairs[2].LoadFactor == nil ||
		*repository.defaultRepairs[2].LoadFactor != "8" || repository.defaultRepairs[2].RemoteRepaired {
		t.Fatalf("unchanged custom values were not synchronized safely: %#v", repository.defaultRepairs[2])
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
