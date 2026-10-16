package onboarding_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/onboarding"
)

func TestProbeVerificationReusesQuestionBankAndCleansTemporaryKey(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		requests int
	}{
		{"complete report", 17}, {"retained key", 17}, {"cleanup failure", 17}, {"behavior fingerprint", 217}, {"unauthorized", 1}, {"credential echo", 17}, {"credential in returned model", 17}, {"cancelled", 1},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var calls atomic.Int32
			var closePrivate func()
			gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if scenario.name == "cleanup failure" && calls.Load() == 1 {
					closePrivate()
				}
				var body struct {
					Model, Input string
					Stream       bool
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				if body.Model != "selected-model" || !body.Stream || body.Input == "" {
					t.Errorf("unexpected verification request: %+v", body)
				}
				if scenario.name == "unauthorized" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				if scenario.name == "cancelled" {
					cancel()
					return
				}
				answer := "0"
				responseModel := "selected-model"
				if scenario.name == "credential in returned model" {
					responseModel = testKey
				}
				if scenario.name == "credential echo" {
					answer = testKey
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprintf(w, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"model\":%q,\"output_text\":%q}}\n\n", responseModel, answer)
			}))
			t.Cleanup(gateway.Close)
			service, keys, tasks, runner, _, private := modelOptionsFixtureStores(t, gateway.URL, false)
			closePrivate = func() { _ = private.Close() }
			if scenario.name == "cleanup failure" || scenario.name == "retained key" {
				keys.deleteErr = errors.New("isolated cleanup failure")
			}
			deps := &animationDependencies{}
			checks, err := modelcheck.New(deps, deps, deps, deps)
			if err != nil {
				t.Fatal(err)
			}
			service.UseAnimationService(checks)
			mode := "verification"
			if scenario.name == "behavior fingerprint" {
				mode = "fingerprint"
			}
			if _, err := service.EnqueueProbe(t.Context(), "probe", "options.test", "6", "selected-model", mode); err != nil {
				t.Fatal(err)
			}
			runner.run(ctx)
			if int(calls.Load()) != scenario.requests {
				t.Fatalf("requests=%d want=%d", calls.Load(), scenario.requests)
			}
			result, ok := tasks.final.Result["probe_result"].(onboarding.ProbeResult)
			if !ok || result.RequestModel != "selected-model" {
				t.Fatalf("missing probe result: %+v", tasks.final)
			}
			if scenario.name == "complete report" || scenario.name == "behavior fingerprint" {
				if tasks.final.Status != "succeeded" || result.Status != "completed" {
					t.Fatalf("wrong answers must not fail execution: status=%s result=%s", tasks.final.Status, result.Status)
				}
				for _, step := range tasks.final.Result["steps"].([]onboarding.ProbeStep) {
					if step.Stage == "request" && step.Status != "succeeded" {
						t.Fatal("completed requests shown as failed")
					}
				}
			}
			if scenario.name == "unauthorized" && (tasks.final.Status != "failed" || !strings.Contains(result.Message, "权限")) {
				t.Fatalf("request failure needs its actual cause: %+v", result)
			}
			for _, snapshot := range tasks.snapshots {
				raw, _ := json.Marshal(snapshot)
				if strings.Contains(string(raw), testKey) {
					t.Fatal("credential leaked into live snapshot")
				}
			}
			live := false
			for _, snapshot := range tasks.snapshots {
				if snapshot.Result["active"] != nil && snapshot.Progress < 100 {
					live = true
				}
			}
			if !live {
				t.Fatal("missing live verification progress")
			}
			raw, err := json.Marshal(tasks.final)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), testKey) {
				t.Fatal("credential leaked into task")
			}
			var stored struct {
				Result struct {
					Tests []struct {
						Checker         string
						Probes          []modelcheck.VerificationProbe
						Total           int
						CapabilityTotal int `json:"capability_total"`
					}
				}
			}
			if err := json.Unmarshal(raw, &stored); err != nil {
				t.Fatal(err)
			}
			if len(stored.Result.Tests) != 1 || stored.Result.Tests[0].Checker != "verification" || len(stored.Result.Tests[0].Probes) != scenario.requests {
				t.Fatalf("missing shared report: %s", raw)
			}
			if scenario.name == "complete report" && (stored.Result.Tests[0].Total != 17 || stored.Result.Tests[0].CapabilityTotal != 7) {
				t.Fatalf("fingerprint budget counted as capability: %s", raw)
			}
			if scenario.name == "unauthorized" && stored.Result.Tests[0].Probes[0].ErrorCode != "unauthorized" {
				t.Fatalf("missing failure advice: %s", raw)
			}
			if scenario.name == "retained key" {
				retained := false
				for _, step := range tasks.final.Result["steps"].([]onboarding.ProbeStep) {
					retained = retained || (step.Stage == "retain_key" && step.Status == "succeeded")
				}
				if tasks.final.Status != "succeeded" || !retained {
					t.Fatal("safe retained binding must preserve completed detection")
				}
			}
			if scenario.name == "cleanup failure" && (tasks.final.Status != "failed" || !strings.Contains(tasks.final.Message, "清理失败")) {
				t.Fatal("cleanup failure must override completed requests")
			}
			if scenario.name == "cancelled" && tasks.final.Status != "cancelled" {
				t.Fatalf("cancel status=%s", tasks.final.Status)
			}
			if len(keys.created) != 1 || len(keys.deleted) != 1 {
				t.Fatalf("temporary key not cleaned: %+v", keys)
			}
		})
	}
}

func TestProbeVerificationRejectsUnavailableServiceBeforeCreatingKey(t *testing.T) {
	service, keys, _, _ := modelOptionsFixture(t, "http://127.0.0.1:1", false)
	if _, err := service.EnqueueProbe(t.Context(), "probe", "options.test", "6", "selected-model", "verification"); err == nil {
		t.Fatal("missing service accepted")
	}
	if len(keys.created) != 0 {
		t.Fatal("created key before service validation")
	}
}

func TestProbeVerificationDeadlineIsFailureNotManualCancellation(t *testing.T) {
	service, _, tasks, runner := modelOptionsFixture(t, "http://127.0.0.1:1", false)
	deps := &animationDependencies{}
	checks, err := modelcheck.New(deps, deps, deps, deps)
	if err != nil {
		t.Fatal(err)
	}
	service.UseAnimationService(checks)
	if _, err := service.EnqueueProbe(t.Context(), "probe", "options.test", "6", "selected-model", "fingerprint"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()
	runner.run(ctx)
	if tasks.final.Status != "failed" || !strings.Contains(tasks.final.Message, "超时") {
		t.Fatalf("deadline misclassified as manual cancellation: %s %s", tasks.final.Status, tasks.final.Message)
	}
}
