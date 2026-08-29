package onboarding

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

type checkingKeys struct {
	databasePath string
	createCalls  int
	revealCalls  int
	verification []bool
}

type probeKeys struct {
	creates int
	deletes int
}

func (keys *probeKeys) CreateKey(context.Context, configstore.AuthRecord, string, string) (upstreamsync.CreatedKey, error) {
	keys.creates++
	return upstreamsync.CreatedKey{KeyID: "91", Name: "console-probe", GroupID: "6", Secret: "probe-secret"}, nil
}

func (keys *probeKeys) CreateKeyWithVerification(ctx context.Context, record configstore.AuthRecord, name, groupID string, _ bool) (upstreamsync.CreatedKey, error) {
	return keys.CreateKey(ctx, record, name, groupID)
}

func (keys *probeKeys) RevealKey(context.Context, configstore.AuthRecord, string, string) (upstreamsync.CreatedKey, error) {
	return upstreamsync.CreatedKey{}, errors.New("unexpected reveal")
}

func (keys *probeKeys) DeleteKey(context.Context, configstore.AuthRecord, string) error {
	keys.deletes++
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

func TestOnboardKeepsNetworkOutsideTransactionsAndNeverPersistsSecret(t *testing.T) {
	reads := 0
	admin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Header.Get("X-API-Key") != "admin-key" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/admin/accounts":
			body, _ := io.ReadAll(request.Body)
			if !strings.Contains(string(body), "never-store-this") {
				t.Errorf("remote account did not receive the one-time secret: %s", body)
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
	keys := &checkingKeys{databasePath: databasePath}
	service := New(repository, private, keys, nil)
	result, err := service.Onboard(context.Background(), Request{
		Host: "upstream.test", UpstreamType: "sub2api", Multiplier: "0.2", LocalGroupID: "3",
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
	if err := database.QueryRow(`SELECT a.name,b.upstream_key_id FROM accounts a JOIN bindings b ON b.local_account_id=a.id WHERE a.id='77'`).Scan(&accountName, &keyID); err != nil {
		t.Fatal(err)
	}
	if accountName != "upstream-0.2" || keyID != "91" {
		t.Fatalf("account=%q key=%q", accountName, keyID)
	}
	assertSecretAbsent(t, databasePath, "never-store-this")
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
		Host: "upstream.test", UpstreamType: "sub2api", Multiplier: "0.2", LocalGroupID: "3",
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
		Host: "upstream.test", UpstreamType: "sub2api", Multiplier: "0.2", LocalGroupID: "3",
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
		Host: "upstream.test", UpstreamType: "sub2api", Multiplier: "0.2", LocalGroupID: "3",
		UpstreamGroupID: "6", Schedulable: false, Actor: "operator",
	}
	if _, err := service.EnqueueBatch(context.Background(), []Request{request, request}); err == nil || !strings.Contains(err.Error(), "不能在一个批次中重复添加") {
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
		Host: "upstream.test", UpstreamType: "sub2api", Multiplier: "0.2", LocalGroupID: "3",
		UpstreamGroupID: "6", Schedulable: false, Actor: "operator",
	}
	for _, mode := range []string{runtimepolicy.Monitoring, runtimepolicy.Scheduling, runtimepolicy.Full} {
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
