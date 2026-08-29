package management

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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
}

func (reader *captureCatalogReader) ReadCatalog(context.Context, configstore.AuthRecord) (business.UpstreamCatalogSnapshot, error) {
	reader.calls++
	return reader.snapshot, nil
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
	accountNames  map[string]string
	verifications []business.BindingVerification
	repairs       []business.AccountNameRepairCommit
	cleanedIDs    []string
	cleanupErr    error
	observations  []business.AccountRateObservation
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
	mu     sync.Mutex
	values map[string]string
	names  map[string]string
	errors map[string]error
	calls  int
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

func TestAccountRateSyncProbesUpstreamAndWritesEffectiveMultiplier(t *testing.T) {
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
			_, _ = w.Write([]byte(`{"data":{"results":[{"account_id":11,"snapshot":{"status":"ok","data":{"group_rate_multiplier":0.198,"user_rate_multiplier":0.15,"resolved_rate_multiplier":0.15,"effective_rate_multiplier":0.3}}}]}}`))
		case "/api/v1/admin/accounts":
			_, _ = w.Write([]byte(`{"data":{"items":[{"id":11,"name":"Example-0.198","rate_multiplier":0.198}],"total":1}}`))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()
	repository := &captureRepository{maintenance: []business.BoundAccountMaintenance{{
		AccountID: "11", AccountName: "alpha", UpstreamHost: "upstream.example", CurrentMultiplier: "0.198",
		NamingSiteName: "Example", NamingBaseURL: "https://upstream.example",
	}}}
	writer := &captureRateWriter{}
	service := New(staticTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 1}}, repository, &memoryTasks{}, writer)

	result, err := service.syncAccountRates(context.Background(), []string{"11"}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if result["updated"] != 1 || len(paths) != 2 || paths[0] != "/api/v1/admin/accounts/upstream-billing-probe/batch" || paths[1] != "/api/v1/admin/accounts" || writer.calls != 1 || writer.values["11"] != "0.15" || writer.names["11"] != "Example-0.15" {
		t.Fatalf("result=%#v paths=%#v calls=%d values=%#v names=%#v", result, paths, writer.calls, writer.values, writer.names)
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

func TestAccountRateSyncReportsUnboundAccountWithItsStoredName(t *testing.T) {
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
	if result["failed"] != 1 || requests != 1 {
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
		AccountID: "11", UpstreamHost: "edge.example:8443", UpstreamType: "newapi", UpstreamKeyID: "101", RechargeRate: "1", CurrentMultiplier: "0.15",
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
