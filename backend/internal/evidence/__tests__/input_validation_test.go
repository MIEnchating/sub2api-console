package evidence_test

import (
	"context"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/evidence"
)

type validationEvidenceRepository struct {
	samples []business.TrafficSample
	fetched []string
}

func (r *validationEvidenceRepository) EvidenceTargets(context.Context, *string, *string) ([]business.EvidenceTarget, error) {
	return []business.EvidenceTarget{{AccountID: "41", GroupName: "codex"}}, nil
}

func (r *validationEvidenceRepository) PersistTrafficSamples(_ context.Context, samples []business.TrafficSample) (int, error) {
	r.samples = append(r.samples, samples...)
	return len(samples), nil
}

func (r *validationEvidenceRepository) PersistTrafficFetches(_ context.Context, ids []string, _ time.Time) error {
	r.fetched = append(r.fetched, ids...)
	return nil
}

type validationTrafficAdmin struct {
	row map[string]any
}

func (a validationTrafficAdmin) RequestDetails(context.Context, string, int, int) ([]map[string]any, error) {
	return []map[string]any{a.row}, nil
}

func TestInvalidTrafficLatencyCannotBecomeHealthEvidence(t *testing.T) {
	for _, field := range []string{"duration_ms", "first_token_ms"} {
		for _, value := range []string{"NaN", "+Inf", "-Inf", "-1"} {
			t.Run(field+"="+value, func(t *testing.T) {
				now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
				repository := &validationEvidenceRepository{}
				row := map[string]any{"account_id": "41", "request_id": "invalid-latency", "kind": "success", "created_at": now.Format(time.RFC3339Nano), field: value}
				result, err := evidence.New(repository, nil).Collect(context.Background(), map[string]any{
					"probe": map[string]any{"enabled": false}, "recovery": map[string]any{"enabled": false},
				}, validationTrafficAdmin{row: row}, evidence.Options{FetchTraffic: true, Now: now})
				if err != nil {
					t.Fatal(err)
				}
				if result.MalformedRows != 1 || len(repository.samples) != 0 {
					t.Fatalf("invalid latency persisted: result=%+v samples=%+v", result, repository.samples)
				}
			})
		}
	}
}

func TestNonStringManagedGroupModeCannotBroadenCollection(t *testing.T) {
	for _, value := range []any{true, 7, []any{"selected"}} {
		repository := &validationEvidenceRepository{}
		_, err := evidence.New(repository, nil).Plan(context.Background(), map[string]any{
			"scope": map[string]any{"managed_group_mode": value, "managed_group_ids": []any{}},
		}, nil, nil, time.Now().UTC())
		if err == nil {
			t.Fatalf("invalid scope mode %v accepted as all groups", value)
		}
	}
}
