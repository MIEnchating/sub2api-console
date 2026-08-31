package modelcheck

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

type recordingTasks struct {
	mu       sync.Mutex
	tasks    []taskstore.Task
	terminal chan taskstore.Task
}

func (store *recordingTasks) Save(_ context.Context, task taskstore.Task) error {
	store.mu.Lock()
	store.tasks = append(store.tasks, task)
	store.mu.Unlock()
	if task.Status == "succeeded" || task.Status == "failed" {
		select {
		case store.terminal <- task:
		default:
		}
	}
	return nil
}

func (store *recordingTasks) ListBySkill(_ context.Context, skill string) ([]taskstore.Task, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	result := make([]taskstore.Task, 0, len(store.tasks))
	for _, task := range store.tasks {
		if task.Skill == skill {
			result = append(result, task)
		}
	}
	return result, nil
}

func TestAccountStatusesUseLatestModelCheckResultPerAccount(t *testing.T) {
	store := &recordingTasks{tasks: []taskstore.Task{
		{
			ID: "completed", Skill: "sub2api-model-check", Status: "succeeded",
			UpdatedAt: "2026-08-31T02:00:00Z",
			Result: map[string]any{"tests": []map[string]any{
				{"account_id": "41", "verdict": "SOL_CONSISTENT", "requests": map[string]any{"successful": 2, "total": 2}},
				{"account_id": "42", "verdict": "SOL_CONSISTENT", "requests": map[string]any{"successful": 2, "total": 2}},
				{"account_id": "42", "verdict": "LUNA_LIKE", "requests": map[string]any{"successful": 2, "total": 2}},
				{"account_id": "43", "verdict": "INCONCLUSIVE", "requests": map[string]any{"successful": 0, "total": 4}},
			}},
		},
		{
			ID: "running", Skill: "sub2api-model-check", Status: "running",
			UpdatedAt: "2026-08-31T03:00:00Z", Result: map[string]any{"account_ids": []string{"41", "44"}},
		},
		{
			ID: "older-failure", Skill: "sub2api-model-check", Status: "failed",
			UpdatedAt: "2026-08-30T03:00:00Z", Result: map[string]any{"account_ids": []string{"45"}},
		},
	}}
	statuses, err := (&Service{tasks: store}).AccountStatuses(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]AccountCheckStatus{}
	for _, status := range statuses {
		byID[status.AccountID] = status
	}
	for accountID, expected := range map[string]string{
		"41": "consistent", "42": "inconsistent", "43": "inconclusive", "45": "inconclusive",
	} {
		if byID[accountID].Status != expected {
			t.Fatalf("account %s status=%q want=%q; all=%#v", accountID, byID[accountID].Status, expected, statuses)
		}
	}
	if _, found := byID["44"]; found {
		t.Fatalf("running task without a verdict must not be exposed as a detection result: %#v", byID["44"])
	}
	if byID["41"].TaskID != "completed" || byID["41"].CheckedAt != "2026-08-31T02:00:00Z" {
		t.Fatalf("running task must not replace the latest completed verdict: %#v", byID["41"])
	}
}

type fakeCredentials struct {
	record       configstore.AuthRecord
	expectedHost string
	cached       *configstore.UpstreamKeySecret
	authCalls    int
	cacheReads   int
	cacheWrites  int
}

func (credentials *fakeCredentials) AuthRecord(_ context.Context, host string) (*configstore.AuthRecord, error) {
	credentials.authCalls++
	if credentials.expectedHost != "" && host != credentials.expectedHost {
		return nil, fmt.Errorf("auth host=%q want=%q", host, credentials.expectedHost)
	}
	value := credentials.record
	return &value, nil
}

func (credentials *fakeCredentials) UpstreamKeySecret(_ context.Context, host, keyID, groupID string) (*configstore.UpstreamKeySecret, error) {
	credentials.cacheReads++
	if credentials.cached == nil || credentials.cached.Host != host || credentials.cached.KeyID != keyID || credentials.cached.GroupID != groupID {
		return nil, nil
	}
	value := *credentials.cached
	return &value, nil
}

func (credentials *fakeCredentials) SaveUpstreamKeySecret(_ context.Context, value configstore.UpstreamKeySecret) error {
	credentials.cacheWrites++
	credentials.cached = &value
	return nil
}

type fakeAccounts struct {
	rows    []business.AccountStatus
	details map[string]*business.AccountDetail
}

func (accounts fakeAccounts) Accounts(context.Context) ([]business.AccountStatus, error) {
	return accounts.rows, nil
}

func (accounts fakeAccounts) Account(_ context.Context, accountID string) (*business.AccountDetail, error) {
	return accounts.details[accountID], nil
}

type fakeKeyRevealer struct {
	secret          string
	expectedKeyID   string
	expectedGroupID string
	calls           int
}

type missingCredentials struct{}

func (missingCredentials) AuthRecord(context.Context, string) (*configstore.AuthRecord, error) {
	return nil, nil
}

func (missingCredentials) UpstreamKeySecret(context.Context, string, string, string) (*configstore.UpstreamKeySecret, error) {
	return nil, nil
}

func (missingCredentials) SaveUpstreamKeySecret(context.Context, configstore.UpstreamKeySecret) error {
	return nil
}

type fakeAuthResolver struct {
	record configstore.AuthRecord
	host   string
	actor  string
}

func (resolver *fakeAuthResolver) ResolveAuth(_ context.Context, host, actor string) (*configstore.AuthRecord, error) {
	resolver.host, resolver.actor = host, actor
	value := resolver.record
	return &value, nil
}

func (revealer *fakeKeyRevealer) RevealKey(_ context.Context, _ configstore.AuthRecord, keyID, groupID string) (upstreamsync.CreatedKey, error) {
	revealer.calls++
	if revealer.expectedKeyID != "" && keyID != revealer.expectedKeyID {
		return upstreamsync.CreatedKey{}, fmt.Errorf("key ID=%q want=%q", keyID, revealer.expectedKeyID)
	}
	if revealer.expectedGroupID != "" && groupID != revealer.expectedGroupID {
		return upstreamsync.CreatedKey{}, fmt.Errorf("group ID=%q want=%q", groupID, revealer.expectedGroupID)
	}
	return upstreamsync.CreatedKey{KeyID: keyID, GroupID: groupID, Secret: revealer.secret}, nil
}

func TestAccountMatrixRevealsBoundKeyAndCallsAccountBaseURLDirectly(t *testing.T) {
	const accountKey = "direct-account-key"
	requestCount := 0
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()
		if request.URL.Path != "/v1/messages" || request.Header.Get("x-api-key") != accountKey {
			t.Errorf("unexpected direct account request: %s headers=%v", request.URL.Path, request.Header)
		}
		var payload struct {
			Model    string `json:"model"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if payload.Model != "claude-opus-5" || len(payload.Messages) != 1 || !strings.Contains(payload.Messages[0].Content, "Return exactly one JSON array") {
			t.Errorf("unexpected payload: %#v", payload)
		}
		answers := make([]string, 12)
		for index := range answers {
			answers[index] = "A"
		}
		encoded, _ := json.Marshal(answers)
		response.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(response, `{"model":"claude-opus-5","content":[{"type":"text","text":%q}]}`, string(encoded))
	}))
	defer server.Close()

	platform, groupID := "anthropic", "7"
	baseURL := server.URL + "/v1"
	detail := &business.AccountDetail{
		AccountStatus: business.AccountStatus{ID: "41", Name: "主账号", Platform: &platform, BaseURL: &baseURL},
		Bindings:      []business.AccountBinding{{LocalAccountID: "41", UpstreamHost: "api.example", UpstreamKeyID: "91", UpstreamGroupID: &groupID}},
	}
	tasks := &recordingTasks{terminal: make(chan taskstore.Task, 1)}
	credentials := &fakeCredentials{
		record:       configstore.AuthRecord{Host: "api.example", BaseURL: server.URL, UpstreamType: "newapi"},
		expectedHost: "api.example",
	}
	revealer := &fakeKeyRevealer{secret: accountKey, expectedKeyID: "91", expectedGroupID: "7"}
	service, err := New(
		tasks,
		credentials,
		fakeAccounts{rows: []business.AccountStatus{{ID: "41", Name: "主账号"}}, details: map[string]*business.AccountDetail{"41": detail}},
		revealer,
	)
	if err != nil {
		t.Fatal(err)
	}
	queued, err := service.Enqueue(context.Background(), Request{
		AccountIDs: []string{"41"}, Models: []string{"claude-opus-5"}, Rounds: 1, TimeoutSeconds: 5,
	})
	if err != nil || queued.Status != "queued" {
		t.Fatalf("queued=%#v err=%v", queued, err)
	}
	select {
	case terminal := <-tasks.terminal:
		if terminal.Status != "succeeded" || terminal.Result["combinations"] != 1 {
			t.Fatalf("unexpected terminal task: %#v", terminal)
		}
		tests, ok := terminal.Result["tests"].([]map[string]any)
		if !ok || len(tests) != 1 || tests[0]["account_id"] != "41" || tests[0]["claimed_model"] != "claude-opus-5" {
			t.Fatalf("unexpected matrix results: %#v", terminal.Result["tests"])
		}
	case <-time.After(5 * time.Second):
		t.Fatal("model check did not finish")
	}
	mu.Lock()
	gotRequests := requestCount
	mu.Unlock()
	if gotRequests != 6 {
		t.Fatalf("request count=%d want=6", gotRequests)
	}
	if revealer.calls != 1 || credentials.cacheWrites != 1 || credentials.cached == nil || credentials.cached.Secret != accountKey {
		t.Fatalf("reveals=%d cache writes=%d cached=%#v", revealer.calls, credentials.cacheWrites, credentials.cached)
	}
	tasks.mu.Lock()
	defer tasks.mu.Unlock()
	for _, task := range tasks.tasks {
		encoded, _ := json.Marshal(task)
		if strings.Contains(string(encoded), accountKey) {
			t.Fatalf("account key persisted in task: %s", encoded)
		}
	}
}

func TestPrepareRejectsMissingAndUnsupportedSelections(t *testing.T) {
	service, err := New(
		&recordingTasks{terminal: make(chan taskstore.Task, 1)},
		&fakeCredentials{record: configstore.AuthRecord{Host: "api.example", BaseURL: "https://api.example", UpstreamType: "newapi"}},
		fakeAccounts{rows: []business.AccountStatus{{ID: "41", Name: "主账号"}}},
		&fakeKeyRevealer{secret: "key"},
	)
	if err != nil {
		t.Fatal(err)
	}
	for name, request := range map[string]Request{
		"missing accounts":  {Models: []string{"gpt-5.6-sol"}},
		"missing models":    {AccountIDs: []string{"41"}},
		"unknown account":   {AccountIDs: []string{"99"}, Models: []string{"gpt-5.6-sol"}},
		"unsupported model": {AccountIDs: []string{"41"}, Models: []string{"gpt-4o"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := service.prepare(context.Background(), request); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestPrepareRejectsManualPriorityAccounts(t *testing.T) {
	priority := int64(3)
	service, err := New(
		&recordingTasks{terminal: make(chan taskstore.Task, 1)},
		&fakeCredentials{},
		fakeAccounts{rows: []business.AccountStatus{{ID: "41", Name: "人工账号", ManualPriority: &priority}}},
		&fakeKeyRevealer{secret: "key"},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.prepare(context.Background(), Request{AccountIDs: []string{"41"}, Models: []string{"gpt-5.6-sol"}})
	if err == nil || !strings.Contains(err.Error(), "人工优先位") {
		t.Fatalf("manual priority error=%v", err)
	}
}

func TestResolveCredentialCanRecoverMissingUpstreamAuthorization(t *testing.T) {
	service, err := New(
		&recordingTasks{terminal: make(chan taskstore.Task, 1)},
		missingCredentials{},
		fakeAccounts{},
		&fakeKeyRevealer{secret: "direct-key", expectedKeyID: "91", expectedGroupID: "7"},
	)
	if err != nil {
		t.Fatal(err)
	}
	account := selectedAccount{
		ID: "41", Name: "主账号", BaseURL: "https://api.example/v1", Platform: "openai",
		AuthHost: "auth.example", UpstreamKeyID: "91", UpstreamGroupID: "7",
	}
	if _, err := service.resolveCredential(context.Background(), account); err == nil || !strings.Contains(err.Error(), "未配置可用授权") {
		t.Fatalf("missing resolver error=%v", err)
	}

	resolver := &fakeAuthResolver{record: configstore.AuthRecord{
		Host: "auth.example", BaseURL: "https://auth.example", UpstreamType: "newapi",
	}}
	service.UseUpstreamAuthResolver(resolver)
	credential, err := service.resolveCredential(context.Background(), account)
	if err != nil {
		t.Fatal(err)
	}
	if resolver.host != "auth.example" || resolver.actor != "model-check" {
		t.Fatalf("resolver host=%q actor=%q", resolver.host, resolver.actor)
	}
	if credential.BaseURL != "https://api.example/v1" || credential.Secret != "direct-key" || credential.Platform != "openai" {
		t.Fatalf("credential=%#v", credential)
	}
}

func TestResolveCredentialUsesLocalKeyWithoutReadingUpstreamAuthorization(t *testing.T) {
	credentials := &fakeCredentials{cached: &configstore.UpstreamKeySecret{
		Host: "auth.example", KeyID: "91", GroupID: "7", Secret: "cached-key",
	}}
	revealer := &fakeKeyRevealer{secret: "upstream-key"}
	service, err := New(
		&recordingTasks{terminal: make(chan taskstore.Task, 1)}, credentials, fakeAccounts{}, revealer,
	)
	if err != nil {
		t.Fatal(err)
	}
	credential, err := service.resolveCredential(context.Background(), selectedAccount{
		ID: "41", Name: "主账号", BaseURL: "https://api.example/v1", Platform: "openai",
		AuthHost: "auth.example", UpstreamKeyID: "91", UpstreamGroupID: "7",
	})
	if err != nil {
		t.Fatal(err)
	}
	if credential.Secret != "cached-key" || credentials.authCalls != 0 || revealer.calls != 0 || credentials.cacheReads != 1 {
		t.Fatalf("credential=%#v auth calls=%d reveal calls=%d cache reads=%d", credential, credentials.authCalls, revealer.calls, credentials.cacheReads)
	}
}

func TestCapabilitiesExposeEmbeddedStandards(t *testing.T) {
	service, err := New(
		&recordingTasks{terminal: make(chan taskstore.Task, 1)},
		&fakeCredentials{record: configstore.AuthRecord{Host: "api.example", BaseURL: "https://api.example", UpstreamType: "newapi"}},
		fakeAccounts{},
		&fakeKeyRevealer{secret: "key"},
	)
	if err != nil {
		t.Fatal(err)
	}
	capabilities := service.Capabilities()
	if len(capabilities.ClaudeStandards) != 10 || capabilities.ClaudeStandards[0] != "claude-haiku-4.5" {
		t.Fatalf("unexpected Claude standards: %#v", capabilities.ClaudeStandards)
	}
	if len(capabilities.SolModels) != 3 || capabilities.SolModels[0] != "gpt-5.6-sol" {
		t.Fatalf("unexpected Sol models: %#v", capabilities.SolModels)
	}
}
