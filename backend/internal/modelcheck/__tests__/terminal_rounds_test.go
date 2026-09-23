package modelcheck_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"net/http"
	"sync/atomic"
	"testing"
)

func TestTerminalRoundsRetainEveryResponseAndSuspectedVerdict(t *testing.T) {
	var calls atomic.Int32
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		answer := `{"tool":"exec_command","command":"git status --short"}`
		if calls.Add(1) == 2 {
			answer = "当前没有终端工具"
		}
		fmt.Fprintf(w, `{"output_text":%q}`, answer)
	})
	var input modelcheck.TerminalContinuityRequest
	json.Unmarshal([]byte(`{"targets":[{"account_id":"1","model":"test-model"}],"timeout_seconds":5,"rounds":3}`), &input)
	if _, err := f.service.EnqueueTerminalContinuity(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	task := finished(t, f)
	if calls.Load() != 3 {
		t.Fatalf("rounds executed: %d", calls.Load())
	}
	raw, _ := json.Marshal(task.Result["checks"])
	var checks []struct {
		Verdict string `json:"verdict"`
		Rounds  []struct {
			Round     int    `json:"round"`
			Verdict   string `json:"verdict"`
			RequestID string `json:"request_id"`
		} `json:"round_results"`
	}
	json.Unmarshal(raw, &checks)
	if len(checks) != 1 || len(checks[0].Rounds) != 3 || checks[0].Verdict != "suspected" {
		t.Fatalf("round results lost: %s", raw)
	}
	if checks[0].Rounds[0].RequestID == checks[0].Rounds[1].RequestID || checks[0].Rounds[2].Round != 3 {
		t.Fatal("round identity lost")
	}
}

func TestTerminalRoundsRejectOutsideSupportedRange(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("invalid rounds generated") })
	for _, rounds := range []int{-1, 21} {
		var input modelcheck.TerminalContinuityRequest
		json.Unmarshal([]byte(fmt.Sprintf(`{"targets":[{"account_id":"1","model":"test-model"}],"timeout_seconds":5,"rounds":%d}`, rounds)), &input)
		if _, err := f.service.EnqueueTerminalContinuity(context.Background(), input); err == nil {
			t.Fatal("invalid rounds accepted")
		}
	}
}
