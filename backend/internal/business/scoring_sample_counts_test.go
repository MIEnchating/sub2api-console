package business

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestAccountScoreCountsUseMatchingEvaluationRound(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload map[string]any
		offset  time.Duration
		count   int
		want    any
	}{
		{"same round uses actual short count", map[string]any{"short_sample_count": 10}, 0, 58, float64(10)},
		{"small sample uses actual available count", map[string]any{"short_sample_count": 1}, 0, 1, float64(1)},
		{"legacy round does not guess configured window", map[string]any{}, 0, 58, nil},
		{"new evaluation cannot reuse old count", map[string]any{"short_sample_count": 10}, time.Second, 58, nil},
		{"empty evaluation has zero short samples", map[string]any{}, 0, 0, float64(0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := openPolicyStore(t)
			ctx := context.Background()
			now := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
			if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,updated_at) VALUES('41','sample-count-test','now')`); err != nil {
				t.Fatal(err)
			}
			score := 75.0
			evaluations := []RoutingEvaluationWrite{{AccountID: "41", GroupName: "codex", HealthScore: &score, ShortScore: &score, LongScore: &score, SampleCount: tc.count}}
			decisions := []RoutingDecisionWrite{{AccountID: "41", GroupName: "codex", State: "healthy", Payload: tc.payload}}
			if err := store.PersistRoutingRound(ctx, nil, nil, evaluations, decisions, nil, nil, nil, true, now); err != nil {
				t.Fatal(err)
			}
			if tc.offset > 0 {
				if err := store.PersistRoutingRound(ctx, nil, nil, evaluations, nil, nil, nil, nil, false, now.Add(tc.offset)); err != nil {
					t.Fatal(err)
				}
			}
			account := &accountProjection{AccountStatus: AccountStatus{ID: "41"}, latestEvents: map[string]string{}}
			values, err := store.loadAccountEvaluations(ctx, map[string]*accountProjection{"41": account})
			if err != nil {
				t.Fatal(err)
			}
			applyAccountCalculations(account, nil, values["41"], struct {
				message string
				at      *string
			}{}, routingApplyView{})
			encoded, err := json.Marshal(account.AccountStatus)
			if err != nil {
				t.Fatal(err)
			}
			var actual map[string]any
			if err := json.Unmarshal(encoded, &actual); err != nil {
				t.Fatal(err)
			}
			short, present := actual["short_sample_count"]
			if !present || short != tc.want || actual["long_sample_count"] != float64(tc.count) {
				t.Fatalf("sample counts short=%v long=%v, want short=%v long=%d", short, actual["long_sample_count"], tc.want, tc.count)
			}
		})
	}
}
