package onboarding

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
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

type probeKeys struct {
	creates   int
	deletes   int
	reveals   int
	revealErr error
	catalog   []business.UpstreamCatalogKey
	deleteIDs []string
}

type cleanupTaskStore struct {
	final chan taskstore.Task
}

func (store *cleanupTaskStore) Save(_ context.Context, task taskstore.Task) error {
	if task.Status == "succeeded" || task.Status == "failed" {
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
	return nil
}

func (keys *probeKeys) ListKeys(context.Context, configstore.AuthRecord) ([]business.UpstreamCatalogKey, error) {
	return append([]business.UpstreamCatalogKey{}, keys.catalog...), nil
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

func TestOnboardKeepsNetworkOutsideTransactionsAndPersistsSecretOnlyInPrivateStore(t *testing.T) {
	reads := 0
	const upstreamBaseURL = "http://192.0.2.10:8443/accelerated"
	admin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Header.Get("X-API-Key") != "admin-key" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
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
			if credentials["base_url"] != upstreamBaseURL {
				t.Errorf("remote account base_url = %#v, want %q", credentials["base_url"], upstreamBaseURL)
			}
			if body["concurrency"] != json.Number("10") || body["priority"] != json.Number("1") {
				t.Errorf("remote account defaults = concurrency %#v priority %#v", body["concurrency"], body["priority"])
			}
			if _, present := body["load_factor"]; present {
				t.Errorf("remote account must leave load_factor unset: %#v", body["load_factor"])
			}
			_, _ = writer.Write([]byte(`{"data":{"id":77}}`))
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/admin/accounts/77/schedulable":
			_, _ = writer.Write([]byte(`{"data":{"id":77,"name":"upstream-0.2","group_ids":[3],"schedulable":false,"rate_multiplier":0.2}}`))
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts/77":
			reads++
			_, _ = writer.Write([]byte(`{"data":{"id":77,"name":"upstream-0.2","group_ids":[3],"schedulable":false,"rate_multiplier":0.2}}`))
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
		UpstreamGroupID: "6", Schedulable: false, Actor: "operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result["account_id"] != "77" || result["readback_confirmed"] != false || keys.createCalls != 1 || keys.revealCalls != 0 || reads != 0 || len(keys.verification) != 0 {
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

func TestOnboardCreatesOneAccountInEverySelectedLocalGroup(t *testing.T) {
	admin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
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
			_, _ = writer.Write([]byte(`{"data":{"id":77}}`))
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/admin/accounts/77/schedulable":
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
	admin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts/77":
			reads++
			groups := `[3]`
			if reads > 1 {
				groups = `[4]`
			}
			_, _ = writer.Write([]byte(`{"data":{"id":77,"name":"existing","group_ids":` + groups + `}}`))
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
	result, err := New(repository, private, keys, nil).Onboard(context.Background(), Request{
		Host: "upstream.test", UpstreamType: "sub2api", LocalGroupIDs: []string{"4"},
		UpstreamGroupID: "6", AccountIDs: []string{"77"}, Actor: "operator",
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
	var groupID, bindingGroup string
	if err := database.QueryRow(`SELECT group_id FROM account_groups WHERE account_id='77'`).Scan(&groupID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`SELECT local_group FROM bindings WHERE local_account_id='77'`).Scan(&bindingGroup); err != nil {
		t.Fatal(err)
	}
	if groupID != "4" || bindingGroup != "pro" {
		t.Fatalf("groupID=%q bindingGroup=%q", groupID, bindingGroup)
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

func TestOnboardIgnoresAutomaticSchedulingWritebackVerification(t *testing.T) {
	reads := 0
	admin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/admin/accounts":
			_, _ = writer.Write([]byte(`{"data":{"id":77}}`))
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/admin/accounts/77/schedulable":
			_, _ = writer.Write([]byte(`{"success":true}`))
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts/77":
			reads++
			_, _ = writer.Write([]byte(`{"data":{"id":77,"name":"upstream-0.2","group_ids":[3],"schedulable":false,"rate_multiplier":0.2}}`))
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
	if result["readback_confirmed"] != false || reads != 0 || len(keys.verification) != 0 {
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
			upstream_group_id,upstream_group_name,local_group_id,local_group_name,multiplier,reason,created_at,updated_at)
			VALUES('pending-1','upstream.test','sub2api','pending-key','pending','6','pro','3','codex','0.2','retry','now','now')`,
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
