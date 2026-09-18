package evidence_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/evidence"
)

func TestBoundedProbeBatchLimitsAccountsAndReportsDeferredWork(t *testing.T) {
	fixture := newProbeBatchFixture(t, []string{"1", "2", "3", "4", "5"}, nil)
	result, err := fixture.service.Collect(context.Background(), fixture.policy, nil, evidence.Options{
		ProbesAllowed: true, ProbeBatchSize: 2, Now: fixture.now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ProbesPersisted != 2 || result.ProbesDeferred != 3 || !slices.Equal(fixture.probeIDs(), []string{"1", "2"}) {
		t.Fatalf("bounded batch result=%+v probed=%v", result, fixture.probeIDs())
	}
}

func TestBoundedProbeBatchRetainsAllConfiguredModelsOfSelectedAccounts(t *testing.T) {
	fixture := newProbeBatchFixture(t, []string{"1", "2", "3"}, nil)
	if err := fixture.store.SetAccountTestModels(context.Background(), "1", []string{"model-a", "model-b"}, "test"); err != nil {
		t.Fatal(err)
	}
	result, err := fixture.service.Collect(context.Background(), fixture.policy, nil, evidence.Options{
		ProbesAllowed: true, ProbeBatchSize: 1, Now: fixture.now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ProbesPersisted != 2 || result.ProbesDeferred != 2 || !slices.Equal(fixture.probeIDs(), []string{"1", "1"}) {
		t.Fatalf("configured models were lost or batch exceeded: result=%+v probed=%v", result, fixture.probeIDs())
	}
}

func TestBoundedProbeBatchPrioritizesNeverProbedAccountsWithStableIDOrder(t *testing.T) {
	fixture := newProbeBatchFixture(t, []string{"1", "10", "2"}, nil)
	fixture.seedProbe(t, "1", fixture.now.Add(-24*time.Hour))
	_, err := fixture.service.Collect(context.Background(), fixture.policy, nil, evidence.Options{
		ProbesAllowed: true, ProbeBatchSize: 1, Now: fixture.now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(fixture.probeIDs(), []string{"2"}) {
		t.Fatalf("never-probed accounts should use numeric ID order: probed=%v", fixture.probeIDs())
	}
}

func TestBoundedProbeBatchPrioritizesOldestValidProbeEvidence(t *testing.T) {
	fixture := newProbeBatchFixture(t, []string{"1", "2", "3"}, nil)
	fixture.seedProbe(t, "1", fixture.now.Add(-time.Hour))
	fixture.seedProbe(t, "2", fixture.now.Add(-3*time.Hour))
	fixture.seedProbe(t, "3", fixture.now.Add(-2*time.Hour))
	_, err := fixture.service.Collect(context.Background(), fixture.policy, nil, evidence.Options{
		ProbesAllowed: true, ProbeBatchSize: 1, Now: fixture.now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(fixture.probeIDs(), []string{"2"}) {
		t.Fatalf("oldest evidence should be refreshed first: probed=%v", fixture.probeIDs())
	}
}

func TestFollowingProbeBatchServesDeferredAccountsEvenWhenFirstBatchBecomesDueAgain(t *testing.T) {
	fixture := newProbeBatchFixture(t, []string{"1", "2", "3", "4", "5"}, nil)
	for _, now := range []time.Time{fixture.now, fixture.now.Add(6 * time.Minute)} {
		_, err := fixture.service.Collect(context.Background(), fixture.policy, nil, evidence.Options{
			ProbesAllowed: true, ProbeBatchSize: 2, Now: now,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if !slices.Equal(fixture.probeIDs(), []string{"1", "2", "3", "4"}) {
		t.Fatalf("earlier IDs starved the next batch: probed=%v", fixture.probeIDs())
	}
}

func TestDeferredProbeAccountsDoNotReceiveHealthEvidence(t *testing.T) {
	fixture := newProbeBatchFixture(t, []string{"1", "2"}, nil)
	result, err := fixture.service.Collect(context.Background(), fixture.policy, nil, evidence.Options{
		ProbesAllowed: true, ProbeBatchSize: 1, Now: fixture.now,
	})
	if err != nil {
		t.Fatal(err)
	}
	accountID := "2"
	samples, err := fixture.store.RoutingSamples(context.Background(), &accountID, nil, "active-probe", 60)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 0 || len(result.SourceErrors) != 0 {
		t.Fatalf("deferred work must not create success or failure evidence: samples=%v result=%+v", samples, result)
	}
}

func TestUnlimitedProbeCollectionPreservesFullRequestedScope(t *testing.T) {
	fixture := newProbeBatchFixture(t, []string{"1", "2", "3"}, nil)
	result, err := fixture.service.Collect(context.Background(), fixture.policy, nil, evidence.Options{
		ProbesAllowed: true, Now: fixture.now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ProbesPersisted != 3 || result.ProbesDeferred != 0 || !slices.Equal(fixture.probeIDs(), []string{"1", "2", "3"}) {
		t.Fatalf("unlimited manual scope changed: result=%+v probed=%v", result, fixture.probeIDs())
	}
}

func TestFreshTrafficIsRemovedBeforeChoosingBoundedProbeBatch(t *testing.T) {
	fixture := newProbeBatchFixture(t, []string{"1", "2", "3", "4", "5"}, []string{"1", "2"})
	fixture.policy["probe"].(map[string]any)["performance_exploration_enabled"] = false
	result, err := fixture.service.Collect(context.Background(), fixture.policy, fixture.admin, evidence.Options{
		FetchTraffic: true, ProbesAllowed: true, ProbeBatchSize: 2, Now: fixture.now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ProbesPersisted != 2 || result.ProbesDeferred != 1 || result.TrafficPersisted != 2 || !slices.Equal(fixture.probeIDs(), []string{"3", "4"}) {
		t.Fatalf("fresh traffic consumed probe slots: result=%+v probed=%v", result, fixture.probeIDs())
	}
}

func TestFutureProbeEvidenceCannotKeepAnAccountAheadOfDeferredAccounts(t *testing.T) {
	fixture := newProbeBatchFixture(t, []string{"1", "2"}, nil)
	fixture.seedProbe(t, "1", fixture.now.Add(time.Hour))
	fixture.seedProbe(t, "2", fixture.now.Add(-time.Hour))
	for _, now := range []time.Time{fixture.now, fixture.now.Add(6 * time.Minute)} {
		_, err := fixture.service.Collect(context.Background(), fixture.policy, nil, evidence.Options{
			ProbesAllowed: true, ProbeBatchSize: 1, Now: now,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if !slices.Equal(fixture.probeIDs(), []string{"1", "2"}) {
		t.Fatalf("future evidence monopolized successive probe batches: probed=%v", fixture.probeIDs())
	}
}

func TestEvidenceTargetsUsesLatestValidProbeBeforeFutureEvidence(t *testing.T) {
	fixture := newProbeBatchFixture(t, []string{"1"}, nil)
	validAt := fixture.now.Add(-time.Hour)
	fixture.seedProbe(t, "1", validAt)
	fixture.seedProbe(t, "1", fixture.now.Add(time.Hour))
	targets, err := fixture.store.EvidenceTargets(context.Background(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].ProbeAt == nil || !targets[0].ProbeAt.Equal(validAt) {
		t.Fatalf("future evidence hid valid probe timestamp: targets=%+v", targets)
	}
}

func TestUnavailableProbeAccountsCannotStarveLaterBatches(t *testing.T) {
	for _, test := range []struct {
		name        string
		unavailable []string
		wantProbed  []string
	}{
		{name: "partially skipped batch", unavailable: []string{"1"}, wantProbed: []string{"2", "3"}},
		{name: "entire batch has no models", unavailable: []string{"1", "2"}, wantProbed: []string{"3"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newProbeBatchFixture(t, []string{"1", "2", "3"}, nil)
			fixture.removeModels(t, test.unavailable)
			for _, now := range []time.Time{fixture.now, fixture.now.Add(15 * time.Second)} {
				_, err := fixture.service.Collect(context.Background(), fixture.policy, nil, evidence.Options{
					ProbesAllowed: true, ProbeBatchSize: 2, Now: now,
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			if !slices.Equal(fixture.probeIDs(), test.wantProbed) {
				t.Fatalf("unavailable accounts starved later targets: probed=%v", fixture.probeIDs())
			}
			for _, accountID := range test.unavailable {
				samples, err := fixture.store.RoutingSamples(context.Background(), &accountID, nil, "active-probe", 60)
				if err != nil || len(samples) != 0 {
					t.Fatalf("selection bookkeeping created health evidence: account=%s samples=%v err=%v", accountID, samples, err)
				}
			}
		})
	}
}
