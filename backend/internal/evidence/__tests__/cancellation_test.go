package evidence_test

import (
	"context"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/evidence"
)

type cancelOnTrafficWorkerCheck struct {
	context.Context
	cancel context.CancelFunc
}

func (ctx cancelOnTrafficWorkerCheck) Err() error {
	ctx.cancel()
	return ctx.Context.Err()
}

func TestCancelledDispatchedTrafficFetchCannotRecordSuccessfulFetch(t *testing.T) {
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	repository := &validationEvidenceRepository{}
	_, err := evidence.New(repository, nil).Collect(cancelOnTrafficWorkerCheck{Context: base, cancel: cancel}, map[string]any{
		"probe": map[string]any{"enabled": false}, "recovery": map[string]any{"enabled": false},
	}, validationTrafficAdmin{}, evidence.Options{FetchTraffic: true})
	if err != nil && err != context.Canceled {
		t.Fatal(err)
	}
	if len(repository.fetched) != 0 {
		t.Fatalf("cancelled traffic fetch recorded success for accounts: %q", repository.fetched)
	}
}
