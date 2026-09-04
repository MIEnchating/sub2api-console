package upstreamconfig

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamauth"
)

type observingVerifier struct {
	business *business.Store
	private  *configstore.Store
	calls    int
}

type captureRateSyncScheduler struct {
	host          string
	actor         string
	taskID        string
	err           error
	contextErr    error
	hasDeadline   bool
	calls         int
	baseURLCalls  int
	baseURLTaskID string
	baseURLError  error
}

type updateResultBusiness struct {
	Business
	primaryHost     string
	cancel          context.CancelFunc
	readContextErr  error
	readHasDeadline bool
}

type concurrentCreateBusiness struct {
	mu     sync.Mutex
	public *business.UpstreamConfigurationWrite
}

type blockingLeaseBusiness struct {
	*concurrentCreateBusiness
	lease    chan struct{}
	attempts chan []string
}

func newBlockingLeaseBusiness() *blockingLeaseBusiness {
	return &blockingLeaseBusiness{
		concurrentCreateBusiness: &concurrentCreateBusiness{},
		lease:                    make(chan struct{}, 1),
		attempts:                 make(chan []string, 4),
	}
}

func (store *blockingLeaseBusiness) AcquireMutationLease(
	ctx context.Context,
	_ string,
	resources []string,
	_ time.Time,
	_ time.Duration,
) (bool, error) {
	store.attempts <- append([]string{}, resources...)
	select {
	case store.lease <- struct{}{}:
		return true, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

func (*blockingLeaseBusiness) RenewMutationLease(context.Context, string, []string, time.Time, time.Duration) (bool, error) {
	return true, nil
}

func (store *blockingLeaseBusiness) ReleaseMutationLease(context.Context, string, []string) error {
	<-store.lease
	return nil
}

func (store *concurrentCreateBusiness) Upstreams(context.Context) (business.UpstreamSummary, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	result := business.UpstreamSummary{Hosts: []business.UpstreamHost{}}
	if store.public != nil {
		result.Hosts = append(result.Hosts, business.UpstreamHost{
			Host: store.public.Host, Name: strings.TrimSpace(*store.public.Name), BaseURL: store.public.BaseURL,
			UpstreamType: store.public.UpstreamType, RechargeRate: store.public.RechargeRate,
		})
	}
	return result, nil
}

func (*concurrentCreateBusiness) UpstreamGroups(context.Context, string, bool) ([]business.UpstreamGroup, error) {
	return []business.UpstreamGroup{}, nil
}

func (store *concurrentCreateBusiness) UpstreamExists(context.Context, string) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.public != nil, nil
}

func (store *concurrentCreateBusiness) CreateUpstreamConfiguration(_ context.Context, value business.UpstreamConfigurationWrite) (business.UpstreamConfigurationWriteResult, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.public != nil {
		return business.UpstreamConfigurationWriteResult{}, ErrConflict
	}
	copy := value
	store.public = &copy
	return business.UpstreamConfigurationWriteResult{}, nil
}

func (*concurrentCreateBusiness) UpdateUpstreamConfiguration(context.Context, business.UpstreamConfigurationWrite) (business.UpstreamConfigurationWriteResult, error) {
	return business.UpstreamConfigurationWriteResult{}, errors.New("unexpected update")
}

func (*concurrentCreateBusiness) RecordRuntimeEvent(context.Context, string, string, string, map[string]any) (int64, error) {
	return -1, nil
}

type concurrentPrivateStore struct {
	mu     sync.Mutex
	record *configstore.AuthRecord
}

func (store *concurrentPrivateStore) AuthRecord(context.Context, string) (*configstore.AuthRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.record == nil {
		return nil, nil
	}
	copy := cloneRecord(*store.record)
	return &copy, nil
}

func (store *concurrentPrivateStore) SaveAuthRecord(_ context.Context, record configstore.AuthRecord, _ map[string]bool) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	copy := cloneRecord(record)
	store.record = &copy
	return nil
}

func (store *concurrentPrivateStore) DeleteAuthRecord(context.Context, string) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	found := store.record != nil
	store.record = nil
	return found, nil
}

func (*concurrentPrivateStore) VaultEntry(context.Context, string) (*configstore.VaultEntry, error) {
	return nil, nil
}

func (*concurrentPrivateStore) SaveVaultEntry(context.Context, configstore.VaultEntry, map[string]bool) error {
	return nil
}

func (*concurrentPrivateStore) DeleteVaultEntry(context.Context, string) (bool, error) {
	return false, nil
}

type firstVerifyBlocker struct {
	mu      sync.Mutex
	calls   int
	started chan struct{}
	release chan struct{}
}

type firstLoginBlocker struct {
	mu      sync.Mutex
	calls   int
	started chan struct{}
	release chan struct{}
}

type cancelingAuthSaveStore struct {
	cancel          context.CancelFunc
	vaultSaved      bool
	rollbackStarted chan struct{}
	allowRollback   chan struct{}
	rollbackErr     error
	hasDeadline     bool
}

func (*cancelingAuthSaveStore) AuthRecord(context.Context, string) (*configstore.AuthRecord, error) {
	return nil, nil
}

func (store *cancelingAuthSaveStore) SaveAuthRecord(context.Context, configstore.AuthRecord, map[string]bool) error {
	store.cancel()
	return errors.New("auth save failed")
}

func (*cancelingAuthSaveStore) DeleteAuthRecord(context.Context, string) (bool, error) {
	return false, nil
}

func (*cancelingAuthSaveStore) VaultEntry(context.Context, string) (*configstore.VaultEntry, error) {
	return nil, nil
}

func (store *cancelingAuthSaveStore) SaveVaultEntry(context.Context, configstore.VaultEntry, map[string]bool) error {
	store.vaultSaved = true
	return nil
}

func (store *cancelingAuthSaveStore) DeleteVaultEntry(ctx context.Context, _ string) (bool, error) {
	store.rollbackErr = ctx.Err()
	_, store.hasDeadline = ctx.Deadline()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if store.rollbackStarted != nil {
		close(store.rollbackStarted)
	}
	if store.allowRollback != nil {
		<-store.allowRollback
	}
	found := store.vaultSaved
	store.vaultSaved = false
	return found, nil
}

func (verifier *firstVerifyBlocker) Verify(context.Context, configstore.AuthRecord) error {
	verifier.mu.Lock()
	verifier.calls++
	first := verifier.calls == 1
	verifier.mu.Unlock()
	if first {
		close(verifier.started)
		<-verifier.release
	}
	return nil
}

func (*firstVerifyBlocker) Login(_ context.Context, record configstore.AuthRecord, _ configstore.VaultEntry) (configstore.AuthRecord, error) {
	return record, nil
}

func (*firstLoginBlocker) Verify(context.Context, configstore.AuthRecord) error { return nil }

func (verifier *firstLoginBlocker) Login(_ context.Context, record configstore.AuthRecord, _ configstore.VaultEntry) (configstore.AuthRecord, error) {
	verifier.mu.Lock()
	verifier.calls++
	first := verifier.calls == 1
	verifier.mu.Unlock()
	if first {
		close(verifier.started)
		<-verifier.release
	}
	return record, nil
}

func (scheduler *captureRateSyncScheduler) EnqueueHostAccountRateSync(ctx context.Context, host, actor string) (string, error) {
	scheduler.calls++
	scheduler.host = host
	scheduler.actor = actor
	scheduler.contextErr = ctx.Err()
	_, scheduler.hasDeadline = ctx.Deadline()
	return scheduler.taskID, scheduler.err
}

func (scheduler *captureRateSyncScheduler) EnqueueHostAccountBaseURLSync(ctx context.Context, host, actor string) (string, error) {
	scheduler.baseURLCalls++
	scheduler.host = host
	scheduler.actor = actor
	scheduler.contextErr = ctx.Err()
	_, scheduler.hasDeadline = ctx.Deadline()
	return scheduler.baseURLTaskID, scheduler.baseURLError
}

func (store *updateResultBusiness) UpdateUpstreamConfiguration(ctx context.Context, value business.UpstreamConfigurationWrite) (business.UpstreamConfigurationWriteResult, error) {
	result, err := store.Business.UpdateUpstreamConfiguration(ctx, value)
	if err != nil {
		return result, err
	}
	if store.primaryHost != "" {
		result.PrimaryHost = store.primaryHost
	}
	if store.cancel != nil {
		store.cancel()
	}
	return result, nil
}

func (store *updateResultBusiness) CreateUpstreamConfiguration(ctx context.Context, value business.UpstreamConfigurationWrite) (business.UpstreamConfigurationWriteResult, error) {
	result, err := store.Business.CreateUpstreamConfiguration(ctx, value)
	if err == nil && store.cancel != nil {
		store.cancel()
	}
	return result, err
}

func (store *updateResultBusiness) Upstreams(ctx context.Context) (business.UpstreamSummary, error) {
	store.readContextErr = ctx.Err()
	_, store.readHasDeadline = ctx.Deadline()
	return store.Business.Upstreams(ctx)
}

func TestConcurrentCreateDoesNotLetFailedRequestDeleteSuccessfulCredentials(t *testing.T) {
	businessStore := &concurrentCreateBusiness{}
	private := &concurrentPrivateStore{}
	verifier := &firstVerifyBlocker{started: make(chan struct{}), release: make(chan struct{})}
	service := New(businessStore, private, verifier)
	name := "Example"
	firstToken, secondToken := "first-token", "second-token"
	input := func(token *string) Input {
		return Input{
			Host: "api.example", Name: &name, BaseURL: "https://api.example", UpstreamType: "custom",
			AuthMode: "custom_headers", RechargeRate: "1", AccessToken: token,
			Headers: map[string]string{"X-API-Key": *token}, Present: map[string]bool{"access_token": true, "headers": true},
		}
	}
	firstDone := make(chan error, 1)
	go func() {
		_, err := service.Create(context.Background(), input(&firstToken), "first")
		firstDone <- err
	}()
	<-verifier.started
	secondCtx, cancelSecond := context.WithCancel(context.Background())
	cancelSecond()
	secondDone := make(chan error, 1)
	go func() {
		_, err := service.Create(secondCtx, input(&secondToken), "second")
		secondDone <- err
	}()
	secondErr := <-secondDone
	close(verifier.release)
	firstErr := <-firstDone

	if firstErr != nil {
		t.Fatalf("first create failed: %v", firstErr)
	}
	if !errors.Is(secondErr, context.Canceled) {
		t.Fatalf("second create error = %v, want context.Canceled", secondErr)
	}
	record, err := private.AuthRecord(context.Background(), "api.example")
	if err != nil || record == nil || record.AccessToken == nil || *record.AccessToken != firstToken {
		t.Fatalf("successful credentials were not preserved: record=%#v err=%v", record, err)
	}
}

func TestCreateSerializesUpstreamIdentityCatalogMutation(t *testing.T) {
	businessStore := newBlockingLeaseBusiness()
	private := &concurrentPrivateStore{}
	service := New(businessStore, private, &passVerifier{})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, catalogRelease, err := mutationguard.Acquire(ctx, businessStore, mutationguard.UpstreamCatalog())
	if err != nil {
		t.Fatal(err)
	}
	defer catalogRelease()
	resources := <-businessStore.attempts
	if len(resources) != 1 || resources[0] != "upstream-catalog" {
		t.Fatalf("catalog lease resources = %#v", resources)
	}
	name, token := "Example", "token"
	created := make(chan error, 1)
	go func() {
		_, err := service.Create(ctx, Input{
			Host: "HTTPS://API.EXAMPLE/", Name: &name, BaseURL: "https://api.example", UpstreamType: "custom",
			AuthMode: "custom_headers", RechargeRate: "1", AccessToken: &token,
			Headers: map[string]string{"X-API-Key": token}, Present: map[string]bool{"access_token": true, "headers": true},
		}, "operator")
		created <- err
	}()
	resources = <-businessStore.attempts
	if len(resources) != 2 || resources[0] != "upstream-catalog" || resources[1] != "upstream/api.example" {
		t.Fatalf("create lease resources = %#v", resources)
	}
	select {
	case err := <-created:
		t.Fatalf("create bypassed upstream catalog lease: %v", err)
	default:
	}
	if err := catalogRelease(); err != nil {
		t.Fatal(err)
	}
	if err := <-created; err != nil {
		t.Fatal(err)
	}
}

func TestConfigureAuthRecordSerializesWithCanonicalUpstreamDelete(t *testing.T) {
	name, oldToken, newToken := "Example", "old", "new"
	businessStore := newBlockingLeaseBusiness()
	businessStore.public = &business.UpstreamConfigurationWrite{
		Host: "api.example", Name: &name, BaseURL: "https://api.example", UpstreamType: "custom", AuthMode: "custom_headers", RechargeRate: "1",
	}
	private := &concurrentPrivateStore{record: &configstore.AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "custom", AuthMode: "custom_headers",
		AccessToken: &oldToken, Headers: map[string]string{"X-API-Key": oldToken}, Cookies: map[string]string{},
	}}
	verifier := &firstVerifyBlocker{started: make(chan struct{}), release: make(chan struct{})}
	service := New(businessStore, private, verifier)
	configured := make(chan error, 1)
	go func() {
		_, err := service.ConfigureAuthRecord(context.Background(), Input{
			Host: "HTTPS://API.EXAMPLE/", BaseURL: "https://api.example", UpstreamType: "custom", AuthMode: "custom_headers",
			AccessToken: &newToken, Headers: map[string]string{"X-API-Key": newToken},
			Present: map[string]bool{"access_token": true, "headers": true},
		})
		configured <- err
	}()
	resources := <-businessStore.attempts
	if len(resources) != 1 || resources[0] != "upstream/api.example" {
		t.Fatalf("configuration lease resources = %#v", resources)
	}
	<-verifier.started

	deleteAcquired := make(chan struct{})
	deleteDone := make(chan error, 1)
	go func() {
		_, release, err := mutationguard.Acquire(context.Background(), businessStore, mutationguard.Upstream("https://API.EXAMPLE/"))
		if err != nil {
			deleteDone <- err
			return
		}
		close(deleteAcquired)
		businessStore.mu.Lock()
		businessStore.public = nil
		businessStore.mu.Unlock()
		_, err = private.DeleteAuthRecord(context.Background(), "api.example")
		if releaseErr := release(); err == nil {
			err = releaseErr
		}
		deleteDone <- err
	}()
	resources = <-businessStore.attempts
	if len(resources) != 1 || resources[0] != "upstream/api.example" {
		t.Fatalf("delete lease resources = %#v", resources)
	}
	select {
	case <-deleteAcquired:
		t.Fatal("delete acquired the canonical upstream while configuration was verifying")
	default:
	}
	close(verifier.release)
	if err := <-configured; err != nil {
		t.Fatalf("configuration failed: %v", err)
	}
	if err := <-deleteDone; err != nil {
		t.Fatalf("delete simulation failed: %v", err)
	}
	if record, err := private.AuthRecord(context.Background(), "api.example"); err != nil || record != nil {
		t.Fatalf("delete did not win after waiting for configuration: record=%#v err=%v", record, err)
	}
}

func TestConfigureAuthRecordRechecksExistenceAfterCanonicalDelete(t *testing.T) {
	name, oldToken, newToken := "Example", "old", "new"
	businessStore := newBlockingLeaseBusiness()
	businessStore.public = &business.UpstreamConfigurationWrite{
		Host: "api.example", Name: &name, BaseURL: "https://api.example", UpstreamType: "custom", AuthMode: "custom_headers", RechargeRate: "1",
	}
	private := &concurrentPrivateStore{record: &configstore.AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "custom", AuthMode: "custom_headers",
		AccessToken: &oldToken, Headers: map[string]string{"X-API-Key": oldToken}, Cookies: map[string]string{},
	}}
	_, deleteRelease, err := mutationguard.Acquire(context.Background(), businessStore, mutationguard.Upstream("HTTPS://API.EXAMPLE/"))
	if err != nil {
		t.Fatal(err)
	}
	resources := <-businessStore.attempts
	if len(resources) != 1 || resources[0] != "upstream/api.example" {
		t.Fatalf("delete lease resources = %#v", resources)
	}
	businessStore.mu.Lock()
	businessStore.public = nil
	businessStore.mu.Unlock()
	if _, err := private.DeleteAuthRecord(context.Background(), "api.example"); err != nil {
		t.Fatal(err)
	}

	verifier := &firstVerifyBlocker{started: make(chan struct{}), release: make(chan struct{})}
	service := New(businessStore, private, verifier)
	configured := make(chan error, 1)
	go func() {
		_, err := service.ConfigureAuthRecord(context.Background(), Input{
			Host: "api.example", BaseURL: "https://api.example", UpstreamType: "custom", AuthMode: "custom_headers",
			AccessToken: &newToken, Headers: map[string]string{"X-API-Key": newToken},
			Present: map[string]bool{"access_token": true, "headers": true},
		})
		configured <- err
	}()
	resources = <-businessStore.attempts
	if len(resources) != 1 || resources[0] != "upstream/api.example" {
		t.Fatalf("configuration lease resources = %#v", resources)
	}
	if err := deleteRelease(); err != nil {
		t.Fatal(err)
	}
	if err := <-configured; !errors.Is(err, ErrNotFound) {
		t.Fatalf("configuration error = %v, want ErrNotFound", err)
	}
	verifier.mu.Lock()
	verifyCalls := verifier.calls
	verifier.mu.Unlock()
	if verifyCalls != 0 {
		t.Fatalf("deleted upstream reached verification: calls=%d", verifyCalls)
	}
	if record, err := private.AuthRecord(context.Background(), "api.example"); err != nil || record != nil {
		t.Fatalf("configuration resurrected deleted auth: record=%#v err=%v", record, err)
	}
}

func TestConfigureAuthRecordSerializesSharedVaultEntryAcrossHosts(t *testing.T) {
	private, _ := openStores(t)
	name := "Example"
	businessStore := &concurrentCreateBusiness{public: &business.UpstreamConfigurationWrite{
		Host: "first.example", Name: &name, BaseURL: "https://first.example", UpstreamType: "sub2api",
		AuthMode: "sub2api_manual_login", RechargeRate: "1",
	}}
	verifier := &firstLoginBlocker{started: make(chan struct{}), release: make(chan struct{})}
	service := New(businessStore, private, verifier)
	entry := "shared-login"
	input := func(host, username, password string) Input {
		return Input{
			Host: host, BaseURL: "https://" + host, UpstreamType: "sub2api", AuthMode: "sub2api_manual_login",
			Username: &username, Password: &password, SaveToVault: true, Entry: &entry,
			Headers: map[string]string{}, Cookies: map[string]string{},
			Present: map[string]bool{"username": true, "password": true, "save_to_vault": true, "entry": true},
		}
	}

	firstDone := make(chan error, 1)
	go func() {
		_, err := service.ConfigureAuthRecord(context.Background(), input("first.example", "first@example.test", "first-secret"))
		firstDone <- err
	}()
	<-verifier.started

	secondCtx, cancelSecond := context.WithTimeout(context.Background(), 100*time.Millisecond)
	_, secondErr := service.ConfigureAuthRecord(secondCtx, input("second.example", "second@example.test", "second-secret"))
	cancelSecond()
	if !errors.Is(secondErr, context.DeadlineExceeded) {
		t.Fatalf("second host changed the shared vault entry concurrently: %v", secondErr)
	}

	close(verifier.release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first configuration failed: %v", err)
	}
	if _, err := service.ConfigureAuthRecord(context.Background(), input("second.example", "second@example.test", "second-secret")); err != nil {
		t.Fatalf("second configuration failed after the shared vault lease was released: %v", err)
	}
	stored, err := private.VaultEntry(context.Background(), entry)
	if err != nil || stored == nil || stored.Username == nil || *stored.Username != "second@example.test" || stored.Password == nil || *stored.Password != "second-secret" {
		t.Fatalf("shared vault entry did not preserve serialized write order: entry=%#v err=%v", stored, err)
	}
}

func TestPrivateCommitRollsBackVaultAfterRequestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	private := &cancelingAuthSaveStore{cancel: cancel}
	service := New(failingCreateBusiness{}, private, &passVerifier{})
	entry := configstore.VaultEntry{Entry: "api.example"}
	_, err := service.commitPrivateAndPublic(ctx, configstore.AuthRecord{Host: "api.example"}, Input{}, &vaultChange{entry: &entry}, true)
	if err == nil || !strings.Contains(err.Error(), "auth save failed") {
		t.Fatalf("commit error = %v", err)
	}
	if private.vaultSaved {
		t.Fatal("vault entry remained after the public commit was aborted")
	}
	if private.rollbackErr != nil || !private.hasDeadline {
		t.Fatalf("compensation context: err=%v has_deadline=%v", private.rollbackErr, private.hasDeadline)
	}
}

func TestConfigureAuthRecordKeepsVaultLeaseThroughCompensation(t *testing.T) {
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	private := &cancelingAuthSaveStore{
		cancel: cancelRequest, rollbackStarted: make(chan struct{}), allowRollback: make(chan struct{}),
	}
	name := "Example"
	businessStore := &concurrentCreateBusiness{public: &business.UpstreamConfigurationWrite{
		Host: "api.example", Name: &name, BaseURL: "https://api.example", UpstreamType: "sub2api",
		AuthMode: "sub2api_manual_login", RechargeRate: "1",
	}}
	service := New(businessStore, private, &passVerifier{})
	username, password, entry := "operator@example.test", "secret", "shared-login"
	configured := make(chan error, 1)
	go func() {
		_, err := service.ConfigureAuthRecord(requestCtx, Input{
			Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_manual_login",
			Username: &username, Password: &password, SaveToVault: true, Entry: &entry,
			Headers: map[string]string{}, Cookies: map[string]string{},
			Present: map[string]bool{"username": true, "password": true, "save_to_vault": true, "entry": true},
		})
		configured <- err
	}()
	<-private.rollbackStarted

	competingCtx, cancelCompeting := context.WithTimeout(context.Background(), 100*time.Millisecond)
	_, competingRelease, competingErr := mutationguard.Acquire(competingCtx, businessStore, mutationguard.Vault(entry))
	cancelCompeting()
	if competingRelease != nil {
		_ = competingRelease()
	}
	if !errors.Is(competingErr, context.DeadlineExceeded) {
		close(private.allowRollback)
		t.Fatalf("competing vault mutation acquired during compensation: %v", competingErr)
	}

	close(private.allowRollback)
	if err := <-configured; err == nil || !strings.Contains(err.Error(), "auth save failed") {
		t.Fatalf("configuration error = %v", err)
	}
	if private.vaultSaved {
		t.Fatal("vault entry remained after compensation")
	}
}

func (v *observingVerifier) Verify(ctx context.Context, record configstore.AuthRecord) error {
	v.calls++
	exists, err := v.business.UpstreamExists(ctx, record.Host)
	if err != nil {
		return err
	}
	stored, err := v.private.AuthRecord(ctx, record.Host)
	if err != nil {
		return err
	}
	if exists || stored != nil {
		return errors.New("鉴权期间不应存在未验证的数据库写入")
	}
	return nil
}

func (v *observingVerifier) Login(context.Context, configstore.AuthRecord, configstore.VaultEntry) (configstore.AuthRecord, error) {
	return configstore.AuthRecord{}, errors.New("unexpected login")
}

func TestCreateVerifiesBeforeBothDatabaseWritesAndReturnsRedactedConfiguration(t *testing.T) {
	private, businessStore := openStores(t)
	verifier := &observingVerifier{business: businessStore, private: private}
	service := New(businessStore, private, verifier)
	token, refresh, name := "access", "refresh", "Example"
	result, err := service.Create(context.Background(), Input{
		Host: "HTTPS://API.EXAMPLE/", Name: &name, BaseURL: "https://api.example/", UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", RechargeRate: "1", AccessToken: &token, RefreshToken: &refresh,
		Headers: map[string]string{"X-Site": "one"}, Cookies: map[string]string{},
		Present: map[string]bool{"access_token": true, "refresh_token": true, "headers": true, "cookies": true},
	}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if verifier.calls != 1 || result.Host != "api.example" || !result.HasAccessToken || !result.HasRefreshToken || result.Headers["X-Site"] != "one" {
		t.Fatalf("unexpected result: %#v calls=%d", result, verifier.calls)
	}
	stored, err := private.AuthRecord(context.Background(), "api.example")
	if err != nil || stored == nil || stored.AccessToken == nil || *stored.AccessToken != "access" {
		t.Fatalf("private record missing: %#v err=%v", stored, err)
	}
}

func TestCreateFinishesReadbackAfterCommittedRequestIsCanceled(t *testing.T) {
	private, businessStore := openStores(t)
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	committedBusiness := &updateResultBusiness{Business: businessStore, cancel: cancelRequest}
	service := New(committedBusiness, private, &passVerifier{})
	token, refresh, name := "access", "refresh", "Example"

	result, err := service.Create(requestCtx, Input{
		Host: "api.example", Name: &name, BaseURL: "https://api.example", UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", RechargeRate: "1", AccessToken: &token, RefreshToken: &refresh,
		Present: map[string]bool{"access_token": true, "refresh_token": true},
	}, "operator")
	if err != nil {
		t.Fatalf("committed configuration was reported as failed after cancellation: %v", err)
	}
	if result.Host != "api.example" || result.RechargeRate != "1" {
		t.Fatalf("committed configuration readback missing: %#v", result)
	}
	if committedBusiness.readContextErr != nil || !committedBusiness.readHasDeadline {
		t.Fatalf("configuration readback context must be detached and bounded: err=%v deadline=%v", committedBusiness.readContextErr, committedBusiness.readHasDeadline)
	}
}

func TestCreateKeepsHostIdentityIndependentFromBaseURL(t *testing.T) {
	private, businessStore := openStores(t)
	service := New(businessStore, private, &passVerifier{})
	access, refresh := "access", "refresh"
	tests := []struct {
		host    string
		baseURL string
	}{
		{host: "192.0.2.44:8080", baseURL: "https://accelerated.example.test:8443/api"},
		{host: "origin.example.test:8081", baseURL: "http://10.0.0.8:8000"},
	}
	for _, test := range tests {
		name := test.host
		result, err := service.Create(context.Background(), Input{
			Host: test.host, Name: &name, BaseURL: test.baseURL, UpstreamType: "sub2api",
			AuthMode: "sub2api_user_token", RechargeRate: "1", AccessToken: &access, RefreshToken: &refresh,
			Present: map[string]bool{"access_token": true, "refresh_token": true},
		}, "operator")
		if err != nil {
			t.Fatalf("create %s: %v", test.host, err)
		}
		if result.Host != test.host || result.BaseURL != test.baseURL {
			t.Fatalf("host/base URL were coupled: %#v", result)
		}
		stored, err := private.AuthRecord(context.Background(), test.host)
		if err != nil || stored == nil || stored.Host != test.host || stored.BaseURL != test.baseURL {
			t.Fatalf("stored record = %#v err=%v", stored, err)
		}
	}
}

func TestUpdatePreservesOmittedSecretAndClearsExplicitNull(t *testing.T) {
	private, businessStore := openStores(t)
	verifier := &passVerifier{}
	service := New(businessStore, private, verifier)
	token, refresh, name := "access", "refresh", "Example"
	_, err := service.Create(context.Background(), Input{Host: "api.example", Name: &name, BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1", AccessToken: &token, RefreshToken: &refresh, Present: map[string]bool{"access_token": true, "refresh_token": true}}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Update(context.Background(), "api.example", Input{Name: &name, BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "2", RefreshToken: &refresh, Present: map[string]bool{"refresh_token": true}}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	stored, _ := private.AuthRecord(context.Background(), "api.example")
	if stored.AccessToken == nil || *stored.AccessToken != "access" {
		t.Fatalf("omitted access token was lost: %#v", stored)
	}
	_, err = service.Update(context.Background(), "api.example", Input{Name: &name, BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "2", AccessToken: nil, RefreshToken: &refresh, Present: map[string]bool{"access_token": true, "refresh_token": true}}, "operator")
	if err == nil {
		t.Fatal("explicit null access token must fail verification and remain uncommitted")
	}
	stored, _ = private.AuthRecord(context.Background(), "api.example")
	if stored.AccessToken == nil || *stored.AccessToken != "access" {
		t.Fatalf("failed update changed stored token: %#v", stored)
	}
}

func TestUpdateQueuesAccountRateSyncOnlyWhenRechargeRateValueChanges(t *testing.T) {
	private, businessStore := openStores(t)
	scheduler := &captureRateSyncScheduler{taskID: "task-rate-sync"}
	service := New(businessStore, private, &passVerifier{}, scheduler)
	token, refresh, name := "access", "refresh", "Example"
	_, err := service.Create(context.Background(), Input{
		Host: "api.example", Name: &name, BaseURL: "https://api.example", UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", RechargeRate: "1", AccessToken: &token, RefreshToken: &refresh,
		Present: map[string]bool{"access_token": true, "refresh_token": true},
	}, "operator")
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.Update(context.Background(), "api.example", Input{
		Name: &name, BaseURL: "https://api.example", UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", RechargeRate: "1.0", RefreshToken: &refresh,
		Present: map[string]bool{"refresh_token": true},
	}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if scheduler.calls != 0 || result.RateSyncTaskID != nil {
		t.Fatalf("equal recharge value unexpectedly queued: result=%#v scheduler=%#v", result, scheduler)
	}

	result, err = service.Update(context.Background(), "api.example", Input{
		Name: &name, BaseURL: "https://api.example", UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", RechargeRate: "10", RefreshToken: &refresh,
		Present: map[string]bool{"refresh_token": true},
	}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if scheduler.calls != 1 || scheduler.host != "api.example" || scheduler.actor != "operator" || result.RateSyncTaskID == nil || *result.RateSyncTaskID != "task-rate-sync" {
		t.Fatalf("changed recharge value did not queue correctly: result=%#v scheduler=%#v", result, scheduler)
	}
}

func TestUpdateQueuesRateSyncForCommittedIdentityPrimaryHost(t *testing.T) {
	private, businessStore := openStores(t)
	token, refresh, name := "access", "refresh", "Example"
	service := New(businessStore, private, &passVerifier{})
	_, err := service.Create(context.Background(), Input{
		Host: "api.example", Name: &name, BaseURL: "https://api.example", UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", RechargeRate: "1", AccessToken: &token, RefreshToken: &refresh,
		Present: map[string]bool{"access_token": true, "refresh_token": true},
	}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	scheduler := &captureRateSyncScheduler{taskID: "task-rate-sync"}
	service = New(&updateResultBusiness{Business: businessStore, primaryHost: "primary.example"}, private, &passVerifier{}, scheduler)

	result, err := service.Update(context.Background(), "api.example", Input{
		Name: &name, BaseURL: "https://api.example", UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", RechargeRate: "10", RefreshToken: &refresh,
		Present: map[string]bool{"refresh_token": true},
	}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if scheduler.calls != 1 || scheduler.host != "primary.example" || result.RateSyncTaskID == nil || *result.RateSyncTaskID != "task-rate-sync" {
		t.Fatalf("identity rate sync used the edited alias: result=%#v scheduler=%#v", result, scheduler)
	}
}

func TestUpdateQueuesAllBoundAccountBaseURLsWhenUpstreamSettingChanges(t *testing.T) {
	private, businessStore := openStores(t)
	token, refresh, name := "access", "refresh", "Example"
	service := New(businessStore, private, &passVerifier{})
	_, err := service.Create(context.Background(), Input{
		Host: "api.example", Name: &name, BaseURL: "https://api.example", AccountBaseURL: "https://old-account.example/v1",
		UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1",
		AccessToken: &token, RefreshToken: &refresh, Present: map[string]bool{"access_token": true, "refresh_token": true},
	}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	scheduler := &captureRateSyncScheduler{baseURLTaskID: "task-base-url-sync"}
	service = New(businessStore, private, &passVerifier{}, scheduler)
	result, err := service.Update(context.Background(), "api.example", Input{
		Name: &name, BaseURL: "https://api.example", AccountBaseURL: "https://new-account.example/v2",
		UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1", RefreshToken: &refresh,
		Present: map[string]bool{"refresh_token": true},
	}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if scheduler.calls != 0 || scheduler.baseURLCalls != 1 || scheduler.host != "api.example" ||
		result.BaseURLSyncTaskID == nil || *result.BaseURLSyncTaskID != "task-base-url-sync" {
		t.Fatalf("Base URL change did not queue account sync: result=%#v scheduler=%#v", result, scheduler)
	}
}

func TestUpdateFinishesReadbackAndRateSyncAfterCommittedRequestIsCanceled(t *testing.T) {
	private, businessStore := openStores(t)
	token, refresh, name := "access", "refresh", "Example"
	service := New(businessStore, private, &passVerifier{})
	_, err := service.Create(context.Background(), Input{
		Host: "api.example", Name: &name, BaseURL: "https://api.example", UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", RechargeRate: "1", AccessToken: &token, RefreshToken: &refresh,
		Present: map[string]bool{"access_token": true, "refresh_token": true},
	}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	scheduler := &captureRateSyncScheduler{taskID: "task-rate-sync"}
	committedBusiness := &updateResultBusiness{Business: businessStore, cancel: cancelRequest}
	service = New(committedBusiness, private, &passVerifier{}, scheduler)

	result, err := service.Update(requestCtx, "api.example", Input{
		Name: &name, BaseURL: "https://api.example", UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", RechargeRate: "10", RefreshToken: &refresh,
		Present: map[string]bool{"refresh_token": true},
	}, "operator")
	if err != nil {
		t.Fatalf("committed configuration was reported as failed after cancellation: %v", err)
	}
	if result.RechargeRate != "10" || result.RateSyncTaskID == nil || *result.RateSyncTaskID != "task-rate-sync" {
		t.Fatalf("committed configuration or rate task metadata missing: %#v", result)
	}
	if scheduler.contextErr != nil || !scheduler.hasDeadline || committedBusiness.readContextErr != nil || !committedBusiness.readHasDeadline {
		t.Fatalf("post-commit contexts must be detached and bounded: schedule_err=%v schedule_deadline=%v read_err=%v read_deadline=%v",
			scheduler.contextErr, scheduler.hasDeadline, committedBusiness.readContextErr, committedBusiness.readHasDeadline)
	}
}

func TestUpdateKeepsSavedConfigurationWhenRateSyncQueueFails(t *testing.T) {
	private, businessStore := openStores(t)
	scheduler := &captureRateSyncScheduler{err: errors.New("task storage unavailable")}
	service := New(businessStore, private, &passVerifier{}, scheduler)
	token, refresh, name := "access", "refresh", "Example"
	_, err := service.Create(context.Background(), Input{
		Host: "api.example", Name: &name, BaseURL: "https://api.example", UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", RechargeRate: "1", AccessToken: &token, RefreshToken: &refresh,
		Present: map[string]bool{"access_token": true, "refresh_token": true},
	}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Update(context.Background(), "api.example", Input{
		Name: &name, BaseURL: "https://api.example", UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", RechargeRate: "10", RefreshToken: &refresh,
		Present: map[string]bool{"refresh_token": true},
	}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if result.RechargeRate != "10" || result.RateSyncError == nil || !strings.Contains(*result.RateSyncError, "task storage unavailable") {
		t.Fatalf("saved configuration or queue error metadata missing: %#v", result)
	}
	stored, err := service.Get(context.Background(), "api.example")
	if err != nil || stored.RechargeRate != "10" {
		t.Fatalf("configuration was rolled back after queue failure: %#v err=%v", stored, err)
	}
}

type passVerifier struct{}

func (*passVerifier) Verify(context.Context, configstore.AuthRecord) error { return nil }
func (*passVerifier) Login(_ context.Context, record configstore.AuthRecord, _ configstore.VaultEntry) (configstore.AuthRecord, error) {
	return record, nil
}

type capturingLoginVerifier struct {
	credential configstore.VaultEntry
	options    []upstreamauth.LoginOptions
}

func (*capturingLoginVerifier) Verify(context.Context, configstore.AuthRecord) error { return nil }
func (v *capturingLoginVerifier) Login(_ context.Context, record configstore.AuthRecord, credential configstore.VaultEntry) (configstore.AuthRecord, error) {
	v.credential = credential
	return record, nil
}
func (v *capturingLoginVerifier) LoginWithOptions(_ context.Context, record configstore.AuthRecord, credential configstore.VaultEntry, options upstreamauth.LoginOptions) (configstore.AuthRecord, error) {
	v.credential = credential
	v.options = append(v.options, options)
	return record, nil
}

type failingCreateBusiness struct{}

func (failingCreateBusiness) Upstreams(context.Context) (business.UpstreamSummary, error) {
	return business.UpstreamSummary{}, nil
}
func (failingCreateBusiness) UpstreamGroups(context.Context, string, bool) ([]business.UpstreamGroup, error) {
	return []business.UpstreamGroup{}, nil
}
func (failingCreateBusiness) UpstreamExists(context.Context, string) (bool, error) { return false, nil }
func (failingCreateBusiness) CreateUpstreamConfiguration(context.Context, business.UpstreamConfigurationWrite) (business.UpstreamConfigurationWriteResult, error) {
	return business.UpstreamConfigurationWriteResult{}, errors.New("public write failed")
}
func (failingCreateBusiness) UpdateUpstreamConfiguration(context.Context, business.UpstreamConfigurationWrite) (business.UpstreamConfigurationWriteResult, error) {
	return business.UpstreamConfigurationWriteResult{}, errors.New("unexpected update")
}
func (failingCreateBusiness) RecordRuntimeEvent(context.Context, string, string, string, map[string]any) (int64, error) {
	return -1, nil
}

type failingAuthRollbackStore struct {
	*configstore.Store
}

func (s *failingAuthRollbackStore) DeleteAuthRecord(context.Context, string) (bool, error) {
	return false, errors.New("private rollback failed")
}

func TestCreateReportsPublicFailureAndPrivateCompensationFailure(t *testing.T) {
	private, err := configstore.Open(filepath.Join(t.TempDir(), "config.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { private.Close() })
	service := New(failingCreateBusiness{}, &failingAuthRollbackStore{Store: private}, &passVerifier{})
	record := configstore.AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "custom", AuthMode: "custom_headers",
		Headers: map[string]string{"X-API-Key": "secret"}, Cookies: map[string]string{},
	}
	_, err = service.commitPrivateAndPublic(context.Background(), record, Input{
		Name: stringPointer("Example"), RechargeRate: "1",
	}, nil, true)
	if err == nil || !strings.Contains(err.Error(), "public write failed") || !strings.Contains(err.Error(), "private rollback failed") || !strings.Contains(err.Error(), "补偿回滚失败") {
		t.Fatalf("combined compensation error = %v", err)
	}
}

func stringPointer(value string) *string { return &value }

func TestExplicitHeadersAreAppliedToSelectedVaultLoginCandidate(t *testing.T) {
	private, businessStore := openStores(t)
	username, password := "operator", "secret"
	name := "Example"
	if _, err := businessStore.CreateUpstreamConfiguration(context.Background(), business.UpstreamConfigurationWrite{
		Host: "api.example", Name: &name, BaseURL: "https://api.example", UpstreamType: "newapi",
		AuthMode: "newapi_user_login", RechargeRate: "1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := private.SaveVaultEntry(context.Background(), configstore.VaultEntry{
		Entry: "selected", Username: &username, Password: &password, Hosts: []string{"api.example"}, Headers: map[string]string{},
	}, nil); err != nil {
		t.Fatal(err)
	}
	verifier := &capturingLoginVerifier{}
	service := New(businessStore, private, verifier)
	entry := "selected"
	_, err := service.ConfigureAuthRecord(context.Background(), Input{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "newapi", AuthMode: "newapi_user_login", RechargeRate: "1",
		Entry: &entry, Headers: map[string]string{"Authorization": "Bearer gateway", "X-CF-Access": "signed"},
		AcceptLoginAgreement: true,
		Present:              map[string]bool{"entry": true, "headers": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if verifier.credential.Headers["Authorization"] != "Bearer gateway" || verifier.credential.Headers["X-CF-Access"] != "signed" {
		t.Fatalf("explicit headers did not reach vault login: %#v", verifier.credential.Headers)
	}
	if len(verifier.options) != 1 || !verifier.options[0].AcceptLoginAgreement {
		t.Fatalf("login agreement consent did not reach verifier: %#v", verifier.options)
	}
}

func openStores(t *testing.T) (*configstore.Store, *business.Store) {
	t.Helper()
	private, err := configstore.Open(filepath.Join(t.TempDir(), "config.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	businessStore, err := business.Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		private.Close()
		t.Fatal(err)
	}
	if err := businessStore.Bootstrap(context.Background()); err != nil {
		private.Close()
		businessStore.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { private.Close(); businessStore.Close() })
	return private, businessStore
}
