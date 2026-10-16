package modelcheck_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelfingerprint"
)

func TestVerificationRunsFourProbesTogetherAndPreservesCancelledEvidence(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	entered := make(chan struct{}, 16)
	var calls, active, peak atomic.Int32
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		call := calls.Add(1)
		if call == 1 {
			fmt.Fprint(w, `{"model":"test-model","output_text":"21"}`)
			return
		}
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
		}
		entered <- struct{}{}
		<-r.Context().Done()
	})
	type outcome struct {
		result map[string]any
		err    error
	}
	done := make(chan outcome, 1)
	var sawParallel bool
	go func() {
		result, err := f.service.RunEndpointVerification(ctx, modelcheck.AnimationCustomEndpoint{BaseURL: *f.catalog.details["1"].BaseURL, APIKey: "isolated", Platform: "openai", Model: "test-model"}, func(report modelcheck.VerificationReport) {
			raw, _ := json.Marshal(report)
			var snapshot struct {
				Current []modelcheck.VerificationProbe `json:"current_probes"`
			}
			_ = json.Unmarshal(raw, &snapshot)
			if len(snapshot.Current) == 4 {
				sawParallel = true
			}
		})
		done <- outcome{result, err}
	}()
	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()
	for range 4 {
		select {
		case <-entered:
		case <-timeout.C:
			cancel()
			<-done
			t.Fatal("independent probes did not run concurrently")
		}
	}
	cancel()
	out := <-done
	if out.err != nil {
		t.Fatal(out.err)
	}
	var report modelcheck.VerificationReport
	raw, _ := json.Marshal(out.result)
	_ = json.Unmarshal(raw, &report)
	if peak.Load() != 4 || calls.Load() != 5 || !sawParallel {
		t.Fatalf("bad concurrency: peak=%d calls=%d parallel=%v", peak.Load(), calls.Load(), sawParallel)
	}
	if len(report.Probes) != 5 || report.Probes[0].Answer != "21" || report.Total != 17 {
		t.Fatal("cancel lost evidence or launched queued probes")
	}
	for i, p := range report.Probes {
		if p.ID != report.ProbePlan[i].ID {
			t.Fatal("completion order changed report order")
		}
	}
	if report.CurrentProbe != nil {
		t.Fatal("completed report retained running probe")
	}
}

func TestVerificationParallelLimitCancelsOutstandingRequestsWithoutStartingQueuedProbes(t *testing.T) {
	entered := make(chan struct{}, 4)
	fail := make(chan struct{})
	var calls atomic.Int32
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		n := calls.Add(1)
		if n == 1 {
			fmt.Fprint(w, `{"model":"test-model","output_text":"21"}`)
			return
		}
		entered <- struct{}{}
		if n == 2 {
			<-fail
			w.WriteHeader(429)
			fmt.Fprint(w, `{"error":"limit"}`)
			return
		}
		<-r.Context().Done()
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan map[string]any, 1)
	go func() {
		result, err := f.service.RunEndpointVerification(ctx, modelcheck.AnimationCustomEndpoint{BaseURL: *f.catalog.details["1"].BaseURL, APIKey: "isolated", Platform: "openai", Model: "test-model"}, nil)
		if err != nil {
			t.Error(err)
		}
		done <- result
	}()
	for range 4 {
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			cancel()
			close(fail)
			<-done
			t.Fatal("parallel requests did not start")
		}
	}
	close(fail)
	var result map[string]any
	select {
	case result = <-done:
	case <-time.After(3 * time.Second):
		cancel()
		<-done
		t.Fatal("limit did not cancel outstanding requests")
	}
	raw, _ := json.Marshal(result)
	var report modelcheck.VerificationReport
	_ = json.Unmarshal(raw, &report)
	limited := 0
	for _, row := range report.Probes {
		if row.ErrorCode == "rate_limited" {
			limited++
		}
	}
	if calls.Load() != 5 || limited != 1 || len(report.CurrentProbes) != 0 || len(report.Probes) != 5 {
		t.Fatalf("failure did not stop queue: calls=%d report=%s", calls.Load(), raw)
	}
}

func TestVerificationConcurrentTasksShareSixteenRequestSlots(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	entered := make(chan struct{}, 64)
	var active, peak atomic.Int32
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Input string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		// The first capability request is a protocol check; all later requests wait.
		if strings.HasPrefix(body.Input, "计算") {
			fmt.Fprint(w, `{"output_text":"21"}`)
			return
		}
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
		}
		entered <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 5)
	for range 5 {
		go func() {
			_, err := f.service.RunEndpointVerification(ctx, modelcheck.AnimationCustomEndpoint{BaseURL: *f.catalog.details["1"].BaseURL, APIKey: "isolated", Platform: "openai", Model: "test-model"}, nil)
			done <- err
		}()
	}
	for range 16 {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			cancel()
			for range 5 {
				<-done
			}
			t.Fatal("global slots were not utilized")
		}
	}
	cancel()
	for range 5 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if peak.Load() != 16 {
		t.Fatalf("global request limit violated: %d", peak.Load())
	}
}

func TestVerificationReferenceCellsPreserveSampleOrderUnderConcurrency(t *testing.T) {
	for _, model := range []string{"qwen3.7-flash", "qwen3.7-plus"} {
		t.Run(model, func(t *testing.T) {
			f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				fmt.Fprintf(w, `{"model":%q,"output_text":"1","choices":[{"finish_reason":"stop","message":{"content":"1"}}],"usage":{"prompt_tokens":10,"completion_tokens":1}}`, model)
			})
			plans := modelfingerprint.Plan(model)
			cells := map[string]string{}
			order := map[string][]string{}
			for _, p := range plans {
				cells[p.ID] = p.Cell
				order[p.Cell] = append(order[p.Cell], p.ID)
			}
			seen := map[string]bool{}
			result, err := f.service.RunEndpointVerification(t.Context(), modelcheck.AnimationCustomEndpoint{BaseURL: *f.catalog.details["1"].BaseURL, APIKey: "isolated", Platform: "openai", Model: model}, func(report modelcheck.VerificationReport) {
				busy := map[string]bool{}
				for _, row := range report.CurrentProbes {
					cell, ok := cells[row.ID]
					if !ok {
						continue
					}
					if busy[cell] {
						t.Error("same cell has overlapping samples")
					}
					busy[cell] = true
					if !seen[row.ID] {
						if len(order[cell]) == 0 || order[cell][0] != row.ID {
							t.Errorf("cell %s sampling reordered", cell)
						} else {
							order[cell] = order[cell][1:]
						}
						seen[row.ID] = true
					}
				}
			}, false, true)
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(result)
			var report modelcheck.VerificationReport
			_ = json.Unmarshal(raw, &report)
			if len(seen) != len(plans) || len(report.Probes) != 17+len(plans) {
				t.Fatal("missing reference samples")
			}
			for i, p := range report.Probes {
				if p.ID != report.ProbePlan[i].ID {
					t.Fatal("report order is not stable")
				}
			}
		})
	}
}
