package pricing_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/pricing"
)

const revenueLocalResponse = `{"code":0,"data":{"total_account_cost":8,"total_actual_cost":10}}`
const revenueUpstreamResponse = `{"code":0,"data":{"total_actual_cost":6}}`

type revenueTargets struct {
	settings configstore.TargetSettings
	auth     map[string]configstore.AuthRecord
	readAuth func(context.Context, string) (*configstore.AuthRecord, error)
}

func (targets revenueTargets) TargetSettings(context.Context) (configstore.TargetSettings, error) {
	return targets.settings, nil
}

func (targets revenueTargets) AuthRecord(ctx context.Context, host string) (*configstore.AuthRecord, error) {
	if targets.readAuth != nil {
		return targets.readAuth(ctx, host)
	}
	record, found := targets.auth[host]
	if !found {
		return nil, errors.New("isolated authorization missing")
	}
	return &record, nil
}

func revenueReadFixture(t testing.TB, managementURL, upstreamURL string, hosts, keysPerHost int) (*business.Store, revenueTargets) {
	t.Helper()
	for _, endpoint := range []string{managementURL, upstreamURL} {
		parsed, err := url.Parse(endpoint)
		if err != nil || parsed.Scheme != "http" || !net.ParseIP(parsed.Hostname()).IsLoopback() {
			t.Fatalf("revenue fixture requires explicit isolated loopback endpoints: %q", endpoint)
		}
	}
	path := filepath.Join(t.TempDir(), "revenue-reads.db")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	targets := revenueTargets{
		settings: configstore.TargetSettings{BaseURL: managementURL, AdminKey: "isolated-test-key", TimeoutSeconds: 5},
		auth:     make(map[string]configstore.AuthRecord),
	}
	token := "isolated-upstream-token"
	for hostIndex := 0; hostIndex < hosts; hostIndex++ {
		host := fmt.Sprintf("revenue-%d.test", hostIndex)
		targets.auth[host] = configstore.AuthRecord{
			Host: host, BaseURL: upstreamURL, UpstreamType: "sub2api", AuthMode: "sub2api_user_token", AccessToken: &token,
		}
		if _, err := tx.Exec(`INSERT INTO upstreams(host,base_url,upstream_type,auth_status,updated_at) VALUES(?,?,'sub2api','ready','now')`, host, upstreamURL); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(`INSERT INTO recharge_rates(host,recharge_rate,updated_at) VALUES(?,'2','now')`, host); err != nil {
			t.Fatal(err)
		}
		for key := 0; key < keysPerHost; key++ {
			id := strconv.Itoa(hostIndex*keysPerHost + key + 1)
			if _, err := tx.Exec(`INSERT INTO accounts(id,name,updated_at) VALUES(?,?,'now')`, id, "account-"+id); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(`INSERT INTO account_groups(account_id,group_name,group_id) VALUES(?,'standard','8')`, id); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(`INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,updated_at)
				VALUES(?,?,?,?,'standard','now')`, id, host, id, "key-"+id); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return store, targets
}

func TestRevenueReadsStartUpstreamBeforeLocalReadCompletes(t *testing.T) {
	upstreamStarted := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/usage/stats" || r.URL.Query().Get("api_key_id") != "1" {
			t.Errorf("unexpected upstream request: %s", r.URL)
		}
		close(upstreamStarted)
		_, _ = io.WriteString(w, revenueUpstreamResponse)
	}))
	t.Cleanup(upstream.Close)
	management := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/admin/usage/stats" || r.URL.Query().Get("account_id") != "1" ||
			r.URL.Query().Get("timezone") != "Asia/Shanghai" || r.URL.Query().Get("start_date") != "2020-01-01" {
			t.Errorf("unexpected local request: %s", r.URL)
		}
		select {
		case <-upstreamStarted:
			_, _ = io.WriteString(w, revenueLocalResponse)
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(management.Close)
	store, targets := revenueReadFixture(t, management.URL, upstream.URL, 1, 1)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	report, err := pricing.New(store, targets, nil).CalculateRevenue(ctx, pricing.RevenueRequest{Date: "2020-01-01"}, "test")

	if err != nil {
		t.Fatalf("independent reads did not complete: %v", err)
	}
	if report.Comparable != 1 || report.Abnormal != 1 || len(report.Issues) != 0 || len(report.Rows) != 1 ||
		report.Rows[0].Revenue == nil || *report.Rows[0].Revenue != "7.000000" {
		t.Fatalf("overlapping reads changed exact report: %+v", report)
	}
}

type revenueLeaseObserver struct {
	*business.Store
	released chan struct{}
}

func (store revenueLeaseObserver) ReleaseMutationLease(ctx context.Context, owner string, resources []string) error {
	err := store.Store.ReleaseMutationLease(ctx, owner, resources)
	close(store.released)
	return err
}

func TestRevenueReadsWhenCancelledWaitForUpstreamReaderBeforeReleasingTarget(t *testing.T) {
	localStarted := make(chan struct{})
	management := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(localStarted)
		<-r.Context().Done()
	}))
	t.Cleanup(management.Close)
	store, targets := revenueReadFixture(t, management.URL, "http://127.0.0.1:1", 1, 1)
	authStarted, authCancelled, authRelease := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	releaseAuth := func() { releaseOnce.Do(func() { close(authRelease) }) }
	t.Cleanup(releaseAuth)
	targets.readAuth = func(ctx context.Context, _ string) (*configstore.AuthRecord, error) {
		close(authStarted)
		<-ctx.Done()
		close(authCancelled)
		<-authRelease
		return nil, ctx.Err()
	}
	observed := revenueLeaseObserver{Store: store, released: make(chan struct{})}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		_, err := pricing.New(observed, targets, nil).CalculateRevenue(ctx, pricing.RevenueRequest{Date: "2020-01-01"}, "test")
		done <- err
	}()
	t.Cleanup(func() {
		cancel()
		releaseAuth()
		select {
		case <-finished:
		case <-time.After(3 * time.Second):
			t.Error("cancelled revenue calculation did not exit")
		}
	})
	for _, started := range []chan struct{}{localStarted, authStarted} {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("both reads must start before cancellation")
		}
	}
	cancel()
	select {
	case <-authCancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream reader did not receive cancellation")
	}
	select {
	case <-observed.released:
		t.Error("target released while an upstream reader still used its context")
	default:
	}
	releaseAuth()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled reads returned a report instead of cancellation: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled readers did not terminate")
	}
	select {
	case <-observed.released:
	default:
		t.Fatal("completed calculation retained target lease")
	}
}

func TestRevenueReadsWhenSomeAmountsFailPreserveSuccessfulKeyAndSummaries(t *testing.T) {
	management := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("account_id") == "3" {
			_, _ = io.WriteString(w, `{"code":0,"data":{"total_account_cost":"invalid","total_actual_cost":10}}`)
			return
		}
		_, _ = io.WriteString(w, revenueLocalResponse)
	}))
	t.Cleanup(management.Close)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("api_key_id") == "2" {
			_, _ = io.WriteString(w, `{"code":0,"data":{"total_actual_cost":"invalid"}}`)
			return
		}
		_, _ = io.WriteString(w, revenueUpstreamResponse)
	}))
	t.Cleanup(upstream.Close)
	store, targets := revenueReadFixture(t, management.URL, upstream.URL, 1, 3)

	report, err := pricing.New(store, targets, nil).CalculateRevenue(t.Context(), pricing.RevenueRequest{Date: "2020-01-01"}, "test")

	if err != nil {
		t.Fatal(err)
	}
	if report.Comparable != 1 || report.Unavailable != 2 || len(report.Issues) != 1 || len(report.Summaries) != 2 ||
		report.Summaries[1].Accounts != 1 || report.Summaries[1].Revenue != "7.000000" {
		t.Fatalf("partial failures changed successful results: %+v", report)
	}
	if report.Rows[0].AccountID != "1" || report.Rows[1].AccountID != "2" || report.Rows[2].AccountID != "3" ||
		report.Rows[1].UpstreamCost != nil || !strings.Contains(report.Rows[2].Note, "本地计费读取失败") {
		t.Fatalf("failed amounts fabricated costs or changed row order: %+v", report.Rows)
	}
}

func TestRevenueReadsWithEmptyCatalogDoNotRequestAmounts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("empty catalog requested amounts: %s", r.URL)
		http.Error(w, "unexpected request", http.StatusBadRequest)
	}))
	t.Cleanup(server.Close)
	store, targets := revenueReadFixture(t, server.URL, server.URL, 0, 0)
	report, err := pricing.New(store, targets, nil).CalculateRevenue(t.Context(), pricing.RevenueRequest{Date: "2020-01-01"}, "test")
	if err != nil || len(report.Rows) != 0 || report.Comparable != 0 || report.Unavailable != 0 || len(report.Summaries) != 1 || report.Summaries[0].Accounts != 0 {
		t.Fatalf("empty catalog report=%+v err=%v", report, err)
	}
}

func TestRevenueReadsWithMoreAccountsThanWorkersPreserveAllRowsAndConcurrencyLimits(t *testing.T) {
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseReaders := func() { releaseOnce.Do(func() { close(release) }) }
	localStarted, upstreamStarted := make(chan struct{}, 30), make(chan struct{}, 30)
	var mu sync.Mutex
	localActive, localMax, upstreamActive, upstreamMax, hostMax, keyMax := 0, 0, 0, 0, 0, 0
	activeByHost := make(map[int]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		local := r.URL.Path == "/api/v1/admin/usage/stats"
		keyID, _ := strconv.Atoi(r.URL.Query().Get("api_key_id"))
		host := (keyID - 1) / 5
		mu.Lock()
		if local {
			localActive++
			localMax = max(localMax, localActive)
		} else {
			upstreamActive++
			upstreamMax = max(upstreamMax, upstreamActive)
			activeByHost[host]++
			keyMax = max(keyMax, activeByHost[host])
			hostMax = max(hostMax, len(activeByHost))
		}
		mu.Unlock()
		finish := func() {
			mu.Lock()
			defer mu.Unlock()
			if local {
				localActive--
			} else {
				upstreamActive--
				activeByHost[host]--
				if activeByHost[host] == 0 {
					delete(activeByHost, host)
				}
			}
		}
		started := upstreamStarted
		response := revenueUpstreamResponse
		if local {
			started, response = localStarted, revenueLocalResponse
		}
		started <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
		}
		finish()
		_, _ = io.WriteString(w, response)
	}))
	t.Cleanup(server.Close)
	t.Cleanup(releaseReaders)
	store, targets := revenueReadFixture(t, server.URL, server.URL, 6, 5)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	type outcome struct {
		report pricing.RevenueReport
		err    error
	}
	done := make(chan outcome, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		report, err := pricing.New(store, targets, nil).CalculateRevenue(ctx, pricing.RevenueRequest{Date: "2020-01-01"}, "test")
		done <- outcome{report: report, err: err}
	}()
	t.Cleanup(func() {
		cancel()
		releaseReaders()
		select {
		case <-finished:
		case <-time.After(3 * time.Second):
			t.Error("bounded revenue calculation did not exit")
		}
	})
	for _, stage := range []struct {
		started chan struct{}
		workers int
	}{{localStarted, 8}, {upstreamStarted, 16}} {
		for index := 0; index < stage.workers; index++ {
			select {
			case <-stage.started:
			case <-ctx.Done():
				t.Fatal("independent readers did not fill their bounded worker pools")
			}
		}
	}
	releaseReaders()
	select {
	case result := <-done:
		if result.err != nil || result.report.Comparable != 30 || result.report.Unavailable != 0 || len(result.report.Rows) != 30 || len(result.report.Issues) != 0 {
			t.Fatalf("bounded readers lost results: %+v err=%v", result.report, result.err)
		}
	case <-ctx.Done():
		t.Fatal("bounded result buffers did not drain")
	}
	mu.Lock()
	defer mu.Unlock()
	if localMax != 8 || upstreamMax != 16 || hostMax != 4 || keyMax != 4 {
		t.Fatalf("reader concurrency local=%d upstream=%d hosts=%d keys/host=%d", localMax, upstreamMax, hostMax, keyMax)
	}
}
