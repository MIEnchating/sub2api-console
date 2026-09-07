package accountops

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/probe"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type modelSyncTaskObserver struct {
	updates chan taskstore.Task
}

type modelSyncProbeRecorder struct {
	requests []probe.Request
}

func (recorder *modelSyncProbeRecorder) RunNow(_ context.Context, request probe.Request) (probe.RunSummary, error) {
	recorder.requests = append(recorder.requests, request)
	result := probe.Result{AccountID: "41", RequestModel: request.ProbeModel}
	switch request.ProbeModel {
	case "model-a":
		return probe.RunSummary{Targets: 1, Persisted: 1, Passed: 1, Results: []probe.Result{result}}, nil
	case "model-b":
		return probe.RunSummary{Targets: 1, Persisted: 1, Failed: 1, Results: []probe.Result{result}}, fmt.Errorf("probe failed")
	default:
		return probe.RunSummary{Targets: 1, Persisted: 1, Skipped: 1, Results: []probe.Result{result}}, nil
	}
}

func (observer *modelSyncTaskObserver) Save(_ context.Context, task taskstore.Task) error {
	if task.Status == "cancelled" {
		observer.updates <- task
	}
	return nil
}

func TestValidateUnifiedProbeModelsAcceptsModelsSupportedByPartOfBatch(t *testing.T) {
	accounts := []AccountModelSelection{
		{AccountID: "41", Models: []string{"model-a", "model-c"}},
		{AccountID: "42", Models: []string{"model-a", "model-b", "model-c"}},
	}
	result, err := validateUnifiedProbeModels(accounts, []string{"MODEL-B", "model-a", "model-b"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(result, []string{"model-b", "model-a"}) {
		t.Fatalf("probe models=%#v", result)
	}
	if _, err := validateUnifiedProbeModels(accounts, []string{"model-e"}); err == nil {
		t.Fatal("unselected unified probe model was accepted")
	}
	if _, err := validateUnifiedProbeModels(accounts, []string{"missing"}); err == nil {
		t.Fatal("unknown unified probe model was accepted")
	}
	if _, err := validateUnifiedProbeModels(accounts, nil); err == nil {
		t.Fatal("empty unified probe model selection was accepted")
	}
	tooMany := make([]string, maximumUnifiedProbeModels+1)
	for index := range tooMany {
		tooMany[index] = fmt.Sprintf("model-%d", index)
	}
	if _, err := validateUnifiedProbeModels(accounts, tooMany); err == nil {
		t.Fatal("too many unified probe models were accepted")
	}
}

func TestValidateAccountModelSelectionsKeepsAccountsIndependentAndRejectsBlockedModels(t *testing.T) {
	accounts := []business.AccountModelSyncAccount{
		{AccountID: "41", Models: []string{"common", "model-a", "gpt-image-1"}},
		{AccountID: "42", Models: []string{"common", "model-b", "gpt-image-1"}},
	}
	result, err := validateAccountModelSelections(accounts, []string{"*-image-*"}, []AccountModelSelection{
		{AccountID: "41", Models: []string{"common"}},
		{AccountID: "42", Models: []string{"common", "MODEL-B"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(result[0].Models, []string{"common"}) || !slices.Equal(result[1].Models, []string{"common", "model-b"}) {
		t.Fatalf("normalized selections=%#v", result)
	}
	if _, err := validateAccountModelSelections(accounts, []string{"*-image-*"}, []AccountModelSelection{
		{AccountID: "41", Models: []string{"gpt-image-1"}},
		{AccountID: "42", Models: []string{"common"}},
	}); err == nil {
		t.Fatal("globally blocked model was accepted")
	}
}

func TestMergeProbeSummaryAggregatesAllSelectedModels(t *testing.T) {
	summary := probe.RunSummary{Targets: 1, Persisted: 1, Passed: 1, Results: []probe.Result{{AccountID: "41", RequestModel: "model-a"}}}
	mergeProbeSummary(&summary, probe.RunSummary{
		Targets: 2, Persisted: 2, Failed: 1, Skipped: 1,
		Results: []probe.Result{{AccountID: "42", RequestModel: "model-b"}},
	})
	if summary.Targets != 3 || summary.Persisted != 3 || summary.Passed != 1 || summary.Failed != 1 || summary.Skipped != 1 {
		t.Fatalf("merged summary=%#v", summary)
	}
	if len(summary.Results) != 2 || summary.Results[1].RequestModel != "model-b" {
		t.Fatalf("merged results=%#v", summary.Results)
	}
}

func TestRunUnifiedProbeModelsContinuesAndAggregatesAfterOneModelFails(t *testing.T) {
	recorder := &modelSyncProbeRecorder{}
	service := &Service{modelSyncProbe: recorder}
	progress := []int{}
	summary, err := service.runUnifiedProbeModels(
		context.Background(),
		[]string{"41", "42"},
		[]string{"model-a", "model-b", "model-c"},
		func(index, _ int) { progress = append(progress, index) },
	)
	if err == nil || err.Error() != "model-b: probe failed" {
		t.Fatalf("probe error=%v", err)
	}
	if len(recorder.requests) != 3 || recorder.requests[2].ProbeModel != "model-c" {
		t.Fatalf("probe requests=%#v", recorder.requests)
	}
	for _, request := range recorder.requests {
		if !slices.Equal(request.AccountIDs, []string{"41", "42"}) || !request.Automatic || !request.OnePerAccount {
			t.Fatalf("probe request=%#v", request)
		}
	}
	if !slices.Equal(progress, []int{0, 1, 2}) {
		t.Fatalf("progress callbacks=%#v", progress)
	}
	if summary.Targets != 3 || summary.Persisted != 3 || summary.Passed != 1 || summary.Failed != 1 || summary.Skipped != 1 || len(summary.Results) != 3 {
		t.Fatalf("probe summary=%#v", summary)
	}
}

func TestModelDiscoveryCancellationStopsWorkersBeforeTheyClaimAnotherAccount(t *testing.T) {
	repository, db, _ := accountRepository(t)
	for accountID := 42; accountID <= 46; accountID++ {
		if _, err := db.Exec(`INSERT INTO accounts(
			id,name,schedulable,priority,load_factor,concurrency,multiplier,metadata_json,updated_at
		) VALUES(?,?,1,10,'2',3,'0.1','{}','now')`, fmt.Sprint(accountID), fmt.Sprintf("account-%d", accountID)); err != nil {
			t.Fatal(err)
		}
	}
	entered := make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if calls.Add(1) == 1 {
			close(entered)
		}
		<-request.Context().Done()
		_, _ = io.WriteString(writer, `{}`)
	}))
	defer server.Close()
	target := &testTarget{value: configstore.TargetSettings{
		BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 5,
	}}
	observer := &modelSyncTaskObserver{updates: make(chan taskstore.Task, 1)}
	service := New(target, repository, observer)
	accountIDs := []string{"41", "42", "43", "44", "45", "46"}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{
		ID: "cancel-model-discovery", Skill: "sub2api-account-model-sync",
		Operation: "account-model-discovery", Status: "queued", Message: "queued",
		Result: map[string]any{}, CreatedAt: now, UpdatedAt: now,
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		service.executeModelDiscovery(ctx, task, accountIDs, "operator")
	}()
	select {
	case <-entered:
		cancel()
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("initial discovery workers did not reach the remote request")
	}
	select {
	case finished := <-observer.updates:
		if finished.Status != "cancelled" {
			t.Fatalf("final task=%#v", finished)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled discovery task did not finish")
	}
	<-done
	if calls.Load() != 1 {
		t.Fatalf("remote calls=%d, want only the in-flight request", calls.Load())
	}
}

func TestFilteredAccountModelMappingRemovesDisallowedModelsAndPreservesAllowedAliases(t *testing.T) {
	result := filteredAccountModelMapping(map[string]string{
		"model-a": "model-a", "model-e": "model-e", "public-c": "model-c", "*": "model-a",
	}, []string{"model-a", "model-b", "model-c"})
	want := map[string]string{"model-a": "model-a", "model-b": "model-b", "model-c": "model-c", "public-c": "model-c"}
	if !equalStringMap(result, want) {
		t.Fatalf("filtered mapping=%#v want=%#v", result, want)
	}
}

func TestApplyAccountModelsWritesFilteredMappingAndConfirmsReadback(t *testing.T) {
	repository, db, _ := accountRepository(t)
	var requestMu sync.Mutex
	mapping := map[string]any{"model-a": "model-a", "model-e": "model-e", "public-c": "model-c"}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case http.MethodGet:
			requestMu.Lock()
			body, _ := json.Marshal(map[string]any{"data": map[string]any{
				"id": 41, "credentials": map[string]any{"base_url": "https://provider.example/v1", "model_mapping": mapping},
			}})
			requestMu.Unlock()
			_, _ = writer.Write(body)
		case http.MethodPut:
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			credentials := body["credentials"].(map[string]any)
			if credentials["base_url"] != "https://provider.example/v1" {
				t.Fatalf("non-sensitive credentials were not preserved: %#v", credentials)
			}
			requestMu.Lock()
			mapping = credentials["model_mapping"].(map[string]any)
			requestMu.Unlock()
			_, _ = writer.Write([]byte(`{"data":{"id":41}}`))
		default:
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()
	service := New(&testTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 2}}, repository, nil)
	item := service.applyAccountModels(context.Background(), business.AccountModelCatalog{
		AccountID: "41", AccountName: "alpha", Models: []string{"model-a", "model-b", "model-c", "model-e"},
	}, []string{"model-a", "model-b", "model-c"}, "operator")
	if item.Status != "succeeded" || item.ModelCount != 4 || item.Error != "" {
		t.Fatalf("apply result=%#v", item)
	}
	requestMu.Lock()
	confirmed, err := accountModelMapping(mapping)
	requestMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"model-a": "model-a", "model-b": "model-b", "model-c": "model-c", "public-c": "model-c"}
	if !equalStringMap(confirmed, want) {
		t.Fatalf("confirmed mapping=%#v", confirmed)
	}
	var operations int
	if err := db.QueryRow(`SELECT COUNT(*) FROM operation_audit WHERE operation_type='account.models.sync' AND object_id='41'`).Scan(&operations); err != nil {
		t.Fatal(err)
	}
	if operations != 1 {
		t.Fatalf("audit operations=%d", operations)
	}
}

func TestApplyAccountModelsSkipsEmptyIntersectionWithoutRemoteWrite(t *testing.T) {
	repository, _, _ := accountRepository(t)
	service := New(&testTarget{}, repository, nil)
	item := service.applyAccountModels(context.Background(), business.AccountModelCatalog{
		AccountID: "41", AccountName: "alpha", Models: []string{"model-e"},
	}, []string{}, "operator")
	if item.Status != "skipped" || item.Error == "" {
		t.Fatalf("empty intersection result=%#v", item)
	}
}
