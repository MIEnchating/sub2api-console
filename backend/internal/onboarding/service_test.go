package onboarding

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

type checkingKeys struct {
	databasePath string
	createCalls  int
	revealCalls  int
	verification []bool
}

type uncertainKeys struct {
	creates      int
	reconciles   int
	resolved     bool
	reconcileErr error
	name         string
}

type probeKeys struct {
	creates             int
	deletes             int
	reveals             int
	revealErr           error
	deleteErr           error
	commitOnDeleteError bool
	keepAfterDelete     bool
	listErrors          []error
	listCalls           int
	catalog             []business.UpstreamCatalogKey
	deleteIDs           []string
}

type blockingProbeKeys struct {
	*probeKeys
	deleteStarted chan struct{}
	allowDelete   chan struct{}
}

type uncertainProbeKeys struct {
	createStarted chan struct{}
	marker        string
	reconciled    int
	deleted       int
}

type cleanupTaskStore struct {
	final chan taskstore.Task
}

type auditFailingOnboardingRepository struct {
	Repository
}

func (auditFailingOnboardingRepository) RecordRuntimeEvent(context.Context, string, string, string, map[string]any) (int64, error) {
	return 0, errors.New("audit unavailable")
}

type accountAuditFailingOnboardingRepository struct {
	Repository
}

func (accountAuditFailingOnboardingRepository) RecordAccountOperation(context.Context, business.AccountOperation) error {
	return errors.New("account audit unavailable")
}

type observingAccountAuditOnboardingRepository struct {
	Repository
	contextErr error
}

func (repository *observingAccountAuditOnboardingRepository) RecordAccountOperation(ctx context.Context, _ business.AccountOperation) error {
	repository.contextErr = ctx.Err()
	return errors.New("account audit unavailable")
}

func (store *cleanupTaskStore) Save(_ context.Context, task taskstore.Task) error {
	if task.Status == "succeeded" || task.Status == "failed" || task.Status == "cancelled" {
		store.final <- task
	}
	return nil
}

func (keys *probeKeys) CreateKey(context.Context, configstore.AuthRecord, string, string) (upstreamsync.CreatedKey, error) {
	keys.creates++
	return upstreamsync.CreatedKey{KeyID: "91", Name: "console-probe", GroupID: "6", Secret: "probe-secret"}, nil
}

func (keys *probeKeys) CreateKeyWithVerification(ctx context.Context, record configstore.AuthRecord, name, groupID string, _ bool) (upstreamsync.CreatedKey, error) {
	return keys.CreateKey(ctx, record, name, groupID)
}

func (keys *probeKeys) RevealKey(context.Context, configstore.AuthRecord, string, string) (upstreamsync.CreatedKey, error) {
	keys.reveals++
	if keys.revealErr != nil {
		return upstreamsync.CreatedKey{}, keys.revealErr
	}
	return upstreamsync.CreatedKey{}, errors.New("unexpected reveal")
}

func (keys *probeKeys) DeleteKey(_ context.Context, _ configstore.AuthRecord, keyID string) error {
	keys.deletes++
	keys.deleteIDs = append(keys.deleteIDs, keyID)
	if (keys.deleteErr == nil || keys.commitOnDeleteError) && !keys.keepAfterDelete {
		filtered := keys.catalog[:0]
		for _, key := range keys.catalog {
			if key.KeyID != keyID {
				filtered = append(filtered, key)
			}
		}
		keys.catalog = filtered
	}
	return keys.deleteErr
}

func (keys *probeKeys) ListKeys(context.Context, configstore.AuthRecord) ([]business.UpstreamCatalogKey, error) {
	index := keys.listCalls
	keys.listCalls++
	if index < len(keys.listErrors) && keys.listErrors[index] != nil {
		return nil, keys.listErrors[index]
	}
	return append([]business.UpstreamCatalogKey{}, keys.catalog...), nil
}

func (keys *blockingProbeKeys) DeleteKey(ctx context.Context, record configstore.AuthRecord, keyID string) error {
	close(keys.deleteStarted)
	select {
	case <-keys.allowDelete:
	case <-ctx.Done():
		return ctx.Err()
	}
	return keys.probeKeys.DeleteKey(ctx, record, keyID)
}

func (keys *uncertainProbeKeys) CreateKey(ctx context.Context, _ configstore.AuthRecord, name, _ string) (upstreamsync.CreatedKey, error) {
	keys.marker = name
	close(keys.createStarted)
	<-ctx.Done()
	return upstreamsync.CreatedKey{}, &upstreamsync.CommitUnknownError{Marker: name, Cause: ctx.Err()}
}

func (keys *uncertainProbeKeys) CreateKeyWithVerification(ctx context.Context, record configstore.AuthRecord, name, groupID string, _ bool) (upstreamsync.CreatedKey, error) {
	return keys.CreateKey(ctx, record, name, groupID)
}

func (keys *uncertainProbeKeys) RevealKey(context.Context, configstore.AuthRecord, string, string) (upstreamsync.CreatedKey, error) {
	return upstreamsync.CreatedKey{}, errors.New("unexpected reveal")
}

func (keys *uncertainProbeKeys) ReconcileCreatedKey(ctx context.Context, _ configstore.AuthRecord, name, groupID string) (upstreamsync.CreatedKey, bool, error) {
	if ctx.Err() != nil {
		return upstreamsync.CreatedKey{}, false, errors.New("reconciliation used cancelled request context")
	}
	keys.reconciled++
	return upstreamsync.CreatedKey{KeyID: "91", Name: name, GroupID: groupID, Secret: "probe-secret"}, true, nil
}

func (keys *uncertainProbeKeys) DeleteKey(ctx context.Context, _ configstore.AuthRecord, keyID string) error {
	if ctx.Err() != nil {
		return errors.New("cleanup used cancelled request context")
	}
	if keyID != "91" {
		return fmt.Errorf("unexpected key ID %s", keyID)
	}
	keys.deleted++
	return nil
}

func (keys *checkingKeys) CreateKeyWithVerification(_ context.Context, _ configstore.AuthRecord, _, _ string, verification bool) (upstreamsync.CreatedKey, error) {
	keys.createCalls++
	keys.verification = append(keys.verification, verification)
	if err := keys.checkNoTransaction(); err != nil {
		return upstreamsync.CreatedKey{}, err
	}
	return upstreamsync.CreatedKey{KeyID: "91", Name: "pro-key", GroupID: "6", Secret: "never-store-this"}, nil
}

func (keys *checkingKeys) CreateKey(context.Context, configstore.AuthRecord, string, string) (upstreamsync.CreatedKey, error) {
	keys.createCalls++
	if err := keys.checkNoTransaction(); err != nil {
		return upstreamsync.CreatedKey{}, err
	}
	return upstreamsync.CreatedKey{KeyID: "91", Name: "pro-key", GroupID: "6", Secret: "never-store-this"}, nil
}

func (keys *checkingKeys) RevealKey(context.Context, configstore.AuthRecord, string, string) (upstreamsync.CreatedKey, error) {
	keys.revealCalls++
	if err := keys.checkNoTransaction(); err != nil {
		return upstreamsync.CreatedKey{}, err
	}
	return upstreamsync.CreatedKey{KeyID: "91", Name: "pro-key", GroupID: "6", Secret: "never-store-this"}, nil
}

func (keys *checkingKeys) checkNoTransaction() error {
	database, err := sql.Open("sqlite", "file:"+keys.databasePath+"?_pragma=busy_timeout%281%29")
	if err != nil {
		return err
	}
	defer database.Close()
	if _, err := database.Exec("BEGIN IMMEDIATE"); err != nil {
		return err
	}
	_, err = database.Exec("ROLLBACK")
	return err
}

func (keys *uncertainKeys) CreateKey(_ context.Context, _ configstore.AuthRecord, name, _ string) (upstreamsync.CreatedKey, error) {
	keys.creates++
	keys.name = name
	return upstreamsync.CreatedKey{}, &upstreamsync.CommitUnknownError{Marker: name, Cause: errors.New("connection reset")}
}

func (keys *uncertainKeys) CreateKeyWithVerification(ctx context.Context, record configstore.AuthRecord, name, groupID string, _ bool) (upstreamsync.CreatedKey, error) {
	return keys.CreateKey(ctx, record, name, groupID)
}

func (keys *uncertainKeys) ReconcileCreatedKey(_ context.Context, _ configstore.AuthRecord, name, groupID string) (upstreamsync.CreatedKey, bool, error) {
	keys.reconciles++
	if keys.reconcileErr != nil {
		return upstreamsync.CreatedKey{}, false, keys.reconcileErr
	}
	if name != keys.name || !keys.resolved {
		return upstreamsync.CreatedKey{}, false, nil
	}
	return upstreamsync.CreatedKey{KeyID: "91", Name: name, GroupID: groupID, Secret: "recovered-secret"}, true, nil
}

func (keys *uncertainKeys) RevealKey(context.Context, configstore.AuthRecord, string, string) (upstreamsync.CreatedKey, error) {
	return upstreamsync.CreatedKey{}, errors.New("unexpected stable-ID reveal")
}

func TestOnboardKeepsNetworkOutsideTransactionsAndPersistsSecretOnlyInPrivateStore(t *testing.T) {
	reads := 0
	var createdBody map[string]any
	const upstreamBaseURL = "http://192.0.2.10:8443/accelerated"
	accountBaseURL := "https://account-api.example/v1"
	admin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Header.Get("X-API-Key") != "admin-key" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts":
			_, _ = writer.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/admin/accounts":
			var body map[string]any
			decoder := json.NewDecoder(request.Body)
			decoder.UseNumber()
			if err := decoder.Decode(&body); err != nil {
				t.Errorf("decode remote account body: %v", err)
			}
			credentials, _ := body["credentials"].(map[string]any)
			if credentials["api_key"] != "never-store-this" {
				t.Errorf("remote account did not receive the one-time secret: %#v", credentials)
			}
			if credentials["base_url"] != accountBaseURL {
				t.Errorf("remote account base_url = %#v, want %q", credentials["base_url"], accountBaseURL)
			}
			if body["concurrency"] != json.Number("10") || body["priority"] != json.Number("1") {
				t.Errorf("remote account defaults = concurrency %#v priority %#v", body["concurrency"], body["priority"])
			}
			if _, present := body["load_factor"]; present {
				t.Errorf("remote account must leave load_factor unset: %#v", body["load_factor"])
			}
			createdBody = body
			_, _ = writer.Write([]byte(`{"data":{"id":77}}`))
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/admin/accounts/77/schedulable":
			createdBody["schedulable"] = false
			_, _ = writer.Write([]byte(`{"data":{"id":77,"name":"upstream-0.2","group_ids":[3],"schedulable":false,"rate_multiplier":0.2}}`))
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts/77":
			reads++
			createdBody["id"] = json.Number("77")
			_ = json.NewEncoder(writer).Encode(map[string]any{"data": createdBody})
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer admin.Close()
	repository, private, databasePath := onboardingFixture(t, admin.URL)
	token := "upstream-token"
	if err := private.SaveAuthRecord(context.Background(), configstore.AuthRecord{
		Host: "upstream.test", BaseURL: upstreamBaseURL, UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", AccessToken: &token, Headers: map[string]string{}, Cookies: map[string]string{},
	}, nil); err != nil {
		t.Fatal(err)
	}
	keys := &checkingKeys{databasePath: databasePath}
	service := New(repository, private, keys, nil)
	result, err := service.Onboard(context.Background(), Request{
		Host: "upstream.test", UpstreamType: "sub2api", LocalGroupID: "3",
		UpstreamGroupID: "6", BaseURL: &accountBaseURL, Schedulable: false, Actor: "operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result["account_id"] != "77" || result["readback_confirmed"] != true || keys.createCalls != 1 || keys.revealCalls != 0 || reads != 2 || len(keys.verification) != 1 || !keys.verification[0] {
		t.Fatalf("result=%#v keys=%#v", result, keys)
	}
	database, err := sql.Open("sqlite", "file:"+databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var accountName, keyID string
	var priority, concurrency int64
	if err := database.QueryRow(`SELECT a.name,b.upstream_key_id,a.priority,a.concurrency FROM accounts a JOIN bindings b ON b.local_account_id=a.id WHERE a.id='77'`).Scan(&accountName, &keyID, &priority, &concurrency); err != nil {
		t.Fatal(err)
	}
	if accountName != "upstream-0.2" || keyID != "91" || priority != 1 || concurrency != 10 {
		t.Fatalf("account=%q key=%q priority=%d concurrency=%d", accountName, keyID, priority, concurrency)
	}
	assertSecretAbsent(t, databasePath, "never-store-this")
	stored, err := private.UpstreamKeySecret(context.Background(), "upstream.test", "91", "6")
	if err != nil {
		t.Fatal(err)
	}
	if stored == nil || stored.Secret != "never-store-this" {
		t.Fatalf("private key cache=%#v", stored)
	}
}

func TestValidateUsesCatalogConvertedMultiplierWithoutClientInput(t *testing.T) {
	repository, private, _ := onboardingFixture(t, "http://127.0.0.1:1")
	validated, err := New(repository, private, nil, nil).validate(context.Background(), Request{
		Host: "upstream.test", UpstreamType: "sub2api", LocalGroupID: "3",
		UpstreamGroupID: "6", Schedulable: false, Actor: "operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	if validated.multiplier != "0.2" {
		t.Fatalf("validated multiplier=%q", validated.multiplier)
	}
}

func TestOnboardWaitsForCreationCatalogAndUpstreamLeases(t *testing.T) {
	for _, test := range []struct {
		name     string
		resource func() string
	}{
		{name: "account catalog", resource: mutationguard.AccountCatalog},
		{name: "stable upstream", resource: func() string { return mutationguard.Upstream("HTTPS://UPSTREAM.TEST/") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository, private, _ := onboardingFixture(t, "https://admin.example")
			_, release, err := mutationguard.Acquire(context.Background(), repository, test.resource())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = release() }()
			keys := &probeKeys{}
			ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
			defer cancel()
			result, err := New(repository, private, keys, nil).Onboard(ctx, Request{
				Host: "upstream.test", UpstreamType: "sub2api", LocalGroupID: "3",
				UpstreamGroupID: "6", Actor: "operator",
			})
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("result=%#v err=%v", result, err)
			}
			if keys.creates != 0 {
				t.Fatalf("created %d keys before acquiring %s lease", keys.creates, test.name)
			}
		})
	}
}

func TestOnboardWaitsForExistingAccountLeaseBeforeRemoteGroupWrite(t *testing.T) {
	var adminCalls atomic.Int32
	admin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		adminCalls.Add(1)
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer admin.Close()
	repository, private, databasePath := onboardingFixture(t, admin.URL)
	database, err := sql.Open("sqlite", "file:"+databasePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO accounts(id,name,upstream_host,upstream_type,metadata_json,updated_at) VALUES('77','existing','upstream.test','sub2api','{}','now')`,
		`INSERT INTO account_groups(account_id,group_name,group_id) VALUES('77','codex','3')`,
		`INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,upstream_group,upstream_group_id,local_group,status,updated_at) VALUES('77','upstream.test','91','pro-key','pro','6','codex','active','now')`,
	} {
		if _, err := database.Exec(statement); err != nil {
			_ = database.Close()
			t.Fatal(err)
		}
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	_, release, err := mutationguard.Acquire(context.Background(), repository, mutationguard.Account("77"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = release() }()
	keys := &probeKeys{}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	result, err := New(repository, private, keys, nil).Onboard(ctx, Request{
		Host: "upstream.test", UpstreamType: "sub2api", LocalGroupIDs: []string{"4"},
		UpstreamGroupID: "6", AccountIDs: []string{"77"}, Actor: "operator",
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if calls := adminCalls.Load(); calls != 0 {
		t.Fatalf("remote account API called %d times before acquiring account lease", calls)
	}
	if keys.creates != 0 {
		t.Fatalf("created %d keys while updating an existing account", keys.creates)
	}
}

func TestOnboardCreatesOneAccountInEverySelectedLocalGroup(t *testing.T) {
	var createdBody map[string]any
	admin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts":
			_, _ = writer.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/admin/accounts":
			var body map[string]any
			decoder := json.NewDecoder(request.Body)
			decoder.UseNumber()
			if err := decoder.Decode(&body); err != nil {
				t.Fatal(err)
			}
			groups, _ := body["group_ids"].([]any)
			if len(groups) != 2 || groups[0] != json.Number("3") || groups[1] != json.Number("4") {
				t.Errorf("group_ids=%#v", body["group_ids"])
			}
			createdBody = body
			_, _ = writer.Write([]byte(`{"data":{"id":77}}`))
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts/77":
			createdBody["id"] = json.Number("77")
			_ = json.NewEncoder(writer).Encode(map[string]any{"data": createdBody})
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/admin/accounts/77/schedulable":
			createdBody["schedulable"] = false
			_, _ = writer.Write([]byte(`{"data":{"id":77,"name":"upstream-0.2","group_ids":[3,4],"schedulable":false,"rate_multiplier":0.2}}`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer admin.Close()
	repository, private, databasePath := onboardingFixture(t, admin.URL)
	keys := &checkingKeys{databasePath: databasePath}
	result, err := New(repository, private, keys, nil).Onboard(context.Background(), Request{
		Host: "upstream.test", UpstreamType: "sub2api", LocalGroupIDs: []string{"3", "4"},
		UpstreamGroupID: "6", Schedulable: false, Actor: "operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	if keys.createCalls != 1 || result["account_id"] != "77" {
		t.Fatalf("result=%#v createCalls=%d", result, keys.createCalls)
	}
	database, err := sql.Open("sqlite", "file:"+databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var memberships int
	if err := database.QueryRow(`SELECT COUNT(*) FROM account_groups WHERE account_id='77' AND group_id IN ('3','4')`).Scan(&memberships); err != nil || memberships != 2 {
		t.Fatalf("memberships=%d err=%v", memberships, err)
	}
}

func TestOnboardUpdatesExistingAccountGroupsWithoutCreatingAnotherKey(t *testing.T) {
	reads := 0
	const accountBaseURL = "https://account-api.example/v1"
	admin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts/77":
			reads++
			groups := `[3]`
			if reads > 1 {
				groups = `[4]`
			}
			credentials := `{"base_url":"https://old.example/v1"}`
			if reads > 1 {
				credentials = `{"base_url":"` + accountBaseURL + `"}`
			}
			_, _ = writer.Write([]byte(`{"data":{"id":77,"name":"existing","group_ids":` + groups + `,"credentials":` + credentials + `}}`))
		case request.Method == http.MethodPut && request.URL.Path == "/api/v1/admin/accounts/77":
			var body map[string]any
			decoder := json.NewDecoder(request.Body)
			decoder.UseNumber()
			if err := decoder.Decode(&body); err != nil {
				t.Fatal(err)
			}
			groups, _ := body["group_ids"].([]any)
			if len(groups) != 1 || groups[0] != json.Number("4") {
				t.Errorf("group_ids=%#v", body["group_ids"])
			}
			credentials, _ := body["credentials"].(map[string]any)
			if credentials["base_url"] != accountBaseURL {
				t.Errorf("credentials=%#v", credentials)
			}
			_, _ = writer.Write([]byte(`{"data":{"id":77}}`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer admin.Close()
	repository, private, databasePath := onboardingFixture(t, admin.URL)
	database, err := sql.Open("sqlite", "file:"+databasePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO accounts(id,name,upstream_host,upstream_type,metadata_json,updated_at) VALUES('77','existing','upstream.test','sub2api','{}','now')`,
		`INSERT INTO account_groups(account_id,group_name,group_id) VALUES('77','codex','3')`,
		`INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,upstream_group,upstream_group_id,local_group,status,updated_at) VALUES('77','upstream.test','91','pro-key','pro','6','codex','active','now')`,
	} {
		if _, err := database.Exec(statement); err != nil {
			database.Close()
			t.Fatal(err)
		}
	}
	_ = database.Close()
	keys := &checkingKeys{databasePath: databasePath}
	baseURL := accountBaseURL
	result, err := New(repository, private, keys, nil).Onboard(context.Background(), Request{
		Host: "upstream.test", UpstreamType: "sub2api", LocalGroupIDs: []string{"4"},
		UpstreamGroupID: "6", AccountIDs: []string{"77"}, BaseURL: &baseURL, Actor: "operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	if keys.createCalls != 0 || result["operation"] != "account.groups" || reads != 2 {
		t.Fatalf("result=%#v keys=%#v reads=%d", result, keys, reads)
	}
	database, err = sql.Open("sqlite", "file:"+databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var groupID, bindingGroup, metadataRaw string
	if err := database.QueryRow(`SELECT group_id FROM account_groups WHERE account_id='77'`).Scan(&groupID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`SELECT local_group FROM bindings WHERE local_account_id='77'`).Scan(&bindingGroup); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`SELECT metadata_json FROM accounts WHERE id='77'`).Scan(&metadataRaw); err != nil {
		t.Fatal(err)
	}
	if groupID != "4" || bindingGroup != "pro" {
		t.Fatalf("groupID=%q bindingGroup=%q", groupID, bindingGroup)
	}
	if !strings.Contains(metadataRaw, `"base_url":"`+accountBaseURL+`"`) || !strings.Contains(metadataRaw, `"base_url_source":"explicit"`) {
		t.Fatalf("metadata=%s", metadataRaw)
	}
}

func TestExistingAccountGroupUpdateReportsEarlierWritesWhenLaterReadFails(t *testing.T) {
	reads := 0
	admin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts/77":
			reads++
			groups := `[3]`
			if reads > 1 {
				groups = `[4]`
			}
			_, _ = writer.Write([]byte(`{"data":{"id":77,"name":"first","group_ids":` + groups + `}}`))
		case request.Method == http.MethodPut && request.URL.Path == "/api/v1/admin/accounts/77":
			_, _ = writer.Write([]byte(`{"success":true}`))
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts/78":
			http.Error(writer, `{"message":"account unavailable"}`, http.StatusServiceUnavailable)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer admin.Close()
	repository, private, databasePath := onboardingFixture(t, admin.URL)
	database, err := sql.Open("sqlite", "file:"+databasePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO accounts(id,name,upstream_host,upstream_type,metadata_json,updated_at) VALUES('77','first','upstream.test','sub2api','{}','now')`,
		`INSERT INTO accounts(id,name,upstream_host,upstream_type,metadata_json,updated_at) VALUES('78','second','upstream.test','sub2api','{}','now')`,
		`INSERT INTO account_groups(account_id,group_name,group_id) VALUES('77','codex','3')`,
		`INSERT INTO account_groups(account_id,group_name,group_id) VALUES('78','codex','3')`,
		`INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,upstream_group,upstream_group_id,local_group,status,updated_at) VALUES('77','upstream.test','91','first-key','pro','6','codex','active','now')`,
		`INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,upstream_group,upstream_group_id,local_group,status,updated_at) VALUES('78','upstream.test','92','second-key','pro','6','codex','active','now')`,
	} {
		if _, err := database.Exec(statement); err != nil {
			_ = database.Close()
			t.Fatal(err)
		}
	}
	_ = database.Close()
	groupID := "6"
	result, err := New(repository, private, &probeKeys{}, nil).updateAccountGroups(context.Background(), validatedRequest{
		request:   Request{AccountIDs: []string{"77", "78"}, Actor: "operator"},
		locals:    []business.LocalOnboardingGroup{{ID: "4", Name: "pro"}},
		candidate: business.OnboardingCandidate{GroupID: &groupID, GroupName: "pro"},
	})
	if err == nil {
		t.Fatal("later account read failure was accepted")
	}
	accounts, ok := result["accounts"].([]map[string]any)
	if result["remote_write"] != true || result["readback_confirmed"] != false || !ok || len(accounts) != 1 || accounts[0]["account_id"] != "77" {
		t.Fatalf("partial update result=%#v err=%v", result, err)
	}
}

func TestExistingAccountGroupUpdateReportsFailureAuditError(t *testing.T) {
	admin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts/77":
			_, _ = writer.Write([]byte(`{"data":{"id":77,"name":"existing","group_ids":[3]}}`))
		case request.Method == http.MethodPut && request.URL.Path == "/api/v1/admin/accounts/77":
			http.Error(writer, `{"message":"remote update failed"}`, http.StatusInternalServerError)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer admin.Close()
	repository, private, _ := onboardingFixture(t, admin.URL)
	groupID := "6"
	service := New(accountAuditFailingOnboardingRepository{Repository: repository}, private, &probeKeys{}, nil)
	result, err := service.updateAccountGroups(context.Background(), validatedRequest{
		request:   Request{AccountIDs: []string{"77"}, Actor: "operator"},
		locals:    []business.LocalOnboardingGroup{{ID: "4", Name: "pro"}},
		candidate: business.OnboardingCandidate{GroupID: &groupID, GroupName: "pro"},
	})
	if err == nil || !strings.Contains(err.Error(), "account audit unavailable") {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if result["remote_write"] != true || result["readback_confirmed"] != false {
		t.Fatalf("result=%#v", result)
	}
}

func TestOnboardingFailureAuditOutlivesCancelledWorkContext(t *testing.T) {
	repository, private, _ := onboardingFixture(t, "https://admin.example")
	observing := &observingAccountAuditOnboardingRepository{Repository: repository}
	service := New(observing, private, &probeKeys{}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	groupID := "6"
	service.recordFailure(ctx, "operation-1", validatedRequest{
		request:        Request{Actor: "operator"},
		locals:         []business.LocalOnboardingGroup{{ID: "4", Name: "pro"}},
		candidate:      business.OnboardingCandidate{GroupID: &groupID, GroupName: "pro"},
		auth:           configstore.AuthRecord{Host: "upstream.test"},
		accountBaseURL: "https://account.example",
	}, "remote-write", true, errors.New("remote failed"))
	if observing.contextErr != nil {
		t.Fatalf("failure audit inherited cancelled context: %v", observing.contextErr)
	}
}

func TestProbeBeforeOnboardingUsesAndCleansTemporaryKeys(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Header.Get("Authorization") != "Bearer probe-secret" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/v1/models":
			_, _ = writer.Write([]byte(`{"data":[{"id":"gpt-5.2"},{"id":"gpt-5.1-codex"}]}`))
		case "/v1/responses":
			_, _ = writer.Write([]byte(`{"model":"gpt-5.2","output":[{"content":[{"text":"ok"}]}]}`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer gateway.Close()
	repository, private, databasePath := onboardingFixture(t, gateway.URL)
	token := "upstream-token"
	if err := private.SaveAuthRecord(context.Background(), configstore.AuthRecord{
		Host: "upstream.test", BaseURL: gateway.URL, UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", AccessToken: &token, Headers: map[string]string{}, Cookies: map[string]string{},
	}, nil); err != nil {
		t.Fatal(err)
	}
	keys := &probeKeys{}
	service := New(repository, private, keys, nil)
	models, err := service.ProbeModels(context.Background(), "upstream.test", "6")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(models, ",") != "gpt-5.1-codex,gpt-5.2" {
		t.Fatalf("models=%v", models)
	}
	result, err := service.Probe(context.Background(), "upstream.test", "6", "gpt-5.2")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "passed" || result.HTTPStatus != http.StatusOK || result.RequestModel != "gpt-5.2" || result.ActualModel != "gpt-5.2" || !result.TemporaryKey {
		t.Fatalf("result=%#v", result)
	}
	if keys.creates != 2 || keys.deletes != 2 {
		t.Fatalf("creates=%d deletes=%d", keys.creates, keys.deletes)
	}
	database, err := sql.Open("sqlite", "file:"+databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var accounts int
	if err := database.QueryRow(`SELECT COUNT(*) FROM accounts`).Scan(&accounts); err != nil || accounts != 0 {
		t.Fatalf("accounts=%d err=%v", accounts, err)
	}
	assertSecretAbsent(t, databasePath, "probe-secret")
}

func TestProbeHoldsCanonicalUpstreamLeaseUntilTemporaryKeyCleanupFinishes(t *testing.T) {
	requestStarted := make(chan struct{})
	allowResponse := make(chan struct{})
	gateway := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		close(requestStarted)
		<-allowResponse
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"data":[{"id":"gpt-5.2"}]}`))
	}))
	defer gateway.Close()
	repository, private, _ := onboardingFixture(t, gateway.URL)
	token := "upstream-token"
	if err := private.SaveAuthRecord(context.Background(), configstore.AuthRecord{
		Host: "upstream.test", BaseURL: gateway.URL, UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", AccessToken: &token, Headers: map[string]string{}, Cookies: map[string]string{},
	}, nil); err != nil {
		t.Fatal(err)
	}
	keys := &blockingProbeKeys{
		probeKeys: &probeKeys{}, deleteStarted: make(chan struct{}), allowDelete: make(chan struct{}),
	}
	probeDone := make(chan error, 1)
	go func() {
		_, err := New(repository, private, keys, nil).ProbeModels(context.Background(), "HTTPS://UPSTREAM.TEST/", "6")
		probeDone <- err
	}()
	<-requestStarted
	secondAcquired := make(chan func() error, 1)
	go func() {
		_, release, err := mutationguard.Acquire(context.Background(), repository, mutationguard.Upstream("upstream.test"))
		if err != nil {
			secondAcquired <- func() error { return err }
			return
		}
		secondAcquired <- release
	}()
	select {
	case <-secondAcquired:
		t.Fatal("canonical upstream lease was released while the probe request was active")
	case <-time.After(50 * time.Millisecond):
	}
	close(allowResponse)
	<-keys.deleteStarted
	select {
	case <-secondAcquired:
		t.Fatal("canonical upstream lease was released before temporary Key cleanup finished")
	case <-time.After(50 * time.Millisecond):
	}
	close(keys.allowDelete)
	if err := <-probeDone; err != nil {
		t.Fatal(err)
	}
	select {
	case release := <-secondAcquired:
		if err := release(); err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waiting upstream mutation did not resume after probe cleanup")
	}
}

func TestProbeRequestCancellationStillCleansTemporaryKey(t *testing.T) {
	requestStarted := make(chan struct{})
	gateway := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(requestStarted)
		<-request.Context().Done()
	}))
	defer gateway.Close()
	repository, private, _ := onboardingFixture(t, gateway.URL)
	token := "upstream-token"
	if err := private.SaveAuthRecord(context.Background(), configstore.AuthRecord{
		Host: "upstream.test", BaseURL: gateway.URL, UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", AccessToken: &token, Headers: map[string]string{}, Cookies: map[string]string{},
	}, nil); err != nil {
		t.Fatal(err)
	}
	keys := &probeKeys{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := New(repository, private, keys, nil).ProbeModels(ctx, "upstream.test", "6")
		done <- err
	}()
	<-requestStarted
	cancel()
	select {
	case err := <-done:
		if err == nil || keys.creates != 1 || keys.deletes != 1 {
			t.Fatalf("err=%v creates=%d deletes=%d", err, keys.creates, keys.deletes)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled probe did not finish cleanup")
	}
}

func TestProbeCommitUnknownUsesIndependentMarkerReconciliationAndCleanup(t *testing.T) {
	repository, private, _ := onboardingFixture(t, "https://gateway.invalid")
	keys := &uncertainProbeKeys{createStarted: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := New(repository, private, keys, nil).ProbeModels(ctx, "HTTPS://UPSTREAM.TEST/", "6")
		done <- err
	}()
	<-keys.createStarted
	cancel()
	select {
	case err := <-done:
		if err == nil || keys.reconciled != 1 || keys.deleted != 1 {
			t.Fatalf("err=%v reconciled=%d deleted=%d", err, keys.reconciled, keys.deleted)
		}
		if keys.marker != probeKeyMarker("upstream.test", "6") || keys.marker != probeKeyMarker("HTTPS://UPSTREAM.TEST/", "6") {
			t.Fatalf("marker=%q", keys.marker)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("commit-unknown probe did not reconcile and clean up")
	}
}

func TestProbeIgnoresTemporaryKeyLeftInLocalCatalog(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Header.Get("Authorization") != "Bearer probe-secret" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = writer.Write([]byte(`{"data":[{"id":"gpt-5.2"}]}`))
	}))
	defer gateway.Close()
	repository, private, databasePath := onboardingFixture(t, gateway.URL)
	token := "upstream-token"
	if err := private.SaveAuthRecord(context.Background(), configstore.AuthRecord{
		Host: "upstream.test", BaseURL: gateway.URL, UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", AccessToken: &token, Headers: map[string]string{}, Cookies: map[string]string{},
	}, nil); err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("sqlite", "file:"+databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO upstream_keys(host,key_id,name,upstream_group,status,updated_at)
		VALUES('upstream.test','6608','console-probe-b018dbf986fd','6','active','now')`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	_ = database.Close()

	keys := &probeKeys{}
	models, err := New(repository, private, keys, nil).ProbeModels(context.Background(), "upstream.test", "6")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(models, ",") != "gpt-5.2" || keys.reveals != 0 || keys.creates != 1 || keys.deletes != 1 {
		t.Fatalf("models=%v reveals=%d creates=%d deletes=%d", models, keys.reveals, keys.creates, keys.deletes)
	}
}

func TestProbeReplacesExistingKeyConfirmedMissingUpstream(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Header.Get("Authorization") != "Bearer probe-secret" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = writer.Write([]byte(`{"data":[{"id":"gpt-5.2"}]}`))
	}))
	defer gateway.Close()
	repository, private, databasePath := onboardingFixture(t, gateway.URL)
	token := "upstream-token"
	if err := private.SaveAuthRecord(context.Background(), configstore.AuthRecord{
		Host: "upstream.test", BaseURL: gateway.URL, UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", AccessToken: &token, Headers: map[string]string{}, Cookies: map[string]string{},
	}, nil); err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("sqlite", "file:"+databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO upstream_keys(host,key_id,name,upstream_group,status,updated_at)
		VALUES('upstream.test','6608','permanent-key','6','active','now')`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	_ = database.Close()

	keys := &probeKeys{revealErr: upstreamsync.ErrKeyNotFound}
	models, err := New(repository, private, keys, nil).ProbeModels(context.Background(), "upstream.test", "6")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(models, ",") != "gpt-5.2" || keys.reveals != 1 || keys.creates != 1 || keys.deletes != 1 {
		t.Fatalf("models=%v reveals=%d creates=%d deletes=%d", models, keys.reveals, keys.creates, keys.deletes)
	}
}

func TestProbeDoesNotReplaceExistingKeyAfterUnclassifiedReadFailure(t *testing.T) {
	repository, private, databasePath := onboardingFixture(t, "https://admin.example")
	database, err := sql.Open("sqlite", "file:"+databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO upstream_keys(host,key_id,name,upstream_group,status,updated_at)
		VALUES('upstream.test','6608','permanent-key','6','active','now')`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	_ = database.Close()

	keys := &probeKeys{revealErr: errors.New("upstream timeout")}
	_, err = New(repository, private, keys, nil).ProbeModels(context.Background(), "upstream.test", "6")
	if err == nil || !strings.Contains(err.Error(), "upstream timeout") {
		t.Fatalf("err=%v", err)
	}
	if keys.reveals != 1 || keys.creates != 0 || keys.deletes != 0 {
		t.Fatalf("reveals=%d creates=%d deletes=%d", keys.reveals, keys.creates, keys.deletes)
	}
}

func TestProbeCleansTemporaryKeyWhenGatewayFails(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadGateway)
		_, _ = writer.Write([]byte(`{"error":"upstream unavailable"}`))
	}))
	defer gateway.Close()
	repository, private, _ := onboardingFixture(t, gateway.URL)
	token := "upstream-token"
	if err := private.SaveAuthRecord(context.Background(), configstore.AuthRecord{
		Host: "upstream.test", BaseURL: gateway.URL, UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", AccessToken: &token, Headers: map[string]string{}, Cookies: map[string]string{},
	}, nil); err != nil {
		t.Fatal(err)
	}
	keys := &probeKeys{}
	_, err := New(repository, private, keys, nil).Probe(context.Background(), "upstream.test", "6", "gpt-5.2")
	if err == nil || !strings.Contains(err.Error(), "HTTP 502") {
		t.Fatalf("err=%v", err)
	}
	if keys.creates != 1 || keys.deletes != 1 {
		t.Fatalf("creates=%d deletes=%d", keys.creates, keys.deletes)
	}
}

func TestOnboardAlwaysVerifiesSchedulingWithFullReadback(t *testing.T) {
	reads := 0
	var createdBody map[string]any
	admin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts":
			_, _ = writer.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/admin/accounts":
			decoder := json.NewDecoder(request.Body)
			decoder.UseNumber()
			if err := decoder.Decode(&createdBody); err != nil {
				t.Fatal(err)
			}
			_, _ = writer.Write([]byte(`{"data":{"id":77}}`))
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/admin/accounts/77/schedulable":
			createdBody["schedulable"] = false
			_, _ = writer.Write([]byte(`{"success":true}`))
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts/77":
			reads++
			createdBody["id"] = json.Number("77")
			_ = json.NewEncoder(writer).Encode(map[string]any{"data": createdBody})
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer admin.Close()
	repository, private, databasePath := onboardingFixture(t, admin.URL)
	if _, err := repository.UpdatePolicy(context.Background(), map[string]any{
		"advanced_policy": map[string]any{"writeback": map[string]any{"verification": true}},
	}, "test"); err != nil {
		t.Fatal(err)
	}
	keys := &checkingKeys{databasePath: databasePath}
	result, err := New(repository, private, keys, nil).Onboard(context.Background(), Request{
		Host: "upstream.test", UpstreamType: "sub2api", LocalGroupID: "3",
		UpstreamGroupID: "6", Schedulable: false, Actor: "operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result["remote_write"] != true || result["readback_confirmed"] != true || reads != 2 || len(keys.verification) != 1 || !keys.verification[0] {
		t.Fatalf("result=%#v reads=%d verification=%v", result, reads, keys.verification)
	}
}

func TestOnboardReusesPendingKeyAfterRemoteAccountFailure(t *testing.T) {
	admin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadGateway)
		_, _ = writer.Write([]byte(`{"error":"account service unavailable"}`))
	}))
	defer admin.Close()
	repository, private, databasePath := onboardingFixture(t, admin.URL)
	keys := &checkingKeys{databasePath: databasePath}
	service := New(repository, private, keys, nil)
	request := Request{
		Host: "upstream.test", UpstreamType: "sub2api", LocalGroupID: "3",
		UpstreamGroupID: "6", Schedulable: false, Actor: "operator",
	}
	first, firstErr := service.Onboard(context.Background(), request)
	second, secondErr := service.Onboard(context.Background(), request)
	if firstErr == nil || secondErr == nil {
		t.Fatalf("first=%v second=%v", firstErr, secondErr)
	}
	if keys.createCalls != 1 || keys.revealCalls != 1 {
		t.Fatalf("pending key was not reused: create=%d reveal=%d", keys.createCalls, keys.revealCalls)
	}
	if !strings.Contains(firstErr.Error(), "禁止重复创建") || first["pending"] == nil || second["pending"] == nil {
		t.Fatalf("first=%#v err=%v second=%#v err=%v", first, firstErr, second, secondErr)
	}
	database, err := sql.Open("sqlite", "file:"+databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var failures int
	if err := database.QueryRow(`SELECT COUNT(*) FROM operation_audit WHERE operation_type='account.onboarding' AND state='failed' AND object_id IS NULL`).Scan(&failures); err != nil || failures != 2 {
		t.Fatalf("failure audit count=%d err=%v", failures, err)
	}
	assertSecretAbsent(t, databasePath, "never-store-this")
}

func TestOnboardRejectsChangedRetryAgainstFrozenCreationIntent(t *testing.T) {
	var accountPosts int
	admin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts" {
			_, _ = writer.Write([]byte(`{"data":{"items":[],"total":0}}`))
			return
		}
		if request.Method == http.MethodPost && request.URL.Path == "/api/v1/admin/accounts" {
			accountPosts++
			http.Error(writer, `{"message":"validation rejected"}`, http.StatusUnprocessableEntity)
			return
		}
		writer.WriteHeader(http.StatusNotFound)
	}))
	defer admin.Close()
	repository, private, databasePath := onboardingFixture(t, admin.URL)
	keys := &checkingKeys{databasePath: databasePath}
	service := New(repository, private, keys, nil)
	request := Request{
		Host: "upstream.test", UpstreamType: "sub2api", LocalGroupID: "3",
		UpstreamGroupID: "6", Schedulable: false, Actor: "operator",
	}
	if _, err := service.Onboard(context.Background(), request); err == nil {
		t.Fatal("first explicit account failure must remain pending")
	}
	request.Schedulable = true
	_, err := service.Onboard(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "首次冻结的开户意图不一致") {
		t.Fatalf("err=%v", err)
	}
	if accountPosts != 1 || keys.createCalls != 1 || keys.revealCalls != 0 {
		t.Fatalf("accountPosts=%d create=%d reveal=%d", accountPosts, keys.createCalls, keys.revealCalls)
	}
	database, err := sql.Open("sqlite", "file:"+databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var intentHash string
	if err := database.QueryRow(`SELECT intent_hash FROM onboarding_pending`).Scan(&intentHash); err != nil || len(intentHash) != 64 {
		t.Fatalf("intentHash=%q err=%v", intentHash, err)
	}
}

func TestOnboardReusesPendingIdentityWhenLocalGroupOrderChanges(t *testing.T) {
	var accountPosts int
	admin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts":
			_, _ = writer.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/admin/accounts":
			accountPosts++
			http.Error(writer, `{"message":"validation rejected"}`, http.StatusUnprocessableEntity)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer admin.Close()
	repository, private, databasePath := onboardingFixture(t, admin.URL)
	keys := &checkingKeys{databasePath: databasePath}
	service := New(repository, private, keys, nil)
	request := Request{
		Host: "upstream.test", UpstreamType: "sub2api", LocalGroupIDs: []string{"4", "3"},
		UpstreamGroupID: "6", Schedulable: false, Actor: "operator",
	}
	first, firstErr := service.Onboard(context.Background(), request)
	if firstErr == nil {
		t.Fatal("first explicit account failure must remain pending")
	}
	request.LocalGroupIDs = []string{"3", "4"}
	second, secondErr := service.Onboard(context.Background(), request)
	if secondErr == nil {
		t.Fatal("second explicit account failure must remain pending")
	}
	firstPending, firstOK := first["pending"].(map[string]any)
	secondPending, secondOK := second["pending"].(map[string]any)
	if !firstOK || !secondOK || firstPending["operation_id"] != secondPending["operation_id"] {
		t.Fatalf("first=%#v second=%#v", first, second)
	}
	if first["remote_write"] != true || second["remote_write"] != true || keys.createCalls != 1 || keys.revealCalls != 1 || accountPosts != 2 {
		t.Fatalf("first=%#v second=%#v create=%d reveal=%d posts=%d", first, second, keys.createCalls, keys.revealCalls, accountPosts)
	}
	var selection string
	database, err := sql.Open("sqlite", "file:"+databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.QueryRow(`SELECT local_group_ids_json FROM onboarding_pending`).Scan(&selection); err != nil {
		t.Fatal(err)
	}
	if selection != `["3","4"]` {
		t.Fatalf("selection=%q", selection)
	}
}

func TestOnboardMultiplierChangeFindsFrozenPendingBeforeAnyNewRemoteCall(t *testing.T) {
	var accountPosts int
	admin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts":
			_, _ = writer.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/admin/accounts":
			accountPosts++
			http.Error(writer, `{"message":"validation rejected"}`, http.StatusUnprocessableEntity)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer admin.Close()
	repository, private, databasePath := onboardingFixture(t, admin.URL)
	keys := &checkingKeys{databasePath: databasePath}
	service := New(repository, private, keys, nil)
	request := Request{
		Host: "upstream.test", UpstreamType: "sub2api", LocalGroupID: "3",
		UpstreamGroupID: "6", Schedulable: false, Actor: "operator",
	}
	if _, err := service.Onboard(context.Background(), request); err == nil {
		t.Fatal("first explicit account failure must remain pending")
	}
	database, err := sql.Open("sqlite", "file:"+databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE upstream_groups SET raw_rate='0.3',effective_rate='0.3' WHERE host='upstream.test' AND group_id='6'`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	result, err := service.Onboard(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "首次冻结的开户意图不一致") {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if result["remote_write"] != true || keys.createCalls != 1 || keys.revealCalls != 0 || accountPosts != 1 {
		t.Fatalf("result=%#v create=%d reveal=%d posts=%d", result, keys.createCalls, keys.revealCalls, accountPosts)
	}
}

func TestOnboardRejectsPendingWithoutFrozenIntentBeforeRemoteCalls(t *testing.T) {
	var adminCalls int
	admin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		adminCalls++
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer admin.Close()
	repository, private, databasePath := onboardingFixture(t, admin.URL)
	database, err := sql.Open("sqlite", "file:"+databasePath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.Exec(`INSERT INTO onboarding_pending(
		operation_id,upstream_host,upstream_type,upstream_key_id,upstream_key_name,upstream_group_id,upstream_group_name,
		local_group_id,local_group_name,local_group_ids_json,multiplier,intent_hash,reason,created_at,updated_at
	) VALUES('incomplete','upstream.test','sub2api','91','legacy-marker','6','pro','3','codex','["3"]','0.2','','incomplete','now','now')`)
	if err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	keys := &checkingKeys{databasePath: databasePath}
	result, err := New(repository, private, keys, nil).Onboard(context.Background(), Request{
		Host: "upstream.test", UpstreamType: "sub2api", LocalGroupID: "3",
		UpstreamGroupID: "6", Schedulable: false, Actor: "operator",
	})
	if err == nil || !strings.Contains(err.Error(), "缺少首次冻结意图") {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if keys.createCalls != 0 || keys.revealCalls != 0 || adminCalls != 0 {
		t.Fatalf("create=%d reveal=%d adminCalls=%d", keys.createCalls, keys.revealCalls, adminCalls)
	}
}

func TestOnboardPersistsUnknownKeyIntentAndNeverBlindlyCreatesAgain(t *testing.T) {
	var createdBody map[string]any
	admin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts":
			_, _ = writer.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/admin/accounts":
			decoder := json.NewDecoder(request.Body)
			decoder.UseNumber()
			if err := decoder.Decode(&createdBody); err != nil {
				t.Fatal(err)
			}
			_, _ = writer.Write([]byte(`{"data":{"id":77}}`))
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts/77":
			createdBody["id"] = json.Number("77")
			_ = json.NewEncoder(writer).Encode(map[string]any{"data": createdBody})
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/admin/accounts/77/schedulable":
			createdBody["schedulable"] = false
			_, _ = writer.Write([]byte(`{"success":true}`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer admin.Close()
	repository, private, databasePath := onboardingFixture(t, admin.URL)
	keys := &uncertainKeys{}
	service := New(repository, private, keys, nil)
	request := Request{
		Host: "upstream.test", UpstreamType: "sub2api", LocalGroupID: "3",
		UpstreamGroupID: "6", Schedulable: false, Actor: "operator",
	}
	first, err := service.Onboard(context.Background(), request)
	if err == nil {
		t.Fatal("unknown Key commit must remain pending")
	}
	if first["remote_write"] != true {
		t.Fatalf("first result=%#v err=%v", first, err)
	}
	mismatched := request
	mismatched.Schedulable = true
	mismatchResult, err := service.Onboard(context.Background(), mismatched)
	if err == nil || !strings.Contains(err.Error(), "首次冻结的开户意图不一致") {
		t.Fatalf("mismatch result=%#v err=%v", mismatchResult, err)
	}
	if mismatchResult["remote_write"] != true || keys.reconciles != 0 {
		t.Fatalf("mismatch result=%#v reconciles=%d", mismatchResult, keys.reconciles)
	}
	keys.reconcileErr = errors.New("inventory unavailable")
	second, err := service.Onboard(context.Background(), request)
	if err == nil {
		t.Fatal("failed Key reconciliation must remain fail-closed")
	}
	if second["remote_write"] != true {
		t.Fatalf("second result=%#v err=%v", second, err)
	}
	if keys.creates != 1 || keys.reconciles != 1 {
		t.Fatalf("creates=%d reconciles=%d", keys.creates, keys.reconciles)
	}
	database, err := sql.Open("sqlite", "file:"+databasePath)
	if err != nil {
		t.Fatal(err)
	}
	var keyID string
	var keyUnknown bool
	if err := database.QueryRow(`SELECT upstream_key_id,key_commit_unknown FROM onboarding_pending`).Scan(&keyID, &keyUnknown); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if keyID != "" || !keyUnknown {
		database.Close()
		t.Fatalf("keyID=%q keyUnknown=%t", keyID, keyUnknown)
	}
	var remoteConfirmedFailures int
	if err := database.QueryRow(`SELECT COUNT(*) FROM operation_audit
		WHERE operation_id=(SELECT operation_id FROM onboarding_pending LIMIT 1)
		AND operation_type='account.onboarding' AND state='failed' AND remote_confirmed=1`).Scan(&remoteConfirmedFailures); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if remoteConfirmedFailures != 2 {
		database.Close()
		t.Fatalf("remote-confirmed failure audits=%d", remoteConfirmedFailures)
	}
	_ = database.Close()

	keys.reconcileErr = nil
	keys.resolved = true
	result, err := service.Onboard(context.Background(), request)
	if err != nil || result["account_id"] != "77" || keys.creates != 1 || keys.reconciles != 2 {
		t.Fatalf("result=%#v err=%v creates=%d reconciles=%d", result, err, keys.creates, keys.reconciles)
	}
}

func TestOnboardReconcilesUnknownAccountCommitBeforeAnySecondCreate(t *testing.T) {
	var account map[string]any
	var visible bool
	var failCatalog bool
	var posts int
	admin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts":
			if failCatalog {
				http.Error(writer, `{"message":"catalog unavailable"}`, http.StatusServiceUnavailable)
				return
			}
			items := []map[string]any{}
			if visible && account != nil {
				items = append(items, account)
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"items": items, "total": len(items)}})
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/admin/accounts":
			posts++
			decoder := json.NewDecoder(request.Body)
			decoder.UseNumber()
			if err := decoder.Decode(&account); err != nil {
				t.Fatal(err)
			}
			account["id"] = json.Number("77")
			hijacker, ok := writer.(http.Hijacker)
			if !ok {
				t.Fatal("test server does not support hijacking")
			}
			connection, _, err := hijacker.Hijack()
			if err != nil {
				t.Fatal(err)
			}
			_ = connection.Close()
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/admin/accounts/77/schedulable":
			account["schedulable"] = false
			_, _ = writer.Write([]byte(`{"success":true}`))
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts/77":
			_ = json.NewEncoder(writer).Encode(map[string]any{"data": account})
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer admin.Close()
	repository, private, databasePath := onboardingFixture(t, admin.URL)
	keys := &checkingKeys{databasePath: databasePath}
	service := New(repository, private, keys, nil)
	request := Request{
		Host: "upstream.test", UpstreamType: "sub2api", LocalGroupID: "3",
		UpstreamGroupID: "6", Schedulable: false, Actor: "operator",
	}
	if _, err := service.Onboard(context.Background(), request); err == nil {
		t.Fatal("unknown account commit must remain pending")
	}
	failCatalog = true
	if _, err := service.Onboard(context.Background(), request); err == nil {
		t.Fatal("failed account reconciliation must remain fail-closed")
	}
	if posts != 1 {
		t.Fatalf("account create posts=%d", posts)
	}
	database, err := sql.Open("sqlite", "file:"+databasePath)
	if err != nil {
		t.Fatal(err)
	}
	var accountUnknown bool
	if err := database.QueryRow(`SELECT account_commit_unknown FROM onboarding_pending`).Scan(&accountUnknown); err != nil {
		database.Close()
		t.Fatal(err)
	}
	_ = database.Close()
	if !accountUnknown {
		t.Fatal("account commit-unknown state was not persisted")
	}

	failCatalog = false
	visible = true
	result, err := service.Onboard(context.Background(), request)
	if err != nil || result["account_id"] != "77" || posts != 1 {
		t.Fatalf("result=%#v err=%v posts=%d", result, err, posts)
	}
}

func TestOnboardRequiresCompleteAccountReadbackAndReusesStoredAccountID(t *testing.T) {
	var createdBody map[string]any
	var accountPosts, schedulePosts int
	admin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts":
			_, _ = writer.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/admin/accounts":
			accountPosts++
			decoder := json.NewDecoder(request.Body)
			decoder.UseNumber()
			if err := decoder.Decode(&createdBody); err != nil {
				t.Fatal(err)
			}
			_, _ = writer.Write([]byte(`{"data":{"id":77}}`))
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts/77":
			readback := make(map[string]any, len(createdBody)+1)
			for key, value := range createdBody {
				readback[key] = value
			}
			readback["id"] = json.Number("77")
			readback["concurrency"] = json.Number("999")
			_ = json.NewEncoder(writer).Encode(map[string]any{"data": readback})
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/admin/accounts/77/schedulable":
			schedulePosts++
			_, _ = writer.Write([]byte(`{"success":true}`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer admin.Close()
	repository, private, databasePath := onboardingFixture(t, admin.URL)
	keys := &checkingKeys{databasePath: databasePath}
	service := New(repository, private, keys, nil)
	request := Request{
		Host: "upstream.test", UpstreamType: "sub2api", LocalGroupID: "3",
		UpstreamGroupID: "6", Schedulable: false, Actor: "operator",
	}
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := service.Onboard(context.Background(), request); err == nil || !strings.Contains(err.Error(), "并发不一致") {
			t.Fatalf("attempt=%d err=%v", attempt+1, err)
		}
	}
	if accountPosts != 1 || schedulePosts != 0 || keys.createCalls != 1 || keys.revealCalls != 1 {
		t.Fatalf("accountPosts=%d schedulePosts=%d create=%d reveal=%d", accountPosts, schedulePosts, keys.createCalls, keys.revealCalls)
	}
}

func TestOnboardRejectsFrozenFieldMismatchAfterSchedulingReadback(t *testing.T) {
	var createdBody map[string]any
	var accountPosts, schedulePosts, detailReads int
	admin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts":
			_, _ = writer.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/admin/accounts":
			accountPosts++
			decoder := json.NewDecoder(request.Body)
			decoder.UseNumber()
			if err := decoder.Decode(&createdBody); err != nil {
				t.Fatal(err)
			}
			_, _ = writer.Write([]byte(`{"data":{"id":77}}`))
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/admin/accounts/77/schedulable":
			schedulePosts++
			createdBody["schedulable"] = false
			_, _ = writer.Write([]byte(`{"success":true}`))
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts/77":
			detailReads++
			readback := make(map[string]any, len(createdBody)+1)
			for key, value := range createdBody {
				readback[key] = value
			}
			readback["id"] = json.Number("77")
			if detailReads == 2 {
				readback["priority"] = json.Number("999")
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"data": readback})
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer admin.Close()
	repository, private, databasePath := onboardingFixture(t, admin.URL)
	keys := &checkingKeys{databasePath: databasePath}
	result, err := New(repository, private, keys, nil).Onboard(context.Background(), Request{
		Host: "upstream.test", UpstreamType: "sub2api", LocalGroupID: "3",
		UpstreamGroupID: "6", Schedulable: false, Actor: "operator",
	})
	if err == nil || !strings.Contains(err.Error(), "优先级不一致") {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if result["remote_write"] != true || accountPosts != 1 || schedulePosts != 1 || detailReads != 2 {
		t.Fatalf("result=%#v accountPosts=%d schedulePosts=%d detailReads=%d", result, accountPosts, schedulePosts, detailReads)
	}
	database, err := sql.Open("sqlite", "file:"+databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var accountID string
	if err := database.QueryRow(`SELECT upstream_account_id FROM onboarding_pending`).Scan(&accountID); err != nil {
		t.Fatal(err)
	}
	if accountID != "77" {
		t.Fatalf("pending account ID=%q", accountID)
	}
	var localAccounts int
	if err := database.QueryRow(`SELECT COUNT(*) FROM accounts WHERE id='77'`).Scan(&localAccounts); err != nil {
		t.Fatal(err)
	}
	if localAccounts != 0 {
		t.Fatalf("local accounts=%d", localAccounts)
	}
}

func TestEnqueueBatchRejectsDuplicateUpstreamGroupsBeforeStarting(t *testing.T) {
	admin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer admin.Close()
	repository, private, databasePath := onboardingFixture(t, admin.URL)
	service := New(repository, private, &checkingKeys{databasePath: databasePath}, nil)
	request := Request{
		Host: "upstream.test", UpstreamType: "sub2api", LocalGroupID: "3",
		UpstreamGroupID: "6", Schedulable: false, Actor: "operator",
	}
	if _, err := service.EnqueueBatch(context.Background(), []Request{request, request}); err == nil || !strings.Contains(err.Error(), "不能在一个批次中重复提交") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnqueueBatchRequiresABoundedSelection(t *testing.T) {
	service := &Service{}
	if _, err := service.EnqueueBatch(context.Background(), nil); err == nil {
		t.Fatal("empty batch should fail")
	}
	if _, err := service.EnqueueBatch(context.Background(), make([]Request, 51)); err == nil {
		t.Fatal("oversized batch should fail")
	}
}

func TestBatchItemResultPreservesRemoteWriteAndPendingDetails(t *testing.T) {
	pending := map[string]any{"operation_id": "pending-1", "key_commit_unknown": true}
	row := map[string]any{"status": "失败"}
	mergeBatchItemResult(row, map[string]any{"remote_write": true, "pending": pending})
	actualPending, ok := row["pending"].(map[string]any)
	if row["remote_write"] != true || !ok || actualPending["operation_id"] != "pending-1" || actualPending["key_commit_unknown"] != true {
		t.Fatalf("batch item result=%#v", row)
	}
}

func TestValidateAllowsOnboardingInEveryRuntimeMode(t *testing.T) {
	repository, private, databasePath := onboardingFixture(t, "https://admin.example")
	service := New(repository, private, &checkingKeys{databasePath: databasePath}, nil)
	request := Request{
		Host: "upstream.test", UpstreamType: "sub2api", LocalGroupID: "3",
		UpstreamGroupID: "6", Schedulable: false, Actor: "operator",
	}
	for _, mode := range []string{runtimepolicy.Monitoring, runtimepolicy.Full} {
		t.Run(mode, func(t *testing.T) {
			if _, err := repository.SetMode(context.Background(), mode); err != nil {
				t.Fatal(err)
			}
			if _, err := service.validate(context.Background(), request); err != nil {
				t.Fatalf("账号添加不应受运行模式 %q 限制：%v", mode, err)
			}
		})
	}
}

func TestAccountCreationParametersUseSharedDefaultsAndAllowPerAccountOverrides(t *testing.T) {
	defaults := configstore.AccountDefaultsSettings{Concurrency: 24, Priority: 7}
	priority, concurrency := accountCreationParameters(defaults, Request{UpstreamType: "grok"})
	if priority != 7 || concurrency != 24 {
		t.Fatalf("shared defaults priority=%d concurrency=%d", priority, concurrency)
	}
	customPriority, customConcurrency := int64(3), int64(40)
	priority, concurrency = accountCreationParameters(defaults, Request{Priority: &customPriority, Concurrency: &customConcurrency})
	if priority != 3 || concurrency != 40 {
		t.Fatalf("overrides priority=%d concurrency=%d", priority, concurrency)
	}
}

func TestStableIDsUseNumericOrderAndRejectUnsupportedRange(t *testing.T) {
	ids, err := stableIDs([]any{json.Number("10"), json.Number("2")})
	if err != nil || strings.Join(ids, ",") != "2,10" {
		t.Fatalf("ids=%#v err=%v", ids, err)
	}
	if !stableID("9223372036854775807") || stableID("9223372036854775808") {
		t.Fatal("stable IDs must be positive signed int64 values")
	}
	for _, invalid := range []string{"0", "-1", "01"} {
		if stableID(invalid) {
			t.Fatalf("stableID(%q)=true", invalid)
		}
	}
	if _, err := stableIDs([]any{json.Number("9223372036854775808")}); err == nil || !strings.Contains(err.Error(), "无效稳定 ID") {
		t.Fatalf("error=%v", err)
	}
}

func TestKeyCleanupOnlyDeletesKeysWithoutBindingsOrPendingOnboarding(t *testing.T) {
	repository, private, databasePath := onboardingFixture(t, "https://admin.example")
	database, err := sql.Open("sqlite", "file:"+databasePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,metadata_json,updated_at)
			VALUES('41','upstream.test','bound-key','bound','codex','{}','now')`,
		`INSERT INTO onboarding_pending(operation_id,upstream_host,upstream_type,upstream_key_id,upstream_key_name,
			upstream_group_id,upstream_group_name,local_group_id,local_group_name,multiplier,intent_hash,reason,created_at,updated_at)
			VALUES('pending-1','upstream.test','sub2api','pending-key','pending','6','pro','3','codex','0.2','test-intent','retry','now','now')`,
	} {
		if _, err := database.Exec(statement); err != nil {
			database.Close()
			t.Fatal(err)
		}
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	groupID := "6"
	status := "active"
	keys := &probeKeys{catalog: []business.UpstreamCatalogKey{
		{KeyID: "bound-key", Name: "bound", UpstreamGroup: &groupID, Status: &status},
		{KeyID: "pending-key", Name: "pending", UpstreamGroup: &groupID, Status: &status},
		{KeyID: "unused-key", Name: "unused", UpstreamGroup: &groupID, Status: &status},
	}}
	tasks := &cleanupTaskStore{final: make(chan taskstore.Task, 1)}
	service := New(repository, private, keys, tasks)
	preview, err := service.PreviewUnboundKeys(context.Background(), "upstream.test")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Keys) != 1 || preview.Keys[0].KeyID != "unused-key" {
		t.Fatalf("preview=%#v", preview)
	}
	if _, err := service.EnqueueKeyCleanup(context.Background(), "upstream.test", []string{"bound-key"}, "tester"); err == nil {
		t.Fatal("bound key cleanup should be rejected")
	}
	queued, err := service.EnqueueKeyCleanup(context.Background(), "upstream.test", []string{"unused-key"}, "tester")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case completed := <-tasks.final:
		if completed.ID != queued.ID || completed.Status != "succeeded" || completed.Result["deleted"] != 1 {
			t.Fatalf("completed=%#v", completed)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup task did not complete")
	}
	if strings.Join(keys.deleteIDs, ",") != "unused-key" {
		t.Fatalf("deleted=%#v", keys.deleteIDs)
	}
	database, err = sql.Open("sqlite", "file:"+databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var eventStatus, payloadRaw string
	if err := database.QueryRow(`SELECT status,payload_json FROM runtime_events
		WHERE event_type='upstream.key.cleanup' ORDER BY source_id LIMIT 1`).Scan(&eventStatus, &payloadRaw); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(payloadRaw), &payload); err != nil {
		t.Fatal(err)
	}
	if eventStatus != "succeeded" || payload["key_id"] != "unused-key" || payload["remote_write"] != true ||
		payload["remote_confirmed"] != true || payload["readback_confirmed"] != true {
		t.Fatalf("event status=%q payload=%#v", eventStatus, payload)
	}
}

func TestKeyCleanupReportsAuditFailureAfterConfirmedDelete(t *testing.T) {
	repository, private, _ := onboardingFixture(t, "https://admin.example")
	groupID, status := "6", "active"
	keys := &probeKeys{catalog: []business.UpstreamCatalogKey{{
		KeyID: "unused-key", Name: "unused", UpstreamGroup: &groupID, Status: &status,
	}}}
	tasks := &cleanupTaskStore{final: make(chan taskstore.Task, 1)}
	service := New(auditFailingOnboardingRepository{Repository: repository}, private, keys, tasks)
	queued, err := service.EnqueueKeyCleanup(context.Background(), "upstream.test", []string{"unused-key"}, "tester")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case completed := <-tasks.final:
		if completed.ID != queued.ID || completed.Status != "failed" || completed.Result["deleted"] != 1 || completed.Result["audit_failed"] != 1 {
			t.Fatalf("completed=%#v", completed)
		}
		items, ok := completed.Result["items"].([]map[string]any)
		if !ok || len(items) != 1 || items[0]["status"] != "deleted" || !strings.Contains(fmt.Sprint(items[0]["audit_error"]), "audit unavailable") {
			t.Fatalf("items=%#v", completed.Result["items"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup task did not complete")
	}
	if strings.Join(keys.deleteIDs, ",") != "unused-key" {
		t.Fatalf("remote delete fact was lost: %#v", keys.deleteIDs)
	}
}

func TestKeyCleanupRequiresAbsentReadbackAfterDelete(t *testing.T) {
	repository, private, _ := onboardingFixture(t, "https://admin.example")
	groupID, status := "6", "active"
	keys := &probeKeys{
		catalog:         []business.UpstreamCatalogKey{{KeyID: "unused-key", Name: "unused", UpstreamGroup: &groupID, Status: &status}},
		keepAfterDelete: true,
	}
	tasks := &cleanupTaskStore{final: make(chan taskstore.Task, 1)}
	service := New(repository, private, keys, tasks)
	if _, err := service.EnqueueKeyCleanup(context.Background(), "upstream.test", []string{"unused-key"}, "tester"); err != nil {
		t.Fatal(err)
	}
	select {
	case completed := <-tasks.final:
		if completed.Status != "failed" || completed.Result["deleted"] != 0 || completed.Result["failed"] != 1 {
			t.Fatalf("completed=%#v", completed)
		}
		items := completed.Result["items"].([]map[string]any)
		if !strings.Contains(fmt.Sprint(items[0]["reason"]), "仍可读") {
			t.Fatalf("items=%#v", items)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup task did not complete")
	}
}

func TestKeyCleanupUsesReadbackWhenDeleteReturnsError(t *testing.T) {
	repository, private, _ := onboardingFixture(t, "https://admin.example")
	groupID, status := "6", "active"
	keys := &probeKeys{
		catalog:   []business.UpstreamCatalogKey{{KeyID: "unused-key", Name: "unused", UpstreamGroup: &groupID, Status: &status}},
		deleteErr: errors.New("connection reset after write"), commitOnDeleteError: true,
	}
	tasks := &cleanupTaskStore{final: make(chan taskstore.Task, 1)}
	service := New(repository, private, keys, tasks)
	if _, err := service.EnqueueKeyCleanup(context.Background(), "upstream.test", []string{"unused-key"}, "tester"); err != nil {
		t.Fatal(err)
	}
	select {
	case completed := <-tasks.final:
		if completed.Status != "succeeded" || completed.Result["deleted"] != 1 || completed.Result["failed"] != 0 {
			t.Fatalf("completed=%#v", completed)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup task did not complete")
	}
}

func TestKeyCleanupReportsUnknownPostDeleteReadback(t *testing.T) {
	repository, private, _ := onboardingFixture(t, "https://admin.example")
	groupID, status := "6", "active"
	keys := &probeKeys{
		catalog:    []business.UpstreamCatalogKey{{KeyID: "unused-key", Name: "unused", UpstreamGroup: &groupID, Status: &status}},
		listErrors: []error{nil, nil, errors.New("readback unavailable")},
	}
	tasks := &cleanupTaskStore{final: make(chan taskstore.Task, 1)}
	service := New(repository, private, keys, tasks)
	if _, err := service.EnqueueKeyCleanup(context.Background(), "upstream.test", []string{"unused-key"}, "tester"); err != nil {
		t.Fatal(err)
	}
	select {
	case completed := <-tasks.final:
		if completed.Status != "failed" || completed.Result["deleted"] != 0 || completed.Result["failed"] != 1 {
			t.Fatalf("completed=%#v", completed)
		}
		items := completed.Result["items"].([]map[string]any)
		if !strings.Contains(fmt.Sprint(items[0]["reason"]), "结果未知") {
			t.Fatalf("items=%#v", items)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup task did not complete")
	}
}

func TestKeyCleanupWaitsForCanonicalUpstreamLease(t *testing.T) {
	repository, private, _ := onboardingFixture(t, "https://admin.example")
	groupID, status := "6", "active"
	keys := &probeKeys{catalog: []business.UpstreamCatalogKey{{
		KeyID: "unused-key", Name: "unused", UpstreamGroup: &groupID, Status: &status,
	}}}
	tasks := &cleanupTaskStore{final: make(chan taskstore.Task, 1)}
	service := New(repository, private, keys, tasks)
	if _, err := repository.Upstreams(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, release, err := mutationguard.Acquire(context.Background(), repository, mutationguard.Upstream("HTTPS://UPSTREAM.TEST/"))
	if err != nil {
		t.Fatal(err)
	}
	queued, err := service.EnqueueKeyCleanup(context.Background(), "upstream.test", []string{"unused-key"}, "tester")
	if err != nil {
		_ = release()
		t.Fatal(err)
	}
	select {
	case completed := <-tasks.final:
		_ = release()
		t.Fatalf("cleanup bypassed canonical upstream lease: %#v", completed)
	case <-time.After(50 * time.Millisecond):
	}
	if len(keys.deleteIDs) != 0 {
		_ = release()
		t.Fatalf("deleted while upstream lease was held: %#v", keys.deleteIDs)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	select {
	case completed := <-tasks.final:
		if completed.ID != queued.ID || completed.Status != "succeeded" || completed.Result["deleted"] != 1 {
			t.Fatalf("completed=%#v", completed)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup did not resume after canonical upstream lease release")
	}
}

func TestKeyCleanupDistinguishesCancellationFromLeaseFailure(t *testing.T) {
	for _, testCase := range []struct {
		name              string
		cause             error
		expectedStatus    string
		expectedErrorText string
	}{
		{name: "ordinary cancellation", cause: context.Canceled, expectedStatus: "cancelled"},
		{name: "lease failure", cause: errors.New("变更租约已失效"), expectedStatus: "failed", expectedErrorText: "变更租约已失效"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			repository, private, _ := onboardingFixture(t, "https://admin.example")
			groupID, status := "6", "active"
			keys := &blockingProbeKeys{
				probeKeys: &probeKeys{catalog: []business.UpstreamCatalogKey{{
					KeyID: "unused-key", Name: "unused", UpstreamGroup: &groupID, Status: &status,
				}}},
				deleteStarted: make(chan struct{}), allowDelete: make(chan struct{}),
			}
			tasks := &cleanupTaskStore{final: make(chan taskstore.Task, 1)}
			service := New(repository, private, keys, tasks)
			task, err := service.newQueuedTask("upstream-key-cleanup", "1 个无绑定上游 Key 清理已排队")
			if err != nil {
				t.Fatal(err)
			}
			parent, cancel := context.WithCancelCause(context.Background())
			done := make(chan struct{})
			go func() {
				defer close(done)
				service.executeKeyCleanup(parent, task, "upstream.test", []string{"unused-key"}, "tester")
			}()
			select {
			case <-keys.deleteStarted:
			case <-time.After(2 * time.Second):
				t.Fatal("cleanup did not reach the remote delete")
			}
			cancel(testCase.cause)
			select {
			case completed := <-tasks.final:
				if completed.Status != testCase.expectedStatus {
					t.Fatalf("completed=%#v", completed)
				}
				if testCase.expectedStatus == "cancelled" {
					if completed.Result["cancelled"] != true || completed.Result["error"] != nil {
						t.Fatalf("cancelled task=%#v", completed)
					}
				} else if errorText, _ := completed.Result["error"].(string); !strings.Contains(errorText, testCase.expectedErrorText) || completed.Result["cancelled"] == true {
					t.Fatalf("failed task=%#v", completed)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("cleanup did not persist a terminal task")
			}
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("cleanup did not release its mutation lease")
			}
		})
	}
}

func onboardingFixture(t *testing.T, adminURL string) (*business.Store, *configstore.Store, string) {
	t.Helper()
	directory := t.TempDir()
	databasePath := filepath.Join(directory, "business.sqlite3")
	repository, err := business.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	if err := repository.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("sqlite", "file:"+databasePath)
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`INSERT INTO upstreams(host,base_url,upstream_type,auth_status,metadata_json,updated_at) VALUES('upstream.test','https://upstream.test','sub2api','已鉴权','{}','now')`,
		`INSERT INTO recharge_rates(host,recharge_rate,updated_at) VALUES('upstream.test','1','now')`,
		`INSERT INTO upstream_groups(host,group_id,name,description,platform,status,raw_rate,effective_rate,updated_at) VALUES('upstream.test','6','pro','stable','openai','active','0.2','0.2','now')`,
		`INSERT INTO local_groups(name,remote_id,strategy,strategy_source,updated_at) VALUES('codex','3','balanced','global_default','now')`,
		`INSERT INTO local_groups(name,remote_id,strategy,strategy_source,updated_at) VALUES('pro','4','balanced','global_default','now')`,
	}
	for _, statement := range statements {
		if _, err := database.Exec(statement); err != nil {
			database.Close()
			t.Fatal(err)
		}
	}
	_ = database.Close()
	private, err := configstore.Open(filepath.Join(directory, "config.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = private.Close() })
	if err := private.ConfigureTarget(context.Background(), adminURL, "admin-key", 5); err != nil {
		t.Fatal(err)
	}
	token := "upstream-token"
	if err := private.SaveAuthRecord(context.Background(), configstore.AuthRecord{
		Host: "upstream.test", BaseURL: "https://upstream.test", UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", AccessToken: &token, Headers: map[string]string{}, Cookies: map[string]string{},
	}, nil); err != nil {
		t.Fatal(err)
	}
	return repository, private, databasePath
}

func assertSecretAbsent(t *testing.T, databasePath, secret string) {
	t.Helper()
	for _, path := range []string{databasePath, databasePath + "-wal"} {
		body, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if strings.Contains(string(body), secret) {
			t.Fatalf("secret was persisted in %s", path)
		}
	}
}
