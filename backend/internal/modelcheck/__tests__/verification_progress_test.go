package modelcheck_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type verificationProgressStore struct {
	modelcheck.TaskStore
	current chan taskstore.Task
}

func (s *verificationProgressStore) Save(ctx context.Context, task taskstore.Task) error {
	if err := s.TaskStore.Save(ctx, task); err != nil {
		return err
	}
	active, _ := task.Result["active"].(map[int]map[string]any)
	current, _ := active[0]["current_probe"].(map[string]any)
	if current["id"] == "constraints" {
		select {
		case s.current <- task:
		default:
		}
	}
	return nil
}

func TestVerificationProgressIncludesCurrentProbeAndCompletedEvidence(t *testing.T) {
	secondStarted := make(chan struct{})
	release := make(chan struct{})
	var requests atomic.Int32
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		if requests.Add(1) == 2 {
			close(secondStarted)
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		_, _ = io.WriteString(w, `{"output_text":"**21**","model":"test-model"}`)
	})
	defer close(release)
	store := &verificationProgressStore{TaskStore: f.tasks, current: make(chan taskstore.Task, 1)}
	service, err := modelcheck.New(store, credentials{}, f.catalog, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	service.UseTaskRunner(f.runner)
	queued, err := service.Enqueue(t.Context(), modelcheck.Request{AccountIDs: []string{"1"}, Models: []string{"test-model", "next-model"}, Rounds: 1, TimeoutSeconds: 5})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-secondStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("second request did not start")
	}
	var snapshot taskstore.Task
	select {
	case snapshot = <-store.current:
	case <-time.After(3 * time.Second):
		t.Fatal("current probe was not published while waiting for response")
	}
	persisted, err := f.tasks.Get(t.Context(), queued.ID)
	if err != nil || persisted.Status != "running" {
		t.Fatalf("progress was not persisted: %v", err)
	}
	raw, _ := json.Marshal(snapshot.Result)
	var result struct {
		Active map[string]struct {
			Current   *modelcheck.VerificationProbe  `json:"current_probe"`
			Plan      []modelcheck.VerificationProbe `json:"probe_plan"`
			Probes    []modelcheck.VerificationProbe `json:"probes"`
			Completed int                            `json:"completed_probes"`
		} `json:"active"`
		Queued []map[string]any `json:"queued"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	active := result.Active["0"]
	if active.Current == nil || active.Current.ID != "constraints" || active.Current.Status != "running" || active.Current.RequestID == "" || active.Current.Prompt == "" || active.Current.StartedAt == "" {
		t.Fatalf("current probe incomplete: %+v", active.Current)
	}
	if active.Completed != 1 || len(active.Probes) != 1 || active.Probes[0].Answer != "**21**" || len(active.Plan) != 7 {
		t.Fatalf("missing completed evidence or plan: %+v", active)
	}
	if snapshot.Progress <= 5 || snapshot.Progress >= 50 {
		t.Fatalf("progress must advance within the first combination: %d", snapshot.Progress)
	}
	if len(result.Queued) != 1 || result.Queued[0]["claimed_model"] != "next-model" {
		t.Fatalf("queued model missing: %+v", result.Queued)
	}
}

func TestVerificationRateLimitKeepsUnexecutedPlanWithoutRunningProbe(t *testing.T) {
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTooManyRequests) })
	_, err := f.service.Enqueue(t.Context(), modelcheck.Request{AccountIDs: []string{"1"}, Models: []string{"test-model"}, Rounds: 2, TimeoutSeconds: 5})
	if err != nil {
		t.Fatal(err)
	}
	final := finished(t, f)
	row := final.Result["tests"].([]map[string]any)[0]
	raw, _ := json.Marshal(row)
	var report modelcheck.VerificationReport
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	if report.CurrentProbe != nil || len(report.ProbePlan) != 14 || len(report.Probes) != 1 || report.Probes[0].ErrorCode != "rate_limited" {
		t.Fatalf("early stop lost progress evidence: %s", raw)
	}
}
