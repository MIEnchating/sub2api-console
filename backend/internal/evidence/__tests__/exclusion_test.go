package evidence_test

import (
	"context"
	"github.com/MIEnchating/sub2api-console/backend/internal/evidence"
	"testing"
	"time"
)

type excludedEvidenceRepository struct{ validationEvidenceRepository }

func (r *excludedEvidenceRepository) AccountUpstreamExcluded(context.Context, string) (bool, error) {
	return true, nil
}

type forbiddenTrafficAdmin struct{ t *testing.T }

func (a forbiddenTrafficAdmin) RequestDetails(context.Context, string, int, int) ([]map[string]any, error) {
	a.t.Error("excluded account contacted")
	return nil, nil
}
func TestExclusionAfterEvidencePlanningSkipsTrafficWithoutFailure(t *testing.T) {
	repo := &excludedEvidenceRepository{}
	result, err := evidence.New(repo, nil).Collect(t.Context(), map[string]any{"probe": map[string]any{"enabled": false}, "recovery": map[string]any{"enabled": false}}, forbiddenTrafficAdmin{t}, evidence.Options{FetchTraffic: true, Now: time.Now()})
	if err != nil || len(result.SourceErrors) != 0 || len(repo.fetched) != 0 {
		t.Fatalf("excluded collection: %+v %v fetched=%v", result, err, repo.fetched)
	}
}

func TestLiveEvidenceExcludedAccountDoesNotReadTargetOrRemote(t *testing.T) {
	repo := &excludedEvidenceRepository{}
	if err := evidence.New(repo, nil).CollectLive(t.Context(), map[string]any{}, nil, "41"); err != nil {
		t.Fatal(err)
	}
}
