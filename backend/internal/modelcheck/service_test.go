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

type fakeSettings struct {
	value configstore.TargetSettings
}

func (settings fakeSettings) TargetSettings(context.Context) (configstore.TargetSettings, error) {
	return settings.value, nil
}

type fakeAccounts struct {
	rows []business.AccountStatus
}

func (accounts fakeAccounts) Accounts(context.Context) ([]business.AccountStatus, error) {
	return accounts.rows, nil
}

func TestAccountMatrixUsesConfiguredAdminConnectionAndBehaviorMode(t *testing.T) {
	const adminKey = "configured-admin-key"
	requestCount := 0
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()
		if request.URL.Path != "/api/v1/admin/accounts/41/test" || request.Header.Get("x-api-key") != adminKey {
			t.Errorf("unexpected account test request: %s headers=%v", request.URL.Path, request.Header)
		}
		var payload struct {
			ModelID string `json:"model_id"`
			Prompt  string `json:"prompt"`
			Mode    string `json:"mode"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if payload.ModelID != "claude-opus-5" || payload.Mode != "behavior" || !strings.Contains(payload.Prompt, "Return exactly one JSON array") {
			t.Errorf("unexpected payload: %#v", payload)
		}
		answers := make([]string, 12)
		for index := range answers {
			answers[index] = "A"
		}
		encoded, _ := json.Marshal(answers)
		response.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(response, "data: {\"type\":\"test_start\",\"model\":\"claude-opus-5\"}\n\n")
		_, _ = fmt.Fprintf(response, "data: {\"type\":\"content\",\"text\":%q}\n\n", string(encoded))
		_, _ = fmt.Fprintf(response, "data: {\"type\":\"test_complete\",\"success\":true}\n\n")
	}))
	defer server.Close()

	tasks := &recordingTasks{terminal: make(chan taskstore.Task, 1)}
	service, err := New(
		tasks,
		fakeSettings{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: adminKey, TimeoutSeconds: 5}},
		fakeAccounts{rows: []business.AccountStatus{{ID: "41", Name: "主账号"}}},
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
	tasks.mu.Lock()
	defer tasks.mu.Unlock()
	for _, task := range tasks.tasks {
		encoded, _ := json.Marshal(task)
		if strings.Contains(string(encoded), adminKey) {
			t.Fatalf("admin key persisted in task: %s", encoded)
		}
	}
}

func TestPrepareRejectsMissingAndUnsupportedSelections(t *testing.T) {
	service, err := New(
		&recordingTasks{terminal: make(chan taskstore.Task, 1)},
		fakeSettings{value: configstore.TargetSettings{BaseURL: "https://admin.example", AdminKey: "key"}},
		fakeAccounts{rows: []business.AccountStatus{{ID: "41", Name: "主账号"}}},
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

func TestCapabilitiesExposeEmbeddedStandards(t *testing.T) {
	service, err := New(
		&recordingTasks{terminal: make(chan taskstore.Task, 1)},
		fakeSettings{value: configstore.TargetSettings{BaseURL: "https://admin.example", AdminKey: "key"}},
		fakeAccounts{},
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
