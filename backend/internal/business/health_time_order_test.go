package business

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestPersistedHealthSamplesSelectNewestAcrossPrecisionAndTimezone(t *testing.T) {
	for _, source := range []string{"traffic", "active-probe"} {
		for _, older := range []string{"2026-09-07T12:00:00Z", "2026-09-07T14:00:00+02:00"} {
			t.Run(source+"/"+older, func(t *testing.T) {
				store := openPolicyStore(t)
				ctx := context.Background()
				newest := "2026-09-07T12:00:00.1Z"
				if source == "traffic" {
					_, err := store.PersistTrafficSamples(ctx, []TrafficSample{
						{AccountID: "41", GroupName: "codex", ObservedAt: older, EvidenceKey: "older", Result: "失败", Payload: map[string]any{}},
						{AccountID: "41", GroupName: "codex", ObservedAt: newest, EvidenceKey: "newest", Result: "通过", Payload: map[string]any{}},
					})
					if err != nil {
						t.Fatal(err)
					}
				} else {
					_, err := store.PersistProbeSamples(ctx, []ProbeSample{
						{AccountID: "41", GroupName: "codex", ObservedAt: older, Result: "失败"},
						{AccountID: "41", GroupName: "codex", ObservedAt: newest, Result: "通过"},
					})
					if err != nil {
						t.Fatal(err)
					}
				}
				samples, err := store.RoutingSamples(ctx, nil, nil, source, 1)
				if err != nil || len(samples) != 1 || samples[0].Result != "通过" {
					t.Fatalf("latest routing evidence is not the actual newest sample: samples=%#v err=%v", samples, err)
				}
			})
		}
	}
}

func TestHealthRetentionRemovesOldestWholeSecondBeforeNewerFractionalSamples(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	base := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	inputs := make([]TrafficSample, retainedHealthSamplesPerAccount+1)
	for index := range inputs {
		inputs[index] = TrafficSample{
			AccountID: "41", GroupName: "codex", ObservedAt: base.Add(time.Duration(index) * time.Millisecond).Format(time.RFC3339Nano),
			EvidenceKey: fmt.Sprintf("request-%d", index), Result: "通过", Payload: map[string]any{},
		}
	}
	if _, err := store.PersistTrafficSamples(ctx, inputs); err != nil {
		t.Fatal(err)
	}
	samples, err := store.RoutingSamples(ctx, nil, nil, "traffic", len(inputs))
	if err != nil || len(samples) != retainedHealthSamplesPerAccount {
		t.Fatalf("retained samples=%d err=%v", len(samples), err)
	}
	for _, sample := range samples {
		observed, err := time.Parse(time.RFC3339Nano, sample.ObservedAt)
		if err != nil || !observed.After(base) {
			t.Fatalf("retention kept oldest sample at %q: err=%v", sample.ObservedAt, err)
		}
	}
}
