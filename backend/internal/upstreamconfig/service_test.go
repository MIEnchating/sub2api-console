package upstreamconfig

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type observingVerifier struct {
	business *business.Store
	private  *configstore.Store
	calls    int
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

func TestCreateKeepsHostIdentityIndependentFromBaseURL(t *testing.T) {
	private, businessStore := openStores(t)
	service := New(businessStore, private, &passVerifier{})
	access, refresh := "access", "refresh"
	tests := []struct {
		host    string
		baseURL string
	}{
		{host: "152.53.241.112:8080", baseURL: "https://accelerated.example.test:8443/api"},
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

type passVerifier struct{}

func (*passVerifier) Verify(context.Context, configstore.AuthRecord) error { return nil }
func (*passVerifier) Login(_ context.Context, record configstore.AuthRecord, _ configstore.VaultEntry) (configstore.AuthRecord, error) {
	return record, nil
}

type capturingLoginVerifier struct {
	credential configstore.VaultEntry
}

func (*capturingLoginVerifier) Verify(context.Context, configstore.AuthRecord) error { return nil }
func (v *capturingLoginVerifier) Login(_ context.Context, record configstore.AuthRecord, credential configstore.VaultEntry) (configstore.AuthRecord, error) {
	v.credential = credential
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
	err = service.commitPrivateAndPublic(context.Background(), record, Input{
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
		Present: map[string]bool{"entry": true, "headers": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if verifier.credential.Headers["Authorization"] != "Bearer gateway" || verifier.credential.Headers["X-CF-Access"] != "signed" {
		t.Fatalf("explicit headers did not reach vault login: %#v", verifier.credential.Headers)
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
