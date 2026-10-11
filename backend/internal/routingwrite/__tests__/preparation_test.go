package routingwrite_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/routingwrite"
)

func preparationFixture(count int) *batchAcknowledgementFixture {
	ids := make([]int, count)
	for index := range ids {
		ids[index] = 41 + index
	}
	return &batchAcknowledgementFixture{reply: map[string]any{"success_ids": ids}}
}

func TestPreparationBeyondWorkerLimitStillCombinesAllEligibleAccounts(t *testing.T) {
	t.Parallel()
	fixture := preparationFixture(17)
	service, targets := newBatchAcknowledgementService(t, fixture, 17, false)
	configurationError := "invalid calculated target"
	target := targets["42"]
	target.ConfigurationError = &configurationError
	targets["42"] = target
	target = targets["43"]
	target.GroupNames = nil
	targets["43"] = target
	target = targets["44"]
	unchanged := int64(1000)
	target.Priority = &unchanged
	targets["44"] = target
	// Rejected preparations and no-op accounts must all arrive at the barrier.
	fixture.reply = map[string]any{"success_ids": []int{41, 45, 46, 47, 48, 49, 50, 51, 52, 53, 54, 55, 56, 57}}
	fixture.onWrite = func() {
		if fixture.preparationReads != 15 {
			t.Errorf("write began before all valid accounts were read: %d", fixture.preparationReads)
		}
	}
	result, err := service.Apply(t.Context(), targets, "test")
	if err != nil || len(result.Results) != 17 || result.Failed != 2 || result.Changed != 14 {
		t.Fatalf("mixed preparation lost results: %+v err=%v", result, err)
	}
	if fixture.writes != 1 || fixture.readbacks != 0 {
		t.Fatalf("batch was split or acknowledgements lost: writes=%d reads=%d", fixture.writes, fixture.readbacks)
	}
}

func TestRestoreBeyondWorkerLimitReturnsEveryAccountToItsSavedBaseline(t *testing.T) {
	t.Parallel()
	fixture := preparationFixture(17)
	service, targets := newBatchAcknowledgementService(t, fixture, 17, false)
	initial, err := service.Apply(t.Context(), targets, "test")
	if err != nil || initial.Changed != 17 || initial.Failed != 0 {
		t.Fatalf("initial apply: %+v %v", initial, err)
	}
	result, err := service.RestoreControl(t.Context(), "test")
	if err != nil || result.Restored != 17 || result.Failed != 0 {
		t.Fatalf("restore lost accounts: %+v %v", result, err)
	}
	baselines, err := fixture.store.RoutingBaselines(t.Context())
	if err != nil || len(baselines) != 0 {
		t.Fatalf("confirmed restore retained baselines: %v %v", baselines, err)
	}
	for id, account := range fixture.states {
		if account["priority"] != 1000 {
			t.Fatalf("account %s did not restore priority: %v", id, account)
		}
	}
}

func TestCancelledPreparationCompletesQueuedResultsWithoutWriting(t *testing.T) {
	t.Parallel()
	fixture := preparationFixture(17)
	started := make(chan struct{})
	var once sync.Once
	fixture.onPrepare = func(request *http.Request) {
		once.Do(func() { close(started) })
		<-request.Context().Done()
	}
	service, targets := newBatchAcknowledgementService(t, fixture, 17, false)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	type outcome struct {
		result routingwrite.Result
		err    error
	}
	done := make(chan outcome, 1)
	go func() { result, err := service.Apply(ctx, targets, "test"); done <- outcome{result, err} }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("preparation did not start")
	}
	cancel()
	select {
	case completed := <-done:
		if completed.err != nil || len(completed.result.Results) != 17 || completed.result.Failed != 17 || completed.result.Changed != 0 {
			t.Fatalf("cancellation lost queued outcomes: %+v %v", completed.result, completed.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled preparation did not finish")
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.writes != 0 {
		t.Fatalf("cancelled batch wrote %d times", fixture.writes)
	}
}

func TestDifferentPayloadWritesKeepAvailableWorkersBusyWhenFirstAccountIsSlow(t *testing.T) {
	t.Parallel()
	fixture := preparationFixture(3)
	_, targets := newBatchAcknowledgementService(t, fixture, 3, false)
	for index, id := range []string{"41", "42", "43"} {
		target := targets[id]
		priority := int64(100 * (index + 1))
		target.Priority = &priority
		targets[id] = target
	}
	firstStarted, releaseFirst := make(chan struct{}), make(chan struct{})
	var firstOnce, releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(releaseFirst) }) }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPut {
			fixture.ServeHTTP(w, request)
			return
		}
		id := strings.TrimPrefix(request.URL.Path, "/api/v1/admin/accounts/")
		switch id {
		case "41":
			firstOnce.Do(func() { close(firstStarted) })
			select {
			case <-releaseFirst:
			case <-request.Context().Done():
				return
			}
		case "42":
			select {
			case <-firstStarted:
			case <-request.Context().Done():
				return
			}
		case "43":
			unblock()
		}
		var fields struct {
			Priority int `json:"priority"`
		}
		if err := json.NewDecoder(request.Body).Decode(&fields); err != nil {
			t.Error(err)
			http.Error(w, "invalid fields", http.StatusBadRequest)
			return
		}
		fixture.mu.Lock()
		defer fixture.mu.Unlock()
		fixture.states[id]["priority"] = fields.Priority
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": fixture.states[id]})
	}))
	t.Cleanup(server.Close)
	t.Cleanup(unblock)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	service := routingwrite.New(routingTarget{url: server.URL}, fixture.store)
	result, err := service.Apply(ctx, targets, "test")
	if err != nil || result.Failed != 0 || result.Changed != 3 {
		t.Fatalf("available worker left the third account waiting behind the slow first account: %+v err=%v", result, err)
	}
}

func BenchmarkRoutingPreparation(b *testing.B) {
	for _, count := range []int{32, 256} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			fixture := preparationFixture(count)
			service, targets := newBatchAcknowledgementService(b, fixture, count, false)
			if _, err := fixture.store.UpdatePolicy(b.Context(), map[string]any{"advanced_policy": map[string]any{"scope": map[string]any{"manage_all_accounts": true}}}, "test"); err != nil {
				b.Fatal(err)
			}
			var atWrite, baseline, measured int
			fixture.onWrite = func() { atWrite += runtime.NumGoroutine() - baseline; measured++ }
			b.ReportAllocs()
			for b.Loop() {
				fixture.mu.Lock()
				for _, account := range fixture.states {
					account["priority"] = 1000
				}
				fixture.mu.Unlock()
				baseline = runtime.NumGoroutine()
				result, err := service.Apply(b.Context(), targets, "benchmark")
				if err != nil || result.Changed != count || result.Failed != 0 {
					b.Fatalf("apply: %+v %v", result, err)
				}
			}
			if measured > 0 {
				b.ReportMetric(float64(atWrite)/float64(measured), "goroutines-at-write")
			}
		})
	}
}
