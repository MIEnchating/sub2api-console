package modelcheck_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestMultipleModelsKeepEveryAccountStageAndUniqueRequest(t *testing.T) {
	var mu sync.Mutex
	calls := map[string]int{}
	f := setup(t, 2, "openai", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model     string `json:"model"`
			Reasoning struct {
				Effort string `json:"effort"`
			} `json:"reasoning"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		mu.Lock()
		calls[body.Model+":"+body.Reasoning.Effort]++
		mu.Unlock()
		answer := fixtureSVG
		if body.Reasoning.Effort == "medium" {
			answer = "21"
		}
		fmt.Fprintf(w, `{"output_text":%q}`, answer)
	})
	input := request("1", "2")
	input.Mode = "both"
	for i := range input.Targets {
		input.Targets[i].Models = []string{"first-model", " second-model "}
	}
	queued, err := f.service.EnqueueAnimation(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if queued.Result["total"] != 8 {
		t.Fatalf("queued total: %v", queued.Result["total"])
	}
	final := finished(t, f)
	if final.Status != "succeeded" || final.Result["completed"] != 8 || final.Result["total"] != 8 {
		t.Fatalf("incomplete batch: %#v", final)
	}
	rows := final.Result["animations"].([]modelcheck.AnimationResult)
	ids := map[string]bool{}
	for _, row := range rows {
		if ids[row.RequestID] {
			t.Fatalf("duplicate request ID %s", row.RequestID)
		}
		ids[row.RequestID] = true
	}
	if len(ids) != 8 {
		t.Fatalf("lost results: %d", len(ids))
	}
	mu.Lock()
	defer mu.Unlock()
	for _, key := range []string{"first-model:low", "first-model:medium", "second-model:low", "second-model:medium"} {
		if calls[key] != 2 {
			t.Errorf("%s requests: %d", key, calls[key])
		}
	}
}

func TestCustomMultipleModelsKeepSuccessAfterOneFailsWithoutPersistingKey(t *testing.T) {
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Model == "unavailable-model" {
			http.Error(w, "model unavailable", http.StatusBadRequest)
			return
		}
		fmt.Fprintf(w, `{"output_text":%q}`, fixtureSVG)
	})
	input := customRequest(t, *f.catalog.details["1"].BaseURL, "openai", fixtureSecret, "unavailable-model")
	input.Custom.Models = []string{"unavailable-model", "working-model"}
	queued, err := f.service.EnqueueAnimation(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	final := finished(t, f)
	if final.Status != "partial" || final.Result["completed"] != 2 {
		t.Fatalf("batch: %#v", final)
	}
	rows := final.Result["animations"].([]modelcheck.AnimationResult)
	if len(rows) != 2 {
		t.Fatalf("missing model: %#v", rows)
	}
	for _, row := range rows {
		if row.Model == "working-model" && row.Status != "succeeded" {
			t.Fatal("second model did not finish")
		}
	}
	stored, err := f.tasks.Get(context.Background(), queued.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(stored)
	if strings.Contains(string(raw), fixtureSecret) || stored.Recovery != nil {
		t.Fatal("temporary credential persisted")
	}
}

func TestMultipleModelsRejectEmptyDuplicateInvalidAndExcessLists(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("invalid request reached upstream") })
	cases := [][]string{{}, {""}, {"m", " m "}, {"m", "bad\nmodel"}, {"m", strings.Repeat("x", 257)}}
	tooMany := []string{}
	for i := 0; i < 17; i++ {
		tooMany = append(tooMany, fmt.Sprint("model-", i))
	}
	cases = append(cases, tooMany)
	for _, models := range cases {
		input := request("1")
		input.Targets[0].Models = models
		if _, err := f.service.EnqueueAnimation(context.Background(), input); err == nil {
			t.Errorf("accepted invalid list %#v", models)
		}
	}
	input := customRequest(t, "https://example.invalid", "openai", fixtureSecret, "model")
	input.Custom.Models = []string{"model", fixtureSecret}
	if _, err := f.service.EnqueueAnimation(context.Background(), input); err == nil {
		t.Fatal("accepted secret in additional model")
	}
}

func TestMultipleModelsSchedulesAndManagedTasksSurviveRestart(t *testing.T) {
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) { fmt.Fprintf(w, `{"output_text":%q}`, fixtureSVG) })
	models := []string{"first-model", "second-model"}
	_, err := f.service.SaveAnimationSchedule(context.Background(), modelcheck.AnimationSchedule{AccountID: "1", Enabled: true, Model: models[0], Models: models, IntervalMinutes: 10, TimeoutSeconds: 5}, "test")
	if err != nil {
		t.Fatal(err)
	}
	f.catalog.groupMembers = map[string][]string{"7": {"1"}, "8": {"1"}}
	value := managedTask()
	value.Precheck = false
	value.Models = models
	plans, err := f.service.SaveDetectionTask(context.Background(), value, "test")
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := modelcheck.New(f.tasks, credentials{}, f.catalog, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	runner := &deferredRunner{}
	restarted.UseTaskRunner(runner)
	restarted.RunDueAnimations(context.Background(), time.Now().Add(time.Hour))
	if len(runner.runs) != 1 {
		t.Fatal("schedule not dispatched")
	}
	runner.runs[0](context.Background())
	if final := finished(t, f); len(final.Result["animations"].([]modelcheck.AnimationResult)) != 2 {
		t.Fatal("schedule lost models")
	}
	if _, err := restarted.RunDetectionTask(context.Background(), plans[0].ID, plans[0].Version); err != nil {
		t.Fatal(err)
	}
	runner.runs[1](context.Background())
	if final := finished(t, f); final.Result["total"] != 2 || len(final.Result["animations"].([]modelcheck.AnimationResult)) != 2 {
		t.Fatalf("managed models lost: %#v", final)
	}
}

func TestMultipleModelsManualDeadlineCoversSequentialModels(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("cancelled task reached upstream") })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	credential := &deadlineCredentials{cancel: cancel}
	service, err := modelcheck.New(f.tasks, credential, f.catalog, credential)
	if err != nil {
		t.Fatal(err)
	}
	runner := &deferredRunner{}
	service.UseTaskRunner(runner)
	input := request("1")
	input.TimeoutSeconds = 120
	for i := 0; i < 16; i++ {
		input.Targets[0].Models = append(input.Targets[0].Models, fmt.Sprint("model-", i))
	}
	if _, err := service.EnqueueAnimation(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	runner.runs[0](ctx)
	credential.mu.Lock()
	remaining := time.Until(credential.deadline)
	credential.mu.Unlock()
	if remaining < 95*time.Minute {
		t.Fatalf("multi-model deadline too short: %s", remaining)
	}
	if final := finished(t, f); final.Status != "cancelled" {
		t.Fatalf("cancellation: %s", final.Status)
	}
}
