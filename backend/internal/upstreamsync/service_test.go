package upstreamsync

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type syncRepository struct {
	mu         sync.Mutex
	applied    []business.UpstreamSyncWrite
	failures   []string
	hosts      []business.UpstreamHost
	failureErr error
}

func (r *syncRepository) Upstreams(context.Context) (business.UpstreamSummary, error) {
	return business.UpstreamSummary{Hosts: r.hosts}, nil
}
func (r *syncRepository) ApplyUpstreamSync(_ context.Context, value business.UpstreamSyncWrite) (business.UpstreamSyncWriteResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.applied = append(r.applied, value)
	result := business.UpstreamSyncWriteResult{Host: value.Host, BalanceStatus: "未执行", CheckedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if value.Catalog != nil {
		result.GroupCount, result.KeyCount = len(value.Catalog.Groups), len(value.Catalog.Keys)
		for _, host := range r.hosts {
			if host.Host == value.Host {
				result.AccountTotal = int(host.AccountCount)
				result.AccountRateSucceeded = min(result.AccountTotal, result.KeyCount)
				result.AccountRateFailed = result.AccountTotal - result.AccountRateSucceeded
				break
			}
		}
	}
	if value.Balance != nil {
		result.RawBalance, result.Balance, result.BalanceStatus = value.Balance.RawBalance, value.Balance.RawBalance, value.Balance.Status
	}
	return result, nil
}

func TestSyncAllNowKeepsUpstreamAndAccountRateCountsSeparate(t *testing.T) {
	token, rate := "token", "0.5"
	repository := &syncRepository{hosts: []business.UpstreamHost{
		{Host: "ready.example", AccountCount: 3},
		{Host: "missing.example", AccountCount: 5},
	}}
	private := &syncPrivate{records: map[string]configstore.AuthRecord{
		"ready.example": {Host: "ready.example", AccessToken: &token, Headers: map[string]string{}, Cookies: map[string]string{}},
	}}
	reader := &syncReader{
		catalog: func(context.Context, configstore.AuthRecord) (business.UpstreamCatalogSnapshot, error) {
			return business.UpstreamCatalogSnapshot{Keys: []business.UpstreamCatalogKey{
				{KeyID: "key-1", Rate: &rate}, {KeyID: "key-2", Rate: &rate},
			}}, nil
		},
		balance: func(context.Context, configstore.AuthRecord) (business.UpstreamBalanceObservation, error) {
			return business.UpstreamBalanceObservation{Status: "已读取"}, nil
		},
	}
	service := New(repository, private, &syncReader{catalog: reader.catalog, balance: reader.balance}, &syncRefresher{}, &memoryTasks{done: make(chan taskstore.Task, 1)})
	result, err := service.SyncAllNow(context.Background(), Scope{Catalog: true, Balance: true}, "tester")
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 2 || result.Succeeded != 1 || result.AuthFailed != 1 || result.Failed != 0 {
		t.Fatalf("upstream counts=%#v", result)
	}
	if result.AccountTotal != 8 || result.AccountRateSucceeded != 2 || result.AccountRateFailed != 6 {
		t.Fatalf("account rate counts=%#v", result)
	}
}
func (r *syncRepository) RecordUpstreamSyncFailure(_ context.Context, host, _, reason string, _ bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failures = append(r.failures, host+":"+reason)
	return r.failureErr
}
func (r *syncRepository) RecordRuntimeEvent(context.Context, string, string, string, map[string]any) (int64, error) {
	return -1, nil
}

type syncPrivate struct {
	mu      sync.Mutex
	records map[string]configstore.AuthRecord
	saved   bool
}

func (s *syncPrivate) AuthRecord(_ context.Context, host string) (*configstore.AuthRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, found := s.records[host]
	if !found {
		return nil, nil
	}
	copy := record
	return &copy, nil
}
func (s *syncPrivate) SaveAuthRecord(_ context.Context, record configstore.AuthRecord, _ map[string]bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[record.Host], s.saved = record, true
	return nil
}
func (s *syncPrivate) wasSaved() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saved
}

type syncReader struct {
	catalog func(context.Context, configstore.AuthRecord) (business.UpstreamCatalogSnapshot, error)
	balance func(context.Context, configstore.AuthRecord) (business.UpstreamBalanceObservation, error)
}

func (r *syncReader) ReadCatalog(ctx context.Context, record configstore.AuthRecord) (business.UpstreamCatalogSnapshot, error) {
	return r.catalog(ctx, record)
}
func (r *syncReader) ReadBalance(ctx context.Context, record configstore.AuthRecord) (business.UpstreamBalanceObservation, error) {
	return r.balance(ctx, record)
}

type syncRefresher struct {
	refresh func(context.Context, configstore.AuthRecord) (configstore.AuthRecord, error)
}

type syncResolver struct {
	mu      sync.Mutex
	record  *configstore.AuthRecord
	err     error
	calls   int
	private *syncPrivate
}

func (r *syncResolver) ResolveAuth(context.Context, string, string) (*configstore.AuthRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.private != nil && r.record != nil {
		r.private.SaveAuthRecord(context.Background(), *r.record, nil)
	}
	return r.record, r.err
}

func (r *syncResolver) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func (r *syncRefresher) Refresh(ctx context.Context, record configstore.AuthRecord) (configstore.AuthRecord, error) {
	return r.refresh(ctx, record)
}

type memoryTasks struct {
	mu    sync.Mutex
	tasks map[string]taskstore.Task
	done  chan taskstore.Task
}

func (s *memoryTasks) Save(_ context.Context, task taskstore.Task) error {
	s.mu.Lock()
	if s.tasks == nil {
		s.tasks = map[string]taskstore.Task{}
	}
	s.tasks[task.ID] = task
	s.mu.Unlock()
	if task.Status == "succeeded" || task.Status == "failed" {
		select {
		case s.done <- task:
		default:
		}
	}
	return nil
}

func TestSyncHostCommitsVerifiedRefreshBeforeRetryingRead(t *testing.T) {
	token, refresh, rotated := "expired", "refresh", "rotated"
	private := &syncPrivate{records: map[string]configstore.AuthRecord{"api.example": {
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token",
		AccessToken: &token, RefreshToken: &refresh, Headers: map[string]string{}, Cookies: map[string]string{},
	}}}
	var catalogCalls int
	reader := &syncReader{
		catalog: func(_ context.Context, record configstore.AuthRecord) (business.UpstreamCatalogSnapshot, error) {
			catalogCalls++
			if catalogCalls == 1 {
				return business.UpstreamCatalogSnapshot{}, &StatusError{StatusCode: http.StatusUnauthorized, Path: "/groups"}
			}
			if !private.wasSaved() || record.AccessToken == nil || *record.AccessToken != "rotated" {
				return business.UpstreamCatalogSnapshot{}, errors.New("retry happened before credential commit")
			}
			return business.UpstreamCatalogSnapshot{Groups: []business.UpstreamCatalogGroup{{GroupID: "7", Name: "pro"}}}, nil
		},
		balance: func(context.Context, configstore.AuthRecord) (business.UpstreamBalanceObservation, error) {
			value := "1"
			return business.UpstreamBalanceObservation{RawBalance: &value, Status: "已读取"}, nil
		},
	}
	refresher := &syncRefresher{refresh: func(_ context.Context, record configstore.AuthRecord) (configstore.AuthRecord, error) {
		record.AccessToken = &rotated
		return record, nil
	}}
	repository := &syncRepository{}
	service := New(repository, private, reader, refresher, &memoryTasks{done: make(chan taskstore.Task, 1)})
	result := service.syncHost(context.Background(), "api.example", Scope{Catalog: true, Balance: true}, "tester")
	if result.Status != "succeeded" || !result.AuthRecovered || catalogCalls != 2 || len(repository.applied) != 1 {
		t.Fatalf("result=%#v calls=%d writes=%d", result, catalogCalls, len(repository.applied))
	}
	if !repository.applied[0].AuthenticationOK || !repository.applied[0].AuthRecovered {
		t.Fatalf("write=%#v", repository.applied[0])
	}
}

func TestSyncHostUsesBaseURLWithoutChangingHostIdentity(t *testing.T) {
	token := "token"
	const host = "152.53.241.112:8080"
	const baseURL = "https://accelerated.example.test:8443/api"
	private := &syncPrivate{records: map[string]configstore.AuthRecord{host: {
		Host: host, BaseURL: baseURL, AccessToken: &token, Headers: map[string]string{}, Cookies: map[string]string{},
	}}}
	var requested configstore.AuthRecord
	reader := &syncReader{catalog: func(_ context.Context, record configstore.AuthRecord) (business.UpstreamCatalogSnapshot, error) {
		requested = record
		return business.UpstreamCatalogSnapshot{}, nil
	}}
	repository := &syncRepository{}
	service := New(repository, private, reader, &syncRefresher{}, &memoryTasks{done: make(chan taskstore.Task, 1)})
	result, err := service.SyncHost(context.Background(), "HTTP://152.53.241.112:8080/", Scope{Catalog: true}, "tester")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "succeeded" || result.Host != host || requested.Host != host || requested.BaseURL != baseURL {
		t.Fatalf("result=%#v requested=%#v", result, requested)
	}
}

func TestEnqueueHostMissingAuthorizationNamesTheExactHost(t *testing.T) {
	service := New(
		&syncRepository{},
		&syncPrivate{records: map[string]configstore.AuthRecord{}},
		&syncReader{},
		&syncRefresher{},
		&memoryTasks{done: make(chan taskstore.Task, 1)},
	)
	_, err := service.EnqueueHost(context.Background(), "HTTP://152.53.241.112:8080/", Scope{Catalog: true}, "tester", "sync")
	if err == nil || !strings.Contains(err.Error(), `152.53.241.112:8080`) || !strings.Contains(err.Error(), "Base URL 可以不同") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnqueueHostResolvesExplicitVaultMatchBeforeQueueing(t *testing.T) {
	token := "token"
	resolved := &configstore.AuthRecord{
		Host: "152.53.241.112:8080", BaseURL: "https://accelerated.example.test", UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", AccessToken: &token, Headers: map[string]string{}, Cookies: map[string]string{},
	}
	private := &syncPrivate{records: map[string]configstore.AuthRecord{}}
	resolver := &syncResolver{record: resolved, private: private}
	tasks := &memoryTasks{done: make(chan taskstore.Task, 1)}
	reader := &syncReader{catalog: func(context.Context, configstore.AuthRecord) (business.UpstreamCatalogSnapshot, error) {
		return business.UpstreamCatalogSnapshot{}, nil
	}}
	service := New(&syncRepository{}, private, reader, &syncRefresher{}, tasks)
	service.SetAuthResolver(resolver)

	if _, err := service.EnqueueHost(context.Background(), "152.53.241.112:8080", Scope{Catalog: true}, "tester", "sync"); err != nil {
		t.Fatal(err)
	}
	if resolver.callCount() != 1 {
		t.Fatalf("resolver calls=%d", resolver.callCount())
	}
}

func TestSyncHostDoesNotPersistPartialNetworkResult(t *testing.T) {
	token := "token"
	private := &syncPrivate{records: map[string]configstore.AuthRecord{"api.example": {
		Host: "api.example", BaseURL: "https://api.example", AccessToken: &token, Headers: map[string]string{}, Cookies: map[string]string{},
	}}}
	repository := &syncRepository{}
	reader := &syncReader{
		catalog: func(context.Context, configstore.AuthRecord) (business.UpstreamCatalogSnapshot, error) {
			return business.UpstreamCatalogSnapshot{Groups: []business.UpstreamCatalogGroup{{GroupID: "7", Name: "pro"}}}, nil
		},
		balance: func(context.Context, configstore.AuthRecord) (business.UpstreamBalanceObservation, error) {
			return business.UpstreamBalanceObservation{}, errors.New("balance timeout")
		},
	}
	service := New(repository, private, reader, &syncRefresher{refresh: func(context.Context, configstore.AuthRecord) (configstore.AuthRecord, error) {
		return configstore.AuthRecord{}, errors.New("must not refresh")
	}}, &memoryTasks{done: make(chan taskstore.Task, 1)})
	result := service.syncHost(context.Background(), "api.example", Scope{Catalog: true, Balance: true}, "tester")
	if result.Status != "failed" || len(repository.applied) != 0 || len(repository.failures) != 1 {
		t.Fatalf("partial network result persisted: result=%#v applied=%d failures=%#v", result, len(repository.applied), repository.failures)
	}
}

func TestSingleHostReadsCatalogAndBalanceInParallel(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})
	reader := &syncReader{
		catalog: func(ctx context.Context, _ configstore.AuthRecord) (business.UpstreamCatalogSnapshot, error) {
			started <- "catalog"
			select {
			case <-release:
			case <-ctx.Done():
			}
			return business.UpstreamCatalogSnapshot{}, nil
		},
		balance: func(ctx context.Context, _ configstore.AuthRecord) (business.UpstreamBalanceObservation, error) {
			started <- "balance"
			select {
			case <-release:
			case <-ctx.Done():
			}
			return business.UpstreamBalanceObservation{}, nil
		},
	}
	service := &Service{reader: reader}
	done := make(chan error, 1)
	go func() {
		_, _, err := service.read(context.Background(), configstore.AuthRecord{}, Scope{Catalog: true, Balance: true})
		done <- err
	}()
	seen := map[string]bool{}
	for range 2 {
		seen[<-started] = true
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !seen["catalog"] || !seen["balance"] {
		t.Fatalf("started=%v", seen)
	}
}

func TestSyncHostExposesFailureStatePersistenceError(t *testing.T) {
	repository := &syncRepository{failureErr: errors.New("database busy")}
	service := New(repository, &syncPrivate{records: map[string]configstore.AuthRecord{}}, &syncReader{}, &syncRefresher{}, &memoryTasks{done: make(chan taskstore.Task, 1)})

	result := service.syncHost(context.Background(), "api.example", Scope{Catalog: true}, "tester")

	if result.Status != "auth_failed" || result.Reason == nil || !strings.Contains(*result.Reason, "失败状态保存失败") || !strings.Contains(*result.Reason, "database busy") {
		t.Fatalf("result=%#v", result)
	}
}

func TestBatchSyncLimitsConcurrencyToFour(t *testing.T) {
	token := "token"
	private := &syncPrivate{records: map[string]configstore.AuthRecord{}}
	hosts := make([]string, 8)
	for index := range hosts {
		hosts[index] = "host-" + string(rune('a'+index)) + ".example"
		private.records[hosts[index]] = configstore.AuthRecord{Host: hosts[index], AccessToken: &token, Headers: map[string]string{}, Cookies: map[string]string{}}
	}
	var active, maximum atomic.Int32
	started := make(chan struct{}, 8)
	release := make(chan struct{})
	reader := &syncReader{catalog: func(ctx context.Context, _ configstore.AuthRecord) (business.UpstreamCatalogSnapshot, error) {
		current := active.Add(1)
		for {
			previous := maximum.Load()
			if current <= previous || maximum.CompareAndSwap(previous, current) {
				break
			}
		}
		started <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
		}
		active.Add(-1)
		return business.UpstreamCatalogSnapshot{}, nil
	}, balance: func(context.Context, configstore.AuthRecord) (business.UpstreamBalanceObservation, error) {
		return business.UpstreamBalanceObservation{}, nil
	}}
	service := New(&syncRepository{}, private, reader, &syncRefresher{}, &memoryTasks{done: make(chan taskstore.Task, 1)})
	done := make(chan BatchResult, 1)
	go func() { done <- service.syncHosts(context.Background(), hosts, Scope{Catalog: true}, "tester", nil) }()
	for index := 0; index < 4; index++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("four workers did not start")
		}
	}
	if maximum.Load() != 4 {
		t.Fatalf("maximum concurrency=%d", maximum.Load())
	}
	close(release)
	result := <-done
	if result.Total != 8 || result.Succeeded != 8 || maximum.Load() > 4 {
		t.Fatalf("result=%#v maximum=%d", result, maximum.Load())
	}
}

func TestEnqueuedTaskResultRedactsSecrets(t *testing.T) {
	secret := "access-secret-value"
	private := &syncPrivate{records: map[string]configstore.AuthRecord{"api.example": {
		Host: "api.example", AccessToken: &secret, Headers: map[string]string{}, Cookies: map[string]string{},
	}}}
	repository := &syncRepository{hosts: []business.UpstreamHost{{Host: "api.example"}}}
	reader := &syncReader{catalog: func(context.Context, configstore.AuthRecord) (business.UpstreamCatalogSnapshot, error) {
		return business.UpstreamCatalogSnapshot{}, errors.New("Authorization=access-secret-value")
	}, balance: func(context.Context, configstore.AuthRecord) (business.UpstreamBalanceObservation, error) {
		return business.UpstreamBalanceObservation{}, nil
	}}
	tasks := &memoryTasks{done: make(chan taskstore.Task, 2)}
	service := New(repository, private, reader, &syncRefresher{}, tasks)
	if _, err := service.EnqueueAll(context.Background(), Scope{Catalog: true}, "tester", "upstream-sync"); err != nil {
		t.Fatal(err)
	}
	var task taskstore.Task
	select {
	case task = <-tasks.done:
	case <-time.After(time.Second):
		t.Fatal("task did not finish")
	}
	encoded, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), secret) {
		t.Fatalf("task leaked credential: %s", encoded)
	}
}
