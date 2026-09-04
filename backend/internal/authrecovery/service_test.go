package authrecovery

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamauth"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamconfig"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamdetect"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

func TestSafeReasonTruncatesAtValidUTF8Boundary(t *testing.T) {
	reason := safeReason(strings.Repeat("a", 499) + "界tail")
	if len(reason) > 500 || !utf8.ValidString(reason) {
		t.Fatalf("safe reason is not valid UTF-8 within the byte limit: len=%d value=%q", len(reason), reason)
	}
	if reason != strings.Repeat("a", 499) {
		t.Fatalf("safe reason split a multibyte rune: len=%d value=%q", len(reason), reason)
	}
}

type recoveryRepository struct {
	outcomes []business.AuthRecoveryOutcome
	seeds    map[string]business.UpstreamAuthSeed
}

func (r *recoveryRepository) UpstreamAuthSeed(_ context.Context, host string) (*business.UpstreamAuthSeed, error) {
	seed, found := r.seeds[host]
	if !found {
		return nil, nil
	}
	return &seed, nil
}

func (r *recoveryRepository) PersistAuthRecoveryOutcomes(_ context.Context, values []business.AuthRecoveryOutcome, _ string) (business.AuthRecoverySummary, error) {
	r.outcomes = append([]business.AuthRecoveryOutcome{}, values...)
	summary := business.AuthRecoverySummary{Hosts: len(values)}
	for _, item := range values {
		if item.Success {
			summary.Recovered++
		} else {
			summary.Failed++
		}
	}
	return summary, nil
}

type recoveryPrivate struct {
	mu          sync.Mutex
	record      *configstore.AuthRecord
	records     map[string]configstore.AuthRecord
	vault       map[string]configstore.VaultEntry
	vaultReads  []string
	saved       bool
	preferences map[string]configstore.AuthRecoveryPreference
}

func (s *recoveryPrivate) AuthRecoveryPreference(_ context.Context, host string) (*configstore.AuthRecoveryPreference, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, found := s.preferences[configstore.CanonicalHost(host)]
	if !found {
		return nil, nil
	}
	copy := value
	return &copy, nil
}

func (s *recoveryPrivate) SaveAuthRecoveryPreference(_ context.Context, value configstore.AuthRecoveryPreference) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.preferences == nil {
		s.preferences = map[string]configstore.AuthRecoveryPreference{}
	}
	if value.VaultEntry == nil {
		if current, found := s.preferences[configstore.CanonicalHost(value.Host)]; found {
			value.VaultEntry = current.VaultEntry
		}
	}
	s.preferences[configstore.CanonicalHost(value.Host)] = value
	return nil
}

func (s *recoveryPrivate) AuthRecord(_ context.Context, host string) (*configstore.AuthRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if record, found := s.records[configstore.CanonicalHost(host)]; found {
		copy := record
		return &copy, nil
	}
	if s.record == nil {
		return nil, nil
	}
	copy := *s.record
	return &copy, nil
}
func (s *recoveryPrivate) AuthRecordIndex(context.Context) ([]configstore.AuthRecordSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]configstore.AuthRecordSummary, 0, len(s.records)+1)
	for _, value := range s.records {
		result = append(result, configstore.AuthRecordSummary{
			Host: value.Host, BaseURL: value.BaseURL, UpstreamType: value.UpstreamType, AuthMode: value.AuthMode,
		})
	}
	if s.record != nil {
		result = append(result, configstore.AuthRecordSummary{
			Host: s.record.Host, BaseURL: s.record.BaseURL, UpstreamType: s.record.UpstreamType, AuthMode: s.record.AuthMode,
		})
	}
	return result, nil
}
func (s *recoveryPrivate) VaultEntryIndex(context.Context) ([]configstore.VaultEntrySummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]configstore.VaultEntrySummary, 0, len(s.vault))
	for _, value := range s.vault {
		result = append(result, configstore.VaultEntrySummary{Entry: value.Entry, Hosts: append([]string{}, value.Hosts...)})
	}
	return result, nil
}
func (s *recoveryPrivate) SaveAuthRecord(_ context.Context, record configstore.AuthRecord, _ map[string]bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.record, s.saved = &record, true
	return nil
}
func (s *recoveryPrivate) VaultEntry(_ context.Context, entry string) (*configstore.VaultEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vaultReads = append(s.vaultReads, entry)
	value, found := s.vault[entry]
	if !found {
		return nil, nil
	}
	return &value, nil
}

type recoveryAuthenticator struct {
	refresh      func(context.Context, configstore.AuthRecord) (configstore.AuthRecord, error)
	login        func(context.Context, configstore.AuthRecord, configstore.VaultEntry) (configstore.AuthRecord, error)
	loginOptions []upstreamauth.LoginOptions
}

func (a *recoveryAuthenticator) Refresh(ctx context.Context, record configstore.AuthRecord) (configstore.AuthRecord, error) {
	return a.refresh(ctx, record)
}
func (a *recoveryAuthenticator) Login(ctx context.Context, record configstore.AuthRecord, entry configstore.VaultEntry) (configstore.AuthRecord, error) {
	return a.login(ctx, record, entry)
}
func (a *recoveryAuthenticator) LoginWithOptions(ctx context.Context, record configstore.AuthRecord, entry configstore.VaultEntry, options upstreamauth.LoginOptions) (configstore.AuthRecord, error) {
	a.loginOptions = append(a.loginOptions, options)
	return a.login(ctx, record, entry)
}

type recoveryConfigurator struct {
	input     upstreamconfig.Input
	host      string
	err       error
	created   bool
	committed *configstore.AuthRecord
	onCreate  func(upstreamconfig.Input)
}

func (c *recoveryConfigurator) Create(_ context.Context, input upstreamconfig.Input, _ string) (upstreamconfig.Configuration, error) {
	c.input, c.created = input, true
	if c.onCreate != nil {
		c.onCreate(input)
	}
	return upstreamconfig.Configuration{Host: configstore.CanonicalHost(input.Host)}, c.err
}

func (c *recoveryConfigurator) ConfigureAuthRecord(_ context.Context, input upstreamconfig.Input) (string, error) {
	c.input = input
	return c.host, c.err
}

func (c *recoveryConfigurator) CommitRecoveredAuth(_ context.Context, record configstore.AuthRecord) error {
	c.committed = &record
	return c.err
}

type recoveryDetector struct {
	result upstreamdetect.Result
	err    error
}

func (d recoveryDetector) Detect(context.Context, string) (upstreamdetect.Result, error) {
	return d.result, d.err
}

type recoveryBalance struct {
	calls int
	check func() error
}

func (b *recoveryBalance) SyncHost(_ context.Context, host string, _ upstreamsync.Scope, _ string) (upstreamsync.HostResult, error) {
	b.calls++
	if b.check != nil {
		if err := b.check(); err != nil {
			return upstreamsync.HostResult{}, err
		}
	}
	balance := "2"
	return upstreamsync.HostResult{Host: host, Status: "succeeded", BalanceStatus: "已读取", Balance: &balance}, nil
}

func TestBalanceResultPreservesUpstreamDisplayCurrency(t *testing.T) {
	balance, displayBalance, unit := "14.131496", "103.1599208", "cny"

	result := balanceResult(upstreamsync.HostResult{
		Status: "succeeded", BalanceStatus: "已读取", Balance: &balance,
		DisplayBalance: &displayBalance, BalanceUnit: &unit,
	}, nil)

	if result.Balance == nil || *result.Balance != balance ||
		result.DisplayBalance == nil || *result.DisplayBalance != displayBalance ||
		result.BalanceUnit == nil || *result.BalanceUnit != unit {
		t.Fatalf("display currency was not preserved: %#v", result)
	}
}

type recoveryTasks struct {
	done chan taskstore.Task
}

type captchaTasks struct {
	mu      sync.Mutex
	items   map[string]taskstore.Task
	updates chan taskstore.Task
}

func (s *captchaTasks) Save(_ context.Context, task taskstore.Task) error {
	s.mu.Lock()
	s.items[task.ID] = task
	s.mu.Unlock()
	select {
	case s.updates <- task:
	default:
	}
	return nil
}

func (s *captchaTasks) Get(_ context.Context, id string) (taskstore.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, found := s.items[id]
	if !found {
		return taskstore.Task{}, taskstore.ErrNotFound
	}
	return task, nil
}

type recoveryCaptcha struct {
	parent            *string
	manualCredential  *configstore.VaultEntry
	manualSaveToVault bool
}

func (c *recoveryCaptcha) Prepare(_ context.Context, record configstore.AuthRecord, entry string, parent *string) (CaptchaChallenge, error) {
	c.parent = parent
	return CaptchaChallenge{
		ChallengeID: "challenge-1", Host: record.Host, ImageData: "data:image/png;base64,cG5n", ExpiresAt: time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano),
		Credential: map[string]string{"entry": entry}, InteractionKind: "image_captcha_ocr",
	}, nil
}

func (c *recoveryCaptcha) PrepareCredential(_ context.Context, record configstore.AuthRecord, credential configstore.VaultEntry, saveToVault bool, parent *string) (CaptchaChallenge, error) {
	c.parent = parent
	c.manualCredential = &credential
	c.manualSaveToVault = saveToVault
	return CaptchaChallenge{
		ChallengeID: "manual-challenge", Host: record.Host, ImageData: "data:image/png;base64,cG5n", ExpiresAt: time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano),
		Credential: map[string]string{"entry": credential.Entry}, InteractionKind: "image_captcha_ocr",
	}, nil
}

func (c *recoveryCaptcha) Submit(context.Context, string, string) (CaptchaResult, error) {
	return CaptchaResult{
		Success: true, Host: "api.example", ProfileStatus: "verified", Keys: 1, Groups: 2, Stored: true,
		InteractionKind: "image_captcha_ocr", ParentTaskID: c.parent,
	}, nil
}

func (c *recoveryCaptcha) Cancel(string) (*CaptchaChallenge, *string) {
	return &CaptchaChallenge{Host: "api.example"}, c.parent
}

func (s *recoveryTasks) Save(_ context.Context, task taskstore.Task) error {
	if task.Status == "succeeded" || task.Status == "failed" {
		select {
		case s.done <- task:
		default:
		}
	}
	return nil
}

func TestRecoveryUsesRefreshBeforeVaultAndCommitsBeforeBalanceRead(t *testing.T) {
	access, refresh, rotated := "expired", "refresh", "rotated"
	private := &recoveryPrivate{record: &configstore.AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token",
		AccessToken: &access, RefreshToken: &refresh, Headers: map[string]string{}, Cookies: map[string]string{},
	}, vault: map[string]configstore.VaultEntry{"selected": {Entry: "selected"}}}
	auth := &recoveryAuthenticator{
		refresh: func(_ context.Context, record configstore.AuthRecord) (configstore.AuthRecord, error) {
			record.AccessToken = &rotated
			return record, nil
		},
		login: func(context.Context, configstore.AuthRecord, configstore.VaultEntry) (configstore.AuthRecord, error) {
			return configstore.AuthRecord{}, errors.New("vault must not be used after refresh succeeds")
		},
	}
	repository := &recoveryRepository{}
	balance := &recoveryBalance{check: func() error {
		guardCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, release, err := mutationguard.Acquire(guardCtx, repository, mutationguard.Upstream("HTTPS://API.EXAMPLE/"))
		if err != nil {
			return errors.New("balance sync started before the recovery lease was released")
		}
		defer release()
		private.mu.Lock()
		defer private.mu.Unlock()
		if !private.saved || private.record.AccessToken == nil || *private.record.AccessToken != "rotated" {
			return errors.New("balance read happened before credential commit")
		}
		return nil
	}}
	tasks := &recoveryTasks{done: make(chan taskstore.Task, 2)}
	service := New(repository, private, auth, &recoveryConfigurator{}, balance, tasks)
	if _, err := service.Enqueue(context.Background(), "api.example", "selected", false, "tester"); err != nil {
		t.Fatal(err)
	}
	var task taskstore.Task
	select {
	case task = <-tasks.done:
	case <-time.After(time.Second):
		t.Fatal("recovery task did not finish")
	}
	if task.Status != "succeeded" || balance.calls != 1 || len(repository.outcomes) != 1 || !repository.outcomes[0].Success {
		t.Fatalf("task=%#v balance=%d outcomes=%#v", task, balance.calls, repository.outcomes)
	}
	if len(private.vaultReads) != 1 { // enqueue validation only; execution never enumerates or rereads on refresh success.
		t.Fatalf("vault reads=%#v", private.vaultReads)
	}
}

func TestRecoveryDoesNotRestoreCredentialsAfterCanonicalUpstreamDelete(t *testing.T) {
	access, refresh, rotated := "expired", "refresh", "rotated"
	record := configstore.AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token",
		AccessToken: &access, RefreshToken: &refresh, Headers: map[string]string{}, Cookies: map[string]string{},
	}
	private := &recoveryPrivate{record: &record, vault: map[string]configstore.VaultEntry{}}
	repository := &recoveryRepository{seeds: map[string]business.UpstreamAuthSeed{
		"api.example": {Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api"},
	}}
	refreshStarted := make(chan struct{})
	finishRefresh := make(chan struct{})
	auth := &recoveryAuthenticator{
		refresh: func(_ context.Context, candidate configstore.AuthRecord) (configstore.AuthRecord, error) {
			close(refreshStarted)
			<-finishRefresh
			candidate.AccessToken = &rotated
			return candidate, nil
		},
		login: func(context.Context, configstore.AuthRecord, configstore.VaultEntry) (configstore.AuthRecord, error) {
			return configstore.AuthRecord{}, errors.New("vault login must not run")
		},
	}
	service := New(repository, private, auth, &recoveryConfigurator{}, &recoveryBalance{}, &recoveryTasks{done: make(chan taskstore.Task, 1)})
	outcomes := make(chan business.AuthRecoveryOutcome, 1)
	go func() {
		outcomes <- service.recover(context.Background(), record, "", false)
	}()
	<-refreshStarted

	deleteCtx, cancelDelete := context.WithTimeout(context.Background(), time.Second)
	_, releaseDelete, err := mutationguard.Acquire(deleteCtx, repository, mutationguard.Upstream("HTTPS://API.EXAMPLE/"))
	cancelDelete()
	if err != nil {
		close(finishRefresh)
		t.Fatalf("delete could not acquire while recovery was doing remote verification: %v", err)
	}
	private.mu.Lock()
	private.record = nil
	private.saved = false
	private.mu.Unlock()
	delete(repository.seeds, "api.example")
	if err := releaseDelete(); err != nil {
		close(finishRefresh)
		t.Fatal(err)
	}
	close(finishRefresh)
	outcome := <-outcomes
	if outcome.Success || pointerOr(outcome.Code, "") != "credential_commit_failed" {
		t.Fatalf("deleted upstream recovery outcome = %#v", outcome)
	}
	private.mu.Lock()
	defer private.mu.Unlock()
	if private.saved || private.record != nil {
		t.Fatalf("recovery resurrected deleted credentials: record=%#v saved=%v", private.record, private.saved)
	}
}

func TestRecoveryCorrectsStoredPlatformAfterVerifiedPublicFingerprint(t *testing.T) {
	username, password, access, refresh := "user", "password", "expired", "refresh"
	private := &recoveryPrivate{record: &configstore.AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_login",
		AccessToken: &access, RefreshToken: &refresh, Headers: map[string]string{}, Cookies: map[string]string{},
	}, vault: map[string]configstore.VaultEntry{
		"selected": {Entry: "selected", Username: &username, Password: &password},
	}}
	auth := &recoveryAuthenticator{
		refresh: func(_ context.Context, record configstore.AuthRecord) (configstore.AuthRecord, error) {
			if record.UpstreamType != "newapi" || record.AuthMode != "newapi_user_login" {
				t.Fatalf("refresh used stale classification: %#v", record)
			}
			return configstore.AuthRecord{}, errors.New("refresh session expired")
		},
		login: func(_ context.Context, record configstore.AuthRecord, _ configstore.VaultEntry) (configstore.AuthRecord, error) {
			if record.UpstreamType != "newapi" || record.AuthMode != "newapi_user_login" {
				t.Fatalf("login used stale classification: %#v", record)
			}
			record.AccessToken = &access
			return record, nil
		},
	}
	platform := "newapi"
	configurator := &recoveryConfigurator{}
	service := New(&recoveryRepository{}, private, auth, configurator, &recoveryBalance{}, &recoveryTasks{done: make(chan taskstore.Task, 1)})
	service.UsePlatformDetector(recoveryDetector{result: upstreamdetect.Result{
		BaseURL: "https://api.example", Host: "api.example", UpstreamType: &platform, TypeDetected: true,
	}})

	outcome := service.recover(context.Background(), *private.record, "selected", false)
	if !outcome.Success || configurator.committed == nil {
		t.Fatalf("outcome=%#v committed=%#v", outcome, configurator.committed)
	}
	if configurator.committed.UpstreamType != "newapi" || configurator.committed.AuthMode != "newapi_user_login" {
		t.Fatalf("incorrect committed classification: %#v", configurator.committed)
	}
	if private.saved {
		t.Fatal("classification repair bypassed the coordinated committer")
	}
}

func TestRecoveryUsesOnlyExplicitlySelectedVaultEntryAfterRefreshFailure(t *testing.T) {
	username, password, refresh := "user", "password", "refresh"
	private := &recoveryPrivate{record: &configstore.AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token",
		RefreshToken: &refresh, Headers: map[string]string{}, Cookies: map[string]string{},
	}, vault: map[string]configstore.VaultEntry{
		"selected": {Entry: "selected", Username: &username, Password: &password},
		"other":    {Entry: "other", Username: &username, Password: &password},
	}}
	var loginEntries []string
	auth := &recoveryAuthenticator{
		refresh: func(context.Context, configstore.AuthRecord) (configstore.AuthRecord, error) {
			return configstore.AuthRecord{}, errors.New("HTTP 401")
		},
		login: func(_ context.Context, record configstore.AuthRecord, entry configstore.VaultEntry) (configstore.AuthRecord, error) {
			loginEntries = append(loginEntries, entry.Entry)
			token := "logged-in"
			record.AccessToken = &token
			return record, nil
		},
	}
	service := New(&recoveryRepository{}, private, auth, &recoveryConfigurator{}, &recoveryBalance{}, &recoveryTasks{done: make(chan taskstore.Task, 2)})
	if _, err := service.Enqueue(context.Background(), "api.example", "selected", false, "tester"); err != nil {
		t.Fatal(err)
	}
	select {
	case task := <-service.tasks.(*recoveryTasks).done:
		if task.Status != "succeeded" {
			t.Fatalf("task=%#v", task)
		}
	case <-time.After(time.Second):
		t.Fatal("recovery task did not finish")
	}
	if len(loginEntries) != 1 || loginEntries[0] != "selected" {
		t.Fatalf("vault enumeration occurred: %#v", loginEntries)
	}
}

func TestRecoveryCreatesMissingPrivateRecordFromBusinessHostAfterVerifiedVaultLogin(t *testing.T) {
	username, password := "operator", "secret"
	repository := &recoveryRepository{seeds: map[string]business.UpstreamAuthSeed{
		"api.example": {Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api"},
	}}
	private := &recoveryPrivate{vault: map[string]configstore.VaultEntry{
		"selected": {Entry: "selected", Username: &username, Password: &password},
	}}
	auth := &recoveryAuthenticator{
		refresh: func(context.Context, configstore.AuthRecord) (configstore.AuthRecord, error) {
			return configstore.AuthRecord{}, errors.New("refresh must not run without a stored token")
		},
		login: func(_ context.Context, record configstore.AuthRecord, entry configstore.VaultEntry) (configstore.AuthRecord, error) {
			if record.BaseURL != "https://api.example" || record.AuthMode != "sub2api_user_login" || entry.Entry != "selected" {
				return configstore.AuthRecord{}, errors.New("recovery candidate is incomplete")
			}
			token, refresh := "fresh", "rotated"
			record.AuthMode, record.AccessToken, record.RefreshToken = "sub2api_user_token", &token, &refresh
			return record, nil
		},
	}
	tasks := &recoveryTasks{done: make(chan taskstore.Task, 2)}
	service := New(repository, private, auth, &recoveryConfigurator{}, &recoveryBalance{}, tasks)
	if _, err := service.Enqueue(context.Background(), "api.example", "selected", false, "tester"); err != nil {
		t.Fatal(err)
	}
	select {
	case task := <-tasks.done:
		if task.Status != "succeeded" {
			t.Fatalf("task=%#v", task)
		}
	case <-time.After(time.Second):
		t.Fatal("recovery task did not finish")
	}
	if !private.saved || private.record == nil || private.record.AccessToken == nil || *private.record.AccessToken != "fresh" {
		t.Fatalf("verified credentials were not persisted: %#v", private.record)
	}
}

func TestRecoveryDoesNotCreateMissingPrivateRecordWhenVaultLoginFails(t *testing.T) {
	username, password := "operator", "wrong"
	repository := &recoveryRepository{seeds: map[string]business.UpstreamAuthSeed{
		"api.example": {Host: "api.example", BaseURL: "https://api.example", UpstreamType: "newapi"},
	}}
	private := &recoveryPrivate{vault: map[string]configstore.VaultEntry{
		"selected": {Entry: "selected", Username: &username, Password: &password},
	}}
	auth := &recoveryAuthenticator{
		refresh: func(context.Context, configstore.AuthRecord) (configstore.AuthRecord, error) {
			return configstore.AuthRecord{}, errors.New("unexpected refresh")
		},
		login: func(context.Context, configstore.AuthRecord, configstore.VaultEntry) (configstore.AuthRecord, error) {
			return configstore.AuthRecord{}, errors.New("HTTP 401")
		},
	}
	tasks := &recoveryTasks{done: make(chan taskstore.Task, 2)}
	service := New(repository, private, auth, &recoveryConfigurator{}, &recoveryBalance{}, tasks)
	if _, err := service.Enqueue(context.Background(), "api.example", "selected", false, "tester"); err != nil {
		t.Fatal(err)
	}
	select {
	case task := <-tasks.done:
		if task.Status != "failed" {
			t.Fatalf("task=%#v", task)
		}
	case <-time.After(time.Second):
		t.Fatal("recovery task did not finish")
	}
	if private.saved || private.record != nil {
		t.Fatalf("failed login persisted candidate credentials: %#v", private.record)
	}
}

func TestRecoveryRejectsHostMissingFromPrivateAndBusinessStores(t *testing.T) {
	service := New(
		&recoveryRepository{seeds: map[string]business.UpstreamAuthSeed{}},
		&recoveryPrivate{vault: map[string]configstore.VaultEntry{}},
		&recoveryAuthenticator{}, &recoveryConfigurator{}, &recoveryBalance{},
		&recoveryTasks{done: make(chan taskstore.Task, 1)},
	)
	_, err := service.Enqueue(context.Background(), "missing.example", "selected", false, "tester")
	if err == nil || !strings.Contains(err.Error(), "上游 Host 不存在") {
		t.Fatalf("expected missing host error, got %v", err)
	}
}

func TestResolveAuthUsesUniqueExplicitVaultHostAndRelatedMetadata(t *testing.T) {
	username, password := "operator", "secret"
	const host = "192.0.2.44:8080"
	private := &recoveryPrivate{
		records: map[string]configstore.AuthRecord{
			"192.0.2.44": {
				Host: "192.0.2.44", BaseURL: "https://accelerated.example.test", UpstreamType: "sub2api",
				AuthMode: "sub2api_user_token", Headers: map[string]string{}, Cookies: map[string]string{},
			},
			"another.example.test": {
				Host: "another.example.test", BaseURL: "https://other-cdn.example.test", UpstreamType: "newapi",
				AuthMode: "newapi_user_token", Headers: map[string]string{}, Cookies: map[string]string{},
			},
		},
		vault: map[string]configstore.VaultEntry{
			"shared": {Entry: "shared", Username: &username, Password: &password, Hosts: []string{host, "accelerated.example.test", "other-cdn.example.test"}},
		},
	}
	configurator := &recoveryConfigurator{onCreate: func(input upstreamconfig.Input) {
		private.records[host] = configstore.AuthRecord{
			Host: host, BaseURL: input.BaseURL, UpstreamType: input.UpstreamType, AuthMode: "sub2api_user_token",
			Headers: map[string]string{}, Cookies: map[string]string{},
		}
	}}
	service := New(&recoveryRepository{seeds: map[string]business.UpstreamAuthSeed{}}, private, &recoveryAuthenticator{}, configurator, &recoveryBalance{}, &recoveryTasks{done: make(chan taskstore.Task, 1)})

	record, err := service.ResolveAuth(context.Background(), host, "tester")
	if err != nil {
		t.Fatal(err)
	}
	if !configurator.created || configurator.input.BaseURL != "https://accelerated.example.test" || configurator.input.AuthMode != "sub2api_user_login" {
		t.Fatalf("input=%#v created=%v", configurator.input, configurator.created)
	}
	if record == nil || record.Host != host {
		t.Fatalf("record=%#v", record)
	}
}

func TestResolveAuthRejectsAmbiguousVaultMatches(t *testing.T) {
	const host = "192.0.2.44:8080"
	private := &recoveryPrivate{vault: map[string]configstore.VaultEntry{
		"first":  {Entry: "first", Hosts: []string{host}},
		"second": {Entry: "second", Hosts: []string{host}},
	}}
	service := New(&recoveryRepository{seeds: map[string]business.UpstreamAuthSeed{}}, private, &recoveryAuthenticator{}, &recoveryConfigurator{}, &recoveryBalance{}, &recoveryTasks{done: make(chan taskstore.Task, 1)})

	_, err := service.ResolveAuth(context.Background(), host, "tester")
	if err == nil || !strings.Contains(err.Error(), "匹配到多个密码箱项") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestManualVerificationBuildsPlatformCredentialPatchAndReturnsBalance(t *testing.T) {
	baseToken := "old"
	private := &recoveryPrivate{record: &configstore.AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "newapi", AuthMode: "newapi_user_token",
		AccessToken: &baseToken, Headers: map[string]string{"Authorization": "Bearer old", "X-Site": "custom"}, Cookies: map[string]string{},
	}}
	configurator := &recoveryConfigurator{host: "api.example"}
	balance := &recoveryBalance{}
	adminKey, userID, mode := "admin", "7", "newapi_admin_key"
	service := New(&recoveryRepository{}, private, &recoveryAuthenticator{}, configurator, balance, &recoveryTasks{done: make(chan taskstore.Task, 1)})
	result, err := service.VerifyManual(context.Background(), ManualInput{
		Host: "api.example", AuthMode: &mode, AdminKey: &adminKey, UserID: &userID,
		AcceptLoginAgreement: true,
		Present:              map[string]bool{"auth_mode": true, "admin_key": true, "user_id": true},
	}, "tester")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Verified || result.BalanceSync.Status != "succeeded" || configurator.input.AdminKey == nil || *configurator.input.AdminKey != "admin" {
		t.Fatalf("result=%#v input=%#v", result, configurator.input)
	}
	if !configurator.input.AcceptLoginAgreement {
		t.Fatal("explicit login agreement consent was not passed to the configurator")
	}
	if _, present := configurator.input.Headers["Authorization"]; present || configurator.input.Headers["X-Site"] != "custom" {
		t.Fatalf("stale bearer header survived: %#v", configurator.input.Headers)
	}
	if !configurator.input.Present["access_token"] || !configurator.input.Present["refresh_token"] || !configurator.input.Present["headers"] {
		t.Fatalf("credential clears missing: %#v", configurator.input.Present)
	}
}

func TestManualVerificationPreservesStoredAuthorizationHeaderWhenCredentialsAreOmitted(t *testing.T) {
	private := &recoveryPrivate{record: &configstore.AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token",
		Headers: map[string]string{"Authorization": "Bearer header-token", "X-Site": "custom"}, Cookies: map[string]string{},
	}}
	configurator := &recoveryConfigurator{host: "api.example"}
	service := New(&recoveryRepository{}, private, &recoveryAuthenticator{}, configurator, &recoveryBalance{}, &recoveryTasks{done: make(chan taskstore.Task, 1)})
	mode := "sub2api_user_token"
	result, err := service.VerifyManual(context.Background(), ManualInput{
		Host: "api.example", AuthMode: &mode, Present: map[string]bool{"auth_mode": true},
	}, "tester")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Verified || configurator.input.Headers["Authorization"] != "Bearer header-token" {
		t.Fatalf("stored header authentication was not preserved: result=%#v input=%#v", result, configurator.input)
	}
	if configurator.input.Present["headers"] {
		t.Fatalf("unchanged headers were unexpectedly replaced: %#v", configurator.input.Present)
	}
}

func TestManualVerificationUsesExplicitHeadersWithoutRemovingAuthorization(t *testing.T) {
	private := &recoveryPrivate{record: &configstore.AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "newapi", AuthMode: "newapi_admin_key",
		Headers: map[string]string{"Authorization": "Bearer stored"}, Cookies: map[string]string{},
	}}
	configurator := &recoveryConfigurator{host: "api.example"}
	service := New(&recoveryRepository{}, private, &recoveryAuthenticator{}, configurator, &recoveryBalance{}, &recoveryTasks{done: make(chan taskstore.Task, 1)})
	mode, adminKey, userID := "newapi_admin_key", "admin", "24"
	_, err := service.VerifyManual(context.Background(), ManualInput{
		Host: "api.example", AuthMode: &mode, AdminKey: &adminKey, UserID: &userID,
		Headers: map[string]string{"Authorization": "Bearer explicit", "X-CF-Access": "signed"},
		Present: map[string]bool{"auth_mode": true, "admin_key": true, "user_id": true, "headers": true},
	}, "tester")
	if err != nil {
		t.Fatal(err)
	}
	if configurator.input.Headers["Authorization"] != "Bearer explicit" || configurator.input.Headers["X-CF-Access"] != "signed" || !configurator.input.Present["headers"] {
		t.Fatalf("explicit headers were not preserved: %#v", configurator.input)
	}
}

func TestManualPasswordLoginReturnsCaptchaChallengeWithoutPersistingCredentials(t *testing.T) {
	private := &recoveryPrivate{record: &configstore.AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token",
		Headers: map[string]string{"X-Site": "custom"}, Cookies: map[string]string{},
	}}
	interaction := &upstreamauth.InteractionError{Code: "image_captcha_required", Detail: "登录需要图片验证码"}
	configurator := &recoveryConfigurator{host: "api.example", err: interaction}
	captcha := &recoveryCaptcha{}
	service := New(&recoveryRepository{}, private, &recoveryAuthenticator{}, configurator, &recoveryBalance{}, &recoveryTasks{done: make(chan taskstore.Task, 1)}, captcha)
	mode, username, password, entry := "sub2api_manual_login", "operator", "secret", "operator-entry"
	result, err := service.VerifyManual(context.Background(), ManualInput{
		Host: "api.example", AuthMode: &mode, Username: &username, Password: &password, SaveToVault: true, Entry: &entry,
		Present: map[string]bool{"auth_mode": true, "username": true, "password": true, "save_to_vault": true, "entry": true},
	}, "tester")
	if err != nil {
		t.Fatal(err)
	}
	if result.Verified || result.CaptchaChallenge == nil || result.CaptchaChallenge.ChallengeID != "manual-challenge" {
		t.Fatalf("result=%#v", result)
	}
	if captcha.manualCredential == nil || captcha.manualCredential.Entry != entry || captcha.manualCredential.Username == nil || *captcha.manualCredential.Username != username || !captcha.manualSaveToVault {
		t.Fatalf("captcha credential=%#v save=%v", captcha.manualCredential, captcha.manualSaveToVault)
	}
	if private.saved || len(private.vaultReads) != 0 {
		t.Fatalf("manual credentials were persisted or vault was read before captcha: saved=%v reads=%#v", private.saved, private.vaultReads)
	}
}

func TestRecoveryTaskDoesNotExposeCredentialInFailure(t *testing.T) {
	refresh := "refresh-secret"
	private := &recoveryPrivate{record: &configstore.AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", RefreshToken: &refresh, Headers: map[string]string{}, Cookies: map[string]string{},
	}, vault: map[string]configstore.VaultEntry{}}
	tasks := &recoveryTasks{done: make(chan taskstore.Task, 2)}
	auth := &recoveryAuthenticator{refresh: func(context.Context, configstore.AuthRecord) (configstore.AuthRecord, error) {
		return configstore.AuthRecord{}, errors.New("refresh_token=refresh-secret")
	}, login: func(context.Context, configstore.AuthRecord, configstore.VaultEntry) (configstore.AuthRecord, error) {
		return configstore.AuthRecord{}, errors.New("unused")
	}}
	service := New(&recoveryRepository{}, private, auth, &recoveryConfigurator{}, &recoveryBalance{}, tasks)
	if _, err := service.Enqueue(context.Background(), "api.example", "", false, "tester"); err != nil {
		t.Fatal(err)
	}
	var task taskstore.Task
	select {
	case task = <-tasks.done:
	case <-time.After(time.Second):
		t.Fatal("task did not finish")
	}
	encoded, _ := json.Marshal(task)
	if strings.Contains(string(encoded), "refresh-secret") {
		t.Fatalf("task leaked credential: %s", encoded)
	}
}

func TestImageCaptchaWaitsForInputThenCompletesParentTask(t *testing.T) {
	username, password := "operator", "secret"
	private := &recoveryPrivate{record: &configstore.AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token",
		Headers: map[string]string{}, Cookies: map[string]string{},
	}, vault: map[string]configstore.VaultEntry{
		"selected": {Entry: "selected", Username: &username, Password: &password},
	}}
	auth := &recoveryAuthenticator{
		refresh: func(context.Context, configstore.AuthRecord) (configstore.AuthRecord, error) {
			return configstore.AuthRecord{}, errors.New("no refresh")
		},
		login: func(context.Context, configstore.AuthRecord, configstore.VaultEntry) (configstore.AuthRecord, error) {
			return configstore.AuthRecord{}, &upstreamauth.InteractionError{Code: "image_captcha_required", Detail: "需要图片验证码"}
		},
	}
	tasks := &captchaTasks{items: map[string]taskstore.Task{}, updates: make(chan taskstore.Task, 16)}
	captcha := &recoveryCaptcha{}
	repository, balance := &recoveryRepository{}, &recoveryBalance{}
	service := New(repository, private, auth, &recoveryConfigurator{}, balance, tasks, captcha)
	queued, err := service.Enqueue(context.Background(), "api.example", "selected", false, "tester")
	if err != nil {
		t.Fatal(err)
	}
	waiting := waitForTaskStatus(t, tasks.updates, "waiting_input")
	challenge, ok := waiting.Result["captcha_challenge"].(CaptchaChallenge)
	if !ok || challenge.ChallengeID != "challenge-1" || waiting.Progress != 90 {
		t.Fatalf("task did not expose a public challenge: %#v", waiting)
	}
	completion, err := service.SubmitCaptcha(context.Background(), "challenge-1", "AB12", "tester")
	if err != nil {
		t.Fatal(err)
	}
	if !completion.Success || completion.BalanceSync.Status != "succeeded" {
		t.Fatalf("unexpected completion: %#v", completion)
	}
	finished, err := tasks.Get(context.Background(), queued.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != "succeeded" || finished.Progress != 100 || finished.Result["credentials_persisted"] != true {
		t.Fatalf("parent task was not completed: %#v", finished)
	}
	if len(repository.outcomes) != 1 || !repository.outcomes[0].Success || balance.calls != 1 {
		t.Fatalf("projection or balance sync missing: outcomes=%#v calls=%d", repository.outcomes, balance.calls)
	}
}

func TestRecoveryWithoutExplicitEntryUsesLastSuccessfulVaultEntry(t *testing.T) {
	refresh := "expired"
	entry := "Primary"
	record := configstore.AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", RefreshToken: &refresh,
	}
	private := &recoveryPrivate{
		record: &record,
		vault:  map[string]configstore.VaultEntry{entry: {Entry: entry}},
		preferences: map[string]configstore.AuthRecoveryPreference{
			"api.example": {Host: "api.example", AuthMode: "sub2api_user_login", RecoveryMethod: "vault", VaultEntry: &entry},
		},
	}
	auth := &recoveryAuthenticator{
		refresh: func(context.Context, configstore.AuthRecord) (configstore.AuthRecord, error) {
			return configstore.AuthRecord{}, errors.New("refresh expired")
		},
		login: func(_ context.Context, current configstore.AuthRecord, _ configstore.VaultEntry) (configstore.AuthRecord, error) {
			current.AuthMode = "sub2api_user_login"
			return current, nil
		},
	}
	service := New(&recoveryRepository{}, private, auth, &recoveryConfigurator{}, &recoveryBalance{}, &recoveryTasks{done: make(chan taskstore.Task, 1)})
	outcome := service.recover(context.Background(), record, "", false)
	if !outcome.Success || outcome.AuthMethod == nil || *outcome.AuthMethod != "sub2api_user_login" || len(private.vaultReads) != 2 || private.vaultReads[0] != entry || private.vaultReads[1] != entry {
		t.Fatalf("outcome=%#v vaultReads=%#v", outcome, private.vaultReads)
	}
	preference := private.preferences["api.example"]
	if preference.RecoveryMethod != "vault" || preference.VaultEntry == nil || *preference.VaultEntry != entry {
		t.Fatalf("preference=%#v", preference)
	}
}

func TestRecoverInvalidDeduplicatesHostsAndRecordsSuccessfulMethods(t *testing.T) {
	refresh := "refresh"
	private := &recoveryPrivate{records: map[string]configstore.AuthRecord{
		"one.example": {Host: "one.example", BaseURL: "https://one.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RefreshToken: &refresh},
		"two.example": {Host: "two.example", BaseURL: "https://two.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RefreshToken: &refresh},
	}}
	auth := &recoveryAuthenticator{
		refresh: func(_ context.Context, record configstore.AuthRecord) (configstore.AuthRecord, error) {
			return record, nil
		},
		login: func(context.Context, configstore.AuthRecord, configstore.VaultEntry) (configstore.AuthRecord, error) {
			return configstore.AuthRecord{}, errors.New("unexpected login")
		},
	}
	repository := &recoveryRepository{}
	service := New(repository, private, auth, &recoveryConfigurator{}, &recoveryBalance{}, &recoveryTasks{done: make(chan taskstore.Task, 1)})
	summary, err := service.RecoverInvalid(context.Background(), []string{"one.example", "ONE.EXAMPLE", "two.example"}, "自动巡检")
	if err != nil || summary.Hosts != 2 || summary.Recovered != 2 || len(repository.outcomes) != 2 {
		t.Fatalf("summary=%#v outcomes=%#v err=%v", summary, repository.outcomes, err)
	}
	for _, outcome := range repository.outcomes {
		if outcome.AuthMethod == nil || *outcome.AuthMethod != "sub2api_user_token" || outcome.RefreshKind == nil || *outcome.RefreshKind != "refresh_token" {
			t.Fatalf("outcome=%#v", outcome)
		}
	}
}

func waitForTaskStatus(t *testing.T, updates <-chan taskstore.Task, status string) taskstore.Task {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for {
		select {
		case task := <-updates:
			if task.Status == status {
				return task
			}
		case <-timer.C:
			t.Fatalf("task did not reach %s", status)
		}
	}
}
