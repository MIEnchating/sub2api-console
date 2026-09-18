package api_test

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/evidence"
)

type accountHealthResponse struct {
	HealthScore       *float64                       `json:"health_score"`
	ShortScore        *float64                       `json:"short_score"`
	LongScore         *float64                       `json:"long_score"`
	SampleCount       int64                          `json:"sample_count"`
	HealthEvaluatedAt string                         `json:"health_evaluated_at"`
	HealthEvidenceAt  *string                        `json:"health_evidence_at"`
	RecentResults     []business.AccountRecentResult `json:"recent_results"`
}

type healthSnapshotLive struct {
	updates chan evidence.LiveUpdate
	stopped chan struct{}
}

func (live *healthSnapshotLive) Subscribe([]string) (<-chan evidence.LiveUpdate, func(), error) {
	return live.updates, func() { close(live.stopped) }, nil
}

type accountHealthFixture struct {
	store   *business.Store
	private *configstore.Store
	db      *sql.DB
	router  http.Handler
	now     time.Time
}

func newAccountHealthFixture(t *testing.T, live api.AccountResultsLive) accountHealthFixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "health-snapshot.sqlite3")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.ExecContext(t.Context(), `INSERT INTO accounts(id,name,multiplier,schedulable,priority,
		concurrency,target_priority,target_load_factor,target_schedulable,target_concurrency,updated_at)
		VALUES('41','health-snapshot','1',1,20,5,21,'0.8',1,4,'2026-09-17T06:00:00Z');
		INSERT INTO account_groups(account_id,group_name) VALUES('41','codex');
		INSERT INTO account_health_evaluations(account_id,group_name,health_score,short_score,long_score,sample_count,evaluated_at)
		VALUES('41','codex',99,99,99,60,'2026-09-17T06:00:00Z');
		INSERT INTO routing_decisions(account_id,group_name,priority,schedulable,role,routing_state,updated_at,payload_json)
		VALUES('41','codex',20,1,'primary','healthy','2026-09-17T06:00:00Z','{"weight":0.9}')`)
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"account_health_evaluations", "routing_decisions", "accounts"} {
		for _, operation := range []string{"INSERT", "UPDATE", "DELETE"} {
			_, err := db.ExecContext(t.Context(), fmt.Sprintf(`CREATE TRIGGER reject_health_read_%s_%s
				BEFORE %s ON %s BEGIN SELECT RAISE(ABORT,'health read changed scheduling state'); END`, table, operation, operation, table))
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	private, err := configstore.Open(filepath.Join(t.TempDir(), "config.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = private.Close() })
	return accountHealthFixture{
		store: store, private: private, db: db, now: time.Now().UTC().Add(-time.Second),
		router: api.New(config.Config{AdminToken: "isolated-health-token"}, private, store, api.Dependencies{AccountResultsLive: live}),
	}
}

func (fixture accountHealthFixture) addSample(t *testing.T, id string, status int, age time.Duration) {
	t.Helper()
	result := "passed"
	if status >= 400 {
		result = "failed"
	}
	_, err := fixture.store.PersistTrafficSamples(t.Context(), []business.TrafficSample{{
		AccountID: "41", GroupName: "codex", EvidenceKey: id, Result: result,
		ObservedAt: fixture.now.Add(-age).Format(time.RFC3339Nano),
		Payload:    map[string]any{"status_code": status, "input_tokens": 1, "output_tokens": 1},
	}})
	if err != nil {
		t.Fatal(err)
	}
}

func (fixture accountHealthFixture) assertSchedulingUnchanged(t *testing.T) {
	t.Helper()
	var unchanged bool
	err := fixture.db.QueryRowContext(t.Context(), `SELECT
		(SELECT health_score=99 AND short_score=99 AND long_score=99 AND sample_count=60
		 AND evaluated_at='2026-09-17T06:00:00Z' FROM account_health_evaluations WHERE account_id='41')
		AND (SELECT priority=20 AND schedulable=1 AND role='primary' AND routing_state='healthy'
		 AND updated_at='2026-09-17T06:00:00Z' AND payload_json='{"weight":0.9}' FROM routing_decisions WHERE account_id='41')
		AND (SELECT priority=20 AND schedulable=1 AND concurrency=5 AND target_priority=21
		 AND target_load_factor='0.8' AND target_schedulable=1 AND target_concurrency=4 FROM accounts WHERE id='41')`).Scan(&unchanged)
	if err != nil || !unchanged {
		t.Fatalf("health read changed persisted evaluations, decisions or targets: unchanged=%t error=%v", unchanged, err)
	}
}

func TestAccountReadsReplaceOldHealthWithCurrentEvidenceWithoutSchedulingWrites(t *testing.T) {
	for _, endpoint := range []string{"/api/accounts", "/api/accounts/41"} {
		t.Run(endpoint, func(t *testing.T) {
			fixture := newAccountHealthFixture(t, nil)
			fixture.addSample(t, "success-old", 200, 2*time.Second)
			fixture.addSample(t, "success-new", 200, time.Second)
			fixture.addSample(t, "capacity-failure", 503, 0)
			started := time.Now().UTC()
			account := readAccountHealth(t, fixture.router, endpoint)

			assertHealthEvidence(t, account, 66.25, 62.5, 75, 3, fixture.now, started)
			if len(account.RecentResults) != 3 || account.RecentResults[0].EventType == nil || *account.RecentResults[0].EventType != "gateway_error" {
				t.Fatalf("response omitted the evidence behind its score: %+v", account.RecentResults)
			}
			fixture.assertSchedulingUnchanged(t)
		})
	}
}

func TestAccountReadsClearOldHealthWhenCurrentEvidenceIsMissing(t *testing.T) {
	for _, endpoint := range []string{"/api/accounts", "/api/accounts/41"} {
		t.Run(endpoint, func(t *testing.T) {
			fixture := newAccountHealthFixture(t, nil)
			account := readAccountHealth(t, fixture.router, endpoint)

			if account.HealthScore != nil || account.ShortScore != nil || account.LongScore != nil || account.SampleCount != 0 || account.HealthEvidenceAt != nil {
				t.Fatalf("missing evidence retained an old health score: %+v", account)
			}
			if _, err := time.Parse(time.RFC3339Nano, account.HealthEvaluatedAt); err != nil {
				t.Fatalf("missing current evaluation time: %q", account.HealthEvaluatedAt)
			}
			fixture.assertSchedulingUnchanged(t)
		})
	}
}

func TestAccountHealthStreamPublishesNewScoresAndResultsInOneSnapshot(t *testing.T) {
	live := &healthSnapshotLive{updates: make(chan evidence.LiveUpdate, 1), stopped: make(chan struct{})}
	fixture := newAccountHealthFixture(t, live)
	fixture.addSample(t, "success-old", 200, 2*time.Second)
	fixture.addSample(t, "success-new", 200, time.Second)
	server := httptest.NewServer(fixture.router)
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
	readSnapshot := func(score, short, long float64, count int64, observedAt time.Time) {
		t.Helper()
		kind, data := readHealthStreamEvent(t, scanner)
		var snapshot struct {
			AccountID string                         `json:"account_id"`
			Results   []business.AccountRecentResult `json:"results"`
			Health    *accountHealthResponse         `json:"health"`
		}
		if err := json.Unmarshal([]byte(data), &snapshot); err != nil {
			t.Fatal(err)
		}
		if kind != "snapshot" || snapshot.AccountID != "41" || snapshot.Health == nil {
			t.Errorf("health and results must arrive atomically in snapshot: event=%s data=%s", kind, data)
			return
		}
		assertHealthEvidence(t, *snapshot.Health, score, short, long, count, observedAt, started)
		if len(snapshot.Results) != int(count) || snapshot.Results[0].ObservedAt == nil {
			t.Fatalf("snapshot omitted results for its score: %+v", snapshot)
		}
		latest, err := time.Parse(time.RFC3339Nano, *snapshot.Results[0].ObservedAt)
		if err != nil || !latest.Equal(observedAt) {
			t.Fatalf("snapshot health and newest result refer to different evidence: %+v", snapshot)
		}
	}

	readSnapshot(100, 100, 100, 2, fixture.now.Add(-time.Second))
	fixture.addSample(t, "capacity-failure", 503, 0)
	live.updates <- evidence.LiveUpdate{AccountID: "41"}
	readSnapshot(66.25, 62.5, 75, 3, fixture.now)
	if kind, _ := readHealthStreamEvent(t, scanner); kind != "collection" {
		t.Fatalf("unexpected intermediate result event: %s", kind)
	}
	fixture.assertSchedulingUnchanged(t)
	_ = response.Body.Close()
	cancel()
	select {
	case <-live.stopped:
	case <-time.After(time.Second):
		t.Fatal("stream did not unsubscribe")
	}
}

func readAccountHealth(t *testing.T, router http.Handler, endpoint string) accountHealthResponse {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, endpoint, nil)
	request.Header.Set("Authorization", "Bearer isolated-health-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var account accountHealthResponse
	if endpoint == "/api/accounts" {
		var accounts []accountHealthResponse
		if err := json.Unmarshal(response.Body.Bytes(), &accounts); err != nil {
			t.Fatal(err)
		}
		if len(accounts) != 1 {
			t.Fatalf("accounts=%d, want one fixture", len(accounts))
		}
		return accounts[0]
	}
	if err := json.Unmarshal(response.Body.Bytes(), &account); err != nil {
		t.Fatal(err)
	}
	return account
}

func assertHealthEvidence(t *testing.T, health accountHealthResponse, score, short, long float64, count int64, observedAt, started time.Time) {
	t.Helper()
	if health.HealthScore == nil || *health.HealthScore != score || health.ShortScore == nil || *health.ShortScore != short || health.LongScore == nil || *health.LongScore != long || health.SampleCount != count {
		t.Fatalf("incorrect current health: %+v; want score=%g short=%g long=%g count=%d", health, score, short, long, count)
	}
	if health.HealthEvidenceAt == nil {
		t.Fatal("health projection omitted its evidence time")
	}
	actual, err := time.Parse(time.RFC3339Nano, *health.HealthEvidenceAt)
	if err != nil || !actual.Equal(observedAt) {
		t.Fatalf("health evidence time=%v, want %s", health.HealthEvidenceAt, observedAt)
	}
	evaluatedAt, err := time.Parse(time.RFC3339Nano, health.HealthEvaluatedAt)
	if err != nil || evaluatedAt.Before(started) || evaluatedAt.After(time.Now().UTC()) {
		t.Fatalf("health evaluation time is not current: %q", health.HealthEvaluatedAt)
	}
}

func readHealthStreamEvent(t *testing.T, scanner *bufio.Scanner) (string, string) {
	t.Helper()
	var kind, data string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			return kind, data
		}
		if strings.HasPrefix(line, "event: ") {
			kind = strings.TrimPrefix(line, "event: ")
		}
		if strings.HasPrefix(line, "data: ") {
			data = strings.TrimPrefix(line, "data: ")
		}
	}
	t.Fatalf("stream ended before the expected health snapshot: %v", scanner.Err())
	return "", ""
}
