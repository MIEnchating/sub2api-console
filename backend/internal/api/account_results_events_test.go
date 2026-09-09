package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/evidence"
)

type liveResultFixture struct {
	fakeBusiness
	mu      sync.Mutex
	results []business.AccountRecentResult
}

func (f *liveResultFixture) RecentAccountResults(context.Context, string, int) ([]business.AccountRecentResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]business.AccountRecentResult{}, f.results...), nil
}

type liveStreamFixture struct {
	updates chan evidence.LiveUpdate
	stopped chan struct{}
}

func (f *liveStreamFixture) Subscribe([]string) (<-chan evidence.LiveUpdate, func(), error) {
	return f.updates, func() { close(f.stopped) }, nil
}
func TestAccountResultsEventsRequireAuthenticationAndStableBoundedIDs(t *testing.T) {
	router, _ := testRouter(t, config.Config{AdminToken: "test-token"}, fakeBusiness{})
	if response := request(t, router, http.MethodGet, "/api/accounts/results/events?account_ids=41", nil, ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d", response.Code)
	}
	for _, ids := range []string{"", "name", "0", "41,", strings.Repeat("41,", 100) + "42"} {
		response := authenticatedRequest(t, router, http.MethodGet, "/api/accounts/results/events?account_ids="+ids, nil)
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("invalid IDs %q status=%d", ids, response.Code)
		}
	}
}
func TestAccountResultsEventsSendNewAndEnrichedRecordsWithoutRepeatingSnapshot(t *testing.T) {
	passed := "通过"
	repository := &liveResultFixture{fakeBusiness: fakeBusiness{accountDetail: &business.AccountDetail{}}, results: []business.AccountRecentResult{
		{ID: "1", Result: &passed, Source: "traffic"},
	}}
	live := &liveStreamFixture{updates: make(chan evidence.LiveUpdate, 2), stopped: make(chan struct{})}
	router, _ := testRouterWithDependencies(t, config.Config{AdminToken: "test-token"}, repository, Dependencies{AccountResultsLive: live})
	server := httptest.NewServer(router)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/accounts/results/events?account_ids=41", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer test-token")
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", response.StatusCode)
	}
	scanner := bufio.NewScanner(response.Body)
	read := func() (string, string) {
		t.Helper()
		var event, data string
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				return event, data
			}
			if strings.HasPrefix(line, "event: ") {
				event = strings.TrimPrefix(line, "event: ")
			}
			if strings.HasPrefix(line, "data: ") {
				data = strings.TrimPrefix(line, "data: ")
			}
		}
		t.Fatalf("stream ended: %v", scanner.Err())
		return "", ""
	}
	kind, _ := read()
	if kind != "snapshot" {
		t.Fatalf("initial event=%s", kind)
	}
	repository.mu.Lock()
	repository.results = append([]business.AccountRecentResult{{ID: "2", Result: &passed, Source: "traffic"}}, repository.results...)
	repository.mu.Unlock()
	live.updates <- evidence.LiveUpdate{AccountID: "41"}
	kind, data := read()
	var result accountResultEvent
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		t.Fatal(err)
	}
	if kind != "result" || result.Result.ID != "2" || result.Result.Score == nil || *result.Result.Score != 100 {
		t.Fatalf("new result=%s %s", kind, data)
	}
	if kind, _ := read(); kind != "collection" {
		t.Fatalf("unchanged result repeated: %s", kind)
	}
	repository.mu.Lock()
	value := 321.0
	repository.results[0].LatencyMS = &value
	repository.mu.Unlock()
	live.updates <- evidence.LiveUpdate{AccountID: "41"}
	kind, data = read()
	if !strings.Contains(data, `"latency_ms":321`) || kind != "result" {
		t.Fatalf("enrichment not pushed: %s %s", kind, data)
	}
	response.Body.Close()
	cancel()
	select {
	case <-live.stopped:
	case <-time.After(time.Second):
		t.Fatal("stream did not unsubscribe")
	}
}
