package api_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/evidence"
)

// Keep all production database reads, but control when a concurrent collector
// commits new evidence between the result read and health calculation.
type interleavedHealthStore struct {
	*business.Store
	readKind string
	insert   func()
	once     sync.Once
}

func (store *interleavedHealthStore) afterRead(kind string) {
	if store.readKind == kind {
		store.once.Do(store.insert)
	}
}

func (store *interleavedHealthStore) Accounts(ctx context.Context) ([]business.AccountStatus, error) {
	rows, err := store.Store.Accounts(ctx)
	if err == nil {
		store.afterRead("list")
	}
	return rows, err
}

func (store *interleavedHealthStore) Account(ctx context.Context, id string) (*business.AccountDetail, error) {
	row, err := store.Store.Account(ctx, id)
	if err == nil {
		store.afterRead("detail")
	}
	return row, err
}

func (store *interleavedHealthStore) RecentAccountResults(ctx context.Context, id string, limit int) ([]business.AccountRecentResult, error) {
	rows, err := store.Store.RecentAccountResults(ctx, id, limit)
	if err == nil {
		store.afterRead("stream")
	}
	return rows, err
}

func TestAccountReadsKeepScoresAndColorsConsistentWhenEvidenceArrivesMidRead(t *testing.T) {
	for _, test := range []struct{ kind, endpoint string }{
		{"list", "/api/accounts"}, {"detail", "/api/accounts/41"},
	} {
		t.Run(test.kind, func(t *testing.T) {
			fixture := newAccountHealthFixture(t, nil)
			fixture.addSample(t, "success-old", 200, 2*time.Second)
			fixture.addSample(t, "success-new", 200, time.Second)
			store := &interleavedHealthStore{
				Store: fixture.store, readKind: test.kind,
				insert: func() { fixture.addSample(t, "concurrent-failure", 503, 0) },
			}
			router := api.New(config.Config{AdminToken: "isolated-health-token"}, fixture.private, store, api.Dependencies{})
			started := time.Now().UTC()
			account := readAccountHealth(t, router, test.endpoint)
			assertHealthEvidence(t, account, 100, 100, 100, 2, fixture.now.Add(-time.Second), started)
			if len(account.RecentResults) != 2 {
				t.Fatalf("old snapshot returned %d results, want 2", len(account.RecentResults))
			}
			// The next request must see the evidence committed during the prior read.
			started = time.Now().UTC()
			account = readAccountHealth(t, router, test.endpoint)
			assertHealthEvidence(t, account, 66.25, 62.5, 75, 3, fixture.now, started)
			if len(account.RecentResults) != 3 {
				t.Fatalf("next snapshot returned %d results, want 3", len(account.RecentResults))
			}
			fixture.assertSchedulingUnchanged(t)
		})
	}
}

func TestAccountHealthStreamKeepsScoresAndColorsConsistentWhenEvidenceArrivesMidRead(t *testing.T) {
	live := &healthSnapshotLive{updates: make(chan evidence.LiveUpdate, 1), stopped: make(chan struct{})}
	fixture := newAccountHealthFixture(t, live)
	fixture.addSample(t, "success-old", 200, 2*time.Second)
	fixture.addSample(t, "success-new", 200, time.Second)
	store := &interleavedHealthStore{
		Store: fixture.store, readKind: "stream",
		insert: func() { fixture.addSample(t, "concurrent-failure", 503, 0) },
	}
	router := api.New(config.Config{AdminToken: "isolated-health-token"}, fixture.private, store, api.Dependencies{AccountResultsLive: live})
	server := httptest.NewServer(router)
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/accounts/results/events?account_ids=41", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer isolated-health-token")
	started := time.Now().UTC()
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("stream status=%d", response.StatusCode)
	}
	scanner := bufio.NewScanner(response.Body)
	for index := range 2 {
		kind, data := readHealthStreamEvent(t, scanner)
		if kind != "snapshot" {
			t.Fatalf("event=%s, want atomic snapshot", kind)
		}
		var snapshot struct {
			Health  accountHealthResponse          `json:"health"`
			Results []business.AccountRecentResult `json:"results"`
		}
		if err := json.Unmarshal([]byte(data), &snapshot); err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			assertHealthEvidence(t, snapshot.Health, 100, 100, 100, 2, fixture.now.Add(-time.Second), started)
			if len(snapshot.Results) != 2 {
				t.Fatalf("initial snapshot returned %d results, want 2", len(snapshot.Results))
			}
			started = time.Now().UTC()
			live.updates <- evidence.LiveUpdate{AccountID: "41"}
		} else {
			assertHealthEvidence(t, snapshot.Health, 66.25, 62.5, 75, 3, fixture.now, started)
			if len(snapshot.Results) != 3 {
				t.Fatalf("next snapshot returned %d results, want 3", len(snapshot.Results))
			}
		}
	}
	fixture.assertSchedulingUnchanged(t)
}
