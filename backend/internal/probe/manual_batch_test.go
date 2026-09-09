package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestManualBatchUsesOnlySelectedAccountsAndConfiguredModels(t *testing.T) {
	repository := &fakeRepository{
		policy: map[string]any{
			"probe":               map[string]any{"enabled": false},
			"account_test_models": map[string]any{"41": []any{"model-a", "model-b"}},
		},
		candidates: []business.ProbeCandidate{
			{AccountID: "41", GroupName: "a", KnownModels: []string{"model-a", "model-b"}},
			{AccountID: "41", GroupName: "b", KnownModels: []string{"model-a", "model-b"}},
			{AccountID: "42", GroupName: "a", KnownModels: []string{"model-c"}},
			{AccountID: "43", GroupName: "a", KnownModels: []string{"unselected-model"}},
		},
	}
	service := New(repository, fakeSettings{target: configstore.TargetSettings{
		BaseURL: "http://127.0.0.1:1", AdminKey: "test", TimeoutSeconds: 1,
	}}, &observingTasks{})
	prepared, err := service.prepare(context.Background(), Request{SelectedAccountIDs: []string{"41", "42"}})
	if err != nil {
		t.Fatal(err)
	}
	models := []string{}
	ids := []string{}
	for _, target := range prepared.targets {
		ids = append(ids, target.AccountID)
		models = append(models, *target.Model)
	}
	if !reflect.DeepEqual(ids, []string{"41", "42"}) || !reflect.DeepEqual(models, []string{"model-a", "model-c"}) {
		t.Fatalf("ids=%v models=%v", ids, models)
	}
}

func TestManualBatchRejectsMissingOrProtectedAccountInsteadOfSilentlyShrinkingScope(t *testing.T) {
	_, err := selectManualCandidates([]business.ProbeCandidate{{AccountID: "41"}}, []string{"41", "42"})
	if err == nil {
		t.Fatal("missing selected account was silently ignored")
	}
}

func TestManualBatchRejectsEmptyDuplicateInvalidAndAutomaticSelections(t *testing.T) {
	for _, request := range []Request{
		{SelectedAccountIDs: []string{}}, {SelectedAccountIDs: []string{"0"}},
		{SelectedAccountIDs: []string{"41", "41"}}, {SelectedAccountIDs: make([]string, 101)},
		{SelectedAccountIDs: []string{"41"}, Automatic: true},
		{SelectedAccountIDs: []string{"41"}, AccountIDs: []string{"42"}},
	} {
		if _, err := manualBatchIDs(request); err == nil {
			t.Fatalf("invalid manual selection accepted: %+v", request)
		}
	}
}

func TestManualBatchRetainsScopeAndBackgroundVisibilityAfterCompletion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/admin/accounts/41/test" {
			t.Errorf("unexpected target: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"content\",\"text\":\"pong\"}\n\n"))
	}))
	defer server.Close()
	repository := &fakeRepository{policy: map[string]any{}, candidates: []business.ProbeCandidate{
		{AccountID: "41", GroupName: "codex", KnownModels: []string{"model-a"}},
		{AccountID: "42", GroupName: "codex", KnownModels: []string{"model-b"}},
	}}
	tasks := &observingTasks{terminal: make(chan taskstore.Task, 1)}
	runner := &deferredProbeRunner{}
	service := New(repository, fakeSettings{target: configstore.TargetSettings{
		BaseURL: server.URL, AdminKey: "test", TimeoutSeconds: 1,
	}}, tasks)
	service.UseTaskRunner(runner)
	queued, err := service.Enqueue(context.Background(), Request{SelectedAccountIDs: []string{"41"}}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if queued.Result["system_info"] != true || !reflect.DeepEqual(queued.Result["account_ids"], []string{"41"}) {
		t.Fatalf("queued task lost scope: %+v", queued)
	}
	runner.Run(context.Background())
	select {
	case completed := <-tasks.terminal:
		if completed.Status != "succeeded" || completed.Result["system_info"] != true || !reflect.DeepEqual(completed.Result["account_ids"], []string{"41"}) {
			t.Fatalf("completed task lost scope: %+v", completed)
		}
	default:
		t.Fatal("batch did not finish")
	}
}
