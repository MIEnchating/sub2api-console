package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/evidence"
)

func TestAccountStreamStartsWithoutDisplayStabilityHistory(t *testing.T) {
	live := &healthSnapshotLive{updates: make(chan evidence.LiveUpdate), stopped: make(chan struct{})}
	close(live.updates)
	f := newAccountHealthFixture(t, live)
	if _, err := f.db.Exec(`DROP TABLE account_stability_samples`); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/accounts/results/events?account_ids=41", nil)
	request.Header.Set("Authorization", "Bearer isolated-health-token")
	response := httptest.NewRecorder()
	f.router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "event: snapshot") {
		t.Fatalf("stream=%d %s", response.Code, response.Body.String())
	}
	select {
	case <-live.stopped:
	default:
		t.Fatal("stream did not release subscription")
	}
}

func TestAccountStreamRejectsSelectionContainingMissingAccount(t *testing.T) {
	live := &healthSnapshotLive{updates: make(chan evidence.LiveUpdate), stopped: make(chan struct{})}
	close(live.updates)
	f := newAccountHealthFixture(t, live)
	request := httptest.NewRequest(http.MethodGet, "/api/accounts/results/events?account_ids=41,999", nil)
	request.Header.Set("Authorization", "Bearer isolated-health-token")
	response := httptest.NewRecorder()
	f.router.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("stream=%d %s", response.Code, response.Body.String())
	}
}
