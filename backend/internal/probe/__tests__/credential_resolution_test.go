package probe_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/probe"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

type credentialSettings struct {
	*configstore.Store
	endpoint string
}

func (settings credentialSettings) TargetSettings(context.Context) (configstore.TargetSettings, error) {
	return configstore.TargetSettings{BaseURL: settings.endpoint, AdminKey: "test-admin-key", TimeoutSeconds: 2}, nil
}

type credentialFixture struct {
	repository  *business.Store
	database    *sql.DB
	private     *configstore.Store
	privatePath string
	settings    credentialSettings
	upstream    *httptest.Server
	generated   atomic.Int32
	keyReads    atomic.Int32
	keyHandler  http.HandlerFunc
	protocolURL string
}

func newCredentialFixture(t *testing.T) *credentialFixture {
	t.Helper()
	fixture := &credentialFixture{}
	fixture.upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/keys" || strings.HasPrefix(r.URL.Path, "/api/token/") {
			fixture.keyReads.Add(1)
			if fixture.keyHandler != nil {
				fixture.keyHandler(w, r)
				return
			}
			http.Error(w, "unexpected key read", http.StatusInternalServerError)
			return
		}
		fixture.generated.Add(1)
		if r.URL.Path != "/v1/responses" || r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer test-bound-key" || r.Header.Get("X-API-Key") != "" {
			t.Errorf("unexpected direct generation method=%s path=%s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"pong\"}\n\n"))
	}))
	t.Cleanup(fixture.upstream.Close)
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/admin/accounts/41" {
			t.Errorf("unexpected management operation method=%s path=%s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		credentials := map[string]any{"base_url": fixture.upstream.URL}
		if fixture.protocolURL != "" {
			credentials["api_base_urls"] = map[string]string{"responses": fixture.protocolURL}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
			"id": 41, "type": "apikey", "platform": "openai",
			"credentials":        credentials,
			"credentials_status": map[string]bool{"has_api_key": true},
		}})
	}))
	t.Cleanup(admin.Close)
	path := filepath.Join(t.TempDir(), "probe-bindings.sqlite3")
	var err error
	fixture.repository, err = business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fixture.repository.Close() })
	if err := fixture.repository.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	fixture.database, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fixture.database.Close() })
	metadata, err := json.Marshal(map[string]any{"known_models": []string{"probe-model"}, "base_url": fixture.upstream.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.database.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','probe account',?,'now'); INSERT INTO account_groups(account_id,group_name) VALUES('41','codex')`, string(metadata)); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.database.Exec(`INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,upstream_group_id,local_group,status,updated_at) VALUES('41',?,'91','bound key','7','codex','active','now')`, configstore.CanonicalHost(fixture.upstream.URL)); err != nil {
		t.Fatal(err)
	}
	fixture.privatePath = filepath.Join(t.TempDir(), "private.sqlite3")
	fixture.private, err = configstore.Open(fixture.privatePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fixture.private.Close() })
	fixture.settings = credentialSettings{Store: fixture.private, endpoint: admin.URL}
	return fixture
}

func (fixture *credentialFixture) cacheKey(t *testing.T) {
	t.Helper()
	if err := fixture.private.SaveUpstreamKeySecret(context.Background(), configstore.UpstreamKeySecret{
		Host: configstore.CanonicalHost(fixture.upstream.URL), KeyID: "91", GroupID: "7", Secret: "test-bound-key",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRedactedManagementKeyUsesExactPrivateBindingWithoutRemoteKeyRead(t *testing.T) {
	fixture := newCredentialFixture(t)
	fixture.cacheKey(t)
	summary, err := probe.New(fixture.repository, fixture.settings, nil).RunNow(context.Background(), probe.Request{AccountID: pointer("41")})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Passed != 1 || summary.Persisted != 1 || fixture.generated.Load() != 1 || fixture.keyReads.Load() != 0 {
		t.Fatalf("cached bound credential did not probe: %+v", summary)
	}
	encoded, _ := json.Marshal(summary)
	if strings.Contains(string(encoded), "test-bound-key") || strings.Contains(string(encoded), "test-admin-key") {
		t.Fatal("summary exposed private credentials")
	}
}

func TestRedactedManagementKeyRejectsMissingAmbiguousOrChangedBindingWithoutHealthEvidence(t *testing.T) {
	for _, scenario := range []struct{ name, mutation string }{
		{"missing", `UPDATE bindings SET status='missing'`},
		{"ambiguous", `INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,upstream_group_id,local_group,status,updated_at) SELECT local_account_id,upstream_host,'92','other key','7',local_group,'active','now' FROM bindings`},
		{"changed_url", `UPDATE accounts SET metadata_json='{"known_models":["probe-model"],"base_url":"https://unrelated.invalid"}'`},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newCredentialFixture(t)
			fixture.cacheKey(t)
			if _, err := fixture.database.Exec(scenario.mutation); err != nil {
				t.Fatal(err)
			}
			summary, err := probe.New(fixture.repository, fixture.settings, nil).RunNow(context.Background(), probe.Request{AccountID: pointer("41")})
			if err != nil {
				t.Fatal(err)
			}
			if summary.Skipped != 1 || summary.Persisted != 0 || summary.Failed != 0 || fixture.generated.Load() != 0 || fixture.keyReads.Load() != 0 {
				t.Fatalf("unsafe binding affected health: %+v", summary)
			}
		})
	}
}

func (fixture *credentialFixture) authorize(t *testing.T, platform string) {
	t.Helper()
	if err := fixture.private.SaveAuthRecord(context.Background(), configstore.AuthRecord{
		Host: configstore.CanonicalHost(fixture.upstream.URL), BaseURL: fixture.upstream.URL,
		UpstreamType: platform, AuthMode: "bearer", AccessToken: pointer("test-upstream-login"),
	}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestRedactedManagementKeyWithoutCachePersistsBoundKeyForReopenedConsole(t *testing.T) {
	for _, platform := range []string{"sub2api", "newapi"} {
		t.Run(platform, func(t *testing.T) {
			fixture := newCredentialFixture(t)
			fixture.authorize(t, platform)
			fixture.keyHandler = func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer test-upstream-login" || r.Header.Get("X-API-Key") != "" {
					t.Error("upstream key read used incorrect authorization")
				}
				if platform == "newapi" && r.Method == http.MethodPost && r.URL.Path == "/api/token/91/key" {
					_, _ = w.Write([]byte(`{"success":true,"data":{"key":"test-bound-key"}}`))
					return
				}
				expectedPath := "/api/v1/keys"
				secret := "test-bound-key"
				if platform == "newapi" {
					expectedPath, secret = "/api/token/", "masked***"
				}
				if r.Method != http.MethodGet || r.URL.Path != expectedPath {
					t.Errorf("unexpected key operation method=%s path=%s", r.Method, r.URL.Path)
					http.NotFound(w, r)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"total": 1, "items": []any{map[string]any{"id": 91, "group_id": 7, "key": secret}}}})
			}
			service := probe.New(fixture.repository, fixture.settings, nil)
			service.UseKeyRevealer(upstreamsync.NewReader(fixture.upstream.Client()))
			summary, err := service.RunNow(context.Background(), probe.Request{AccountID: pointer("41")})
			if err != nil {
				t.Fatal(err)
			}
			expectedReads := int32(1)
			if platform == "newapi" {
				expectedReads = 2
			}
			if summary.Passed != 1 || summary.Persisted != 1 || fixture.generated.Load() != 1 || fixture.keyReads.Load() != expectedReads {
				t.Fatalf("revealed credential did not probe: %+v, key reads=%d", summary, fixture.keyReads.Load())
			}
			cached, err := fixture.private.UpstreamKeySecret(context.Background(), configstore.CanonicalHost(fixture.upstream.URL), "91", "7")
			if err != nil || cached == nil || cached.Secret != "test-bound-key" {
				t.Fatal("probe did not save revealed credential under the exact private binding")
			}
			if err := fixture.private.Close(); err != nil {
				t.Fatal(err)
			}
			fixture.private, err = configstore.Open(fixture.privatePath)
			if err != nil {
				t.Fatal(err)
			}
			fixture.settings.Store = fixture.private
			reopened := probe.New(fixture.repository, fixture.settings, nil)
			summary, err = reopened.RunNow(context.Background(), probe.Request{AccountID: pointer("41")})
			if err != nil {
				t.Fatal(err)
			}
			if summary.Passed != 1 || fixture.generated.Load() != 2 || fixture.keyReads.Load() != expectedReads {
				t.Fatalf("reopened Console did not reuse saved Key without remote access: %+v", summary)
			}
			encoded, _ := json.Marshal(summary)
			if strings.Contains(string(encoded), "test-bound-key") || strings.Contains(string(encoded), "test-upstream-login") {
				t.Fatal("saved private credential leaked into probe result")
			}
		})
	}
}

func TestRedactedManagementKeyRechecksBindingAfterUpstreamCredentialRead(t *testing.T) {
	for _, mutation := range []string{
		`UPDATE bindings SET upstream_key_id='92',updated_at='changed'`,
		`UPDATE accounts SET metadata_json='{"known_models":["probe-model"],"base_url":"https://changed.invalid"}'`,
	} {
		t.Run(mutation, func(t *testing.T) {
			fixture := newCredentialFixture(t)
			fixture.authorize(t, "sub2api")
			fixture.keyHandler = func(w http.ResponseWriter, r *http.Request) {
				if _, err := fixture.database.Exec(mutation); err != nil {
					t.Error(err)
					http.Error(w, "isolated mutation failed", 500)
					return
				}
				_, _ = w.Write([]byte(`{"data":{"total":1,"items":[{"id":91,"group_id":7,"key":"test-bound-key"}]}}`))
			}
			service := probe.New(fixture.repository, fixture.settings, nil)
			service.UseKeyRevealer(upstreamsync.NewReader(fixture.upstream.Client()))
			summary, err := service.RunNow(context.Background(), probe.Request{AccountID: pointer("41")})
			if err != nil {
				t.Fatal(err)
			}
			if summary.Skipped != 1 || summary.Persisted != 0 || fixture.generated.Load() != 0 || summary.Results[0].FailureCode != "probe_key_binding_changed" {
				t.Fatalf("changed binding generated with stale credentials: %+v", summary)
			}
			cached, err := fixture.private.UpstreamKeySecret(context.Background(), configstore.CanonicalHost(fixture.upstream.URL), "91", "7")
			if err != nil || cached != nil {
				t.Fatal("credential was saved after its binding changed")
			}
		})
	}
}

func TestRedactedManagementKeyReadFailureDoesNotExposeRemoteSecretOrRecordFailure(t *testing.T) {
	fixture := newCredentialFixture(t)
	fixture.authorize(t, "sub2api")
	fixture.keyHandler = func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "test-upstream-login test-bound-key", http.StatusForbidden)
	}
	service := probe.New(fixture.repository, fixture.settings, nil)
	service.UseKeyRevealer(upstreamsync.NewReader(fixture.upstream.Client()))
	summary, err := service.RunNow(context.Background(), probe.Request{AccountID: pointer("41")})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Skipped != 1 || summary.Failed != 0 || summary.Persisted != 0 || fixture.generated.Load() != 0 {
		t.Fatalf("credential read failure affected health: %+v", summary)
	}
	encoded, _ := json.Marshal(summary)
	if strings.Contains(string(encoded), "test-upstream-login") || strings.Contains(string(encoded), "test-bound-key") {
		t.Fatal("remote credential read error exposed private material")
	}
}

func TestRedactedManagementKeySharesSameCredentialAcrossBindingsWithSourceAuthHost(t *testing.T) {
	fixture := newCredentialFixture(t)
	fixture.cacheKey(t)
	if _, err := fixture.database.Exec(`INSERT INTO bindings(local_account_id,upstream_host,source_auth_host,upstream_key_id,upstream_key_name,upstream_group_id,local_group,status,updated_at)
		VALUES('41','alias.invalid',?,'91','same bound key','7','second group','active','now')`, configstore.CanonicalHost(fixture.upstream.URL)); err != nil {
		t.Fatal(err)
	}
	summary, err := probe.New(fixture.repository, fixture.settings, nil).RunNow(context.Background(), probe.Request{AccountID: pointer("41")})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Passed != 1 || fixture.generated.Load() != 1 || fixture.keyReads.Load() != 0 {
		t.Fatalf("same stable credential treated as ambiguous: %+v", summary)
	}
}

func TestRedactedManagementKeyWithProtocolURLUsesBindingBaseURLForCachedAndRevealedKeys(t *testing.T) {
	for _, source := range []string{"cached", "revealed"} {
		t.Run(source, func(t *testing.T) {
			fixture := newCredentialFixture(t)
			var generations atomic.Int32
			generation := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				generations.Add(1)
				if r.URL.Path != "/responses-api/v1/responses" || r.Header.Get("Authorization") != "Bearer test-bound-key" {
					t.Error("generation did not use configured protocol URL and bound key")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte("event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"pong\"}\n\n"))
			}))
			defer generation.Close()
			fixture.protocolURL = generation.URL + "/responses-api"
			fixture.authorize(t, "sub2api")
			fixture.keyHandler = func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"data":{"total":1,"items":[{"id":91,"group_id":7,"key":"test-bound-key"}]}}`))
			}
			if source == "cached" {
				fixture.cacheKey(t)
			}
			service := probe.New(fixture.repository, fixture.settings, nil)
			service.UseKeyRevealer(upstreamsync.NewReader(fixture.upstream.Client()))
			summary, err := service.RunNow(context.Background(), probe.Request{AccountID: pointer("41")})
			if err != nil {
				t.Fatal(err)
			}
			if summary.Passed != 1 || generations.Load() != 1 || fixture.generated.Load() != 0 {
				t.Fatalf("protocol URL incorrectly changed key binding: %+v", summary)
			}
		})
	}
}

func TestRedactedManagementKeyWithMaskedCacheSavesUsableBoundKeyForNextProbe(t *testing.T) {
	fixture := newCredentialFixture(t)
	fixture.authorize(t, "sub2api")
	if err := fixture.private.SaveUpstreamKeySecret(context.Background(), configstore.UpstreamKeySecret{
		Host: configstore.CanonicalHost(fixture.upstream.URL), KeyID: "91", GroupID: "7", Secret: "sk-masked***",
	}); err != nil {
		t.Fatal(err)
	}
	fixture.keyHandler = func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"total":1,"items":[{"id":91,"group_id":7,"key":"test-bound-key"}]}}`))
	}
	service := probe.New(fixture.repository, fixture.settings, nil)
	service.UseKeyRevealer(upstreamsync.NewReader(fixture.upstream.Client()))
	summary, err := service.RunNow(context.Background(), probe.Request{AccountID: pointer("41")})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Passed != 1 || fixture.generated.Load() != 1 || fixture.keyReads.Load() != 1 {
		t.Fatalf("masked cache prevented bound key resolution: %+v", summary)
	}
	cached, err := fixture.private.UpstreamKeySecret(context.Background(), configstore.CanonicalHost(fixture.upstream.URL), "91", "7")
	if err != nil || cached == nil || cached.Secret != "test-bound-key" {
		t.Fatal("resolved credential did not replace masked cache")
	}
	service = probe.New(fixture.repository, fixture.settings, nil)
	summary, err = service.RunNow(context.Background(), probe.Request{AccountID: pointer("41")})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Passed != 1 || fixture.generated.Load() != 2 || fixture.keyReads.Load() != 1 {
		t.Fatalf("next probe did not use replaced cache: %+v", summary)
	}
}

func TestRedactedManagementKeyPersistenceFailureSkipsWithoutHealthEvidenceOrSecretDisclosure(t *testing.T) {
	fixture := newCredentialFixture(t)
	fixture.authorize(t, "sub2api")
	fixture.keyHandler = func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"total":1,"items":[{"id":91,"group_id":7,"key":"test-bound-key"}]}}`))
	}
	database, err := sql.Open("sqlite", fixture.privatePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Exec(`CREATE TRIGGER reject_probe_key BEFORE INSERT ON upstream_key_secrets
		BEGIN SELECT RAISE(ABORT, 'test-bound-key test-upstream-login private write refused'); END`); err != nil {
		t.Fatal(err)
	}
	service := probe.New(fixture.repository, fixture.settings, nil)
	service.UseKeyRevealer(upstreamsync.NewReader(fixture.upstream.Client()))
	summary, err := service.RunNow(context.Background(), probe.Request{AccountID: pointer("41")})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Skipped != 1 || summary.Failed != 0 || summary.Persisted != 0 || fixture.generated.Load() != 0 {
		t.Fatalf("credential persistence failure affected health or sent generation: %+v", summary)
	}
	if len(summary.Results) != 1 || summary.Results[0].FailureReason == nil || !strings.Contains(*summary.Results[0].FailureReason, "保存") {
		t.Fatalf("credential persistence failure lacks actionable reason: %+v", summary)
	}
	encoded, _ := json.Marshal(summary)
	if strings.Contains(string(encoded), "test-bound-key") || strings.Contains(string(encoded), "test-upstream-login") || strings.Contains(string(encoded), "private write refused") {
		t.Fatal("private persistence error leaked into probe result")
	}
	cached, err := fixture.private.UpstreamKeySecret(context.Background(), configstore.CanonicalHost(fixture.upstream.URL), "91", "7")
	if err != nil || cached != nil {
		t.Fatal("failed persistence left an available cached credential")
	}
}

func TestRedactedManagementKeyDoesNotSaveOrProbeAfterAuthorizationChanges(t *testing.T) {
	fixture := newCredentialFixture(t)
	fixture.authorize(t, "sub2api")
	fixture.keyHandler = func(w http.ResponseWriter, _ *http.Request) {
		if err := fixture.private.SaveAuthRecord(context.Background(), configstore.AuthRecord{
			Host: configstore.CanonicalHost(fixture.upstream.URL), BaseURL: fixture.upstream.URL,
			UpstreamType: "sub2api", AuthMode: "bearer", AccessToken: pointer("replacement-upstream-login"),
		}, nil); err != nil {
			t.Error(err)
		}
		_, _ = w.Write([]byte(`{"data":{"total":1,"items":[{"id":91,"group_id":7,"key":"test-bound-key"}]}}`))
	}
	service := probe.New(fixture.repository, fixture.settings, nil)
	service.UseKeyRevealer(upstreamsync.NewReader(fixture.upstream.Client()))
	summary, err := service.RunNow(context.Background(), probe.Request{AccountID: pointer("41")})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Skipped != 1 || summary.Failed != 0 || summary.Persisted != 0 || fixture.generated.Load() != 0 {
		t.Fatalf("credential from old authorization used: %+v", summary)
	}
	if len(summary.Results) != 1 || summary.Results[0].FailureCode != "probe_key_auth_changed" {
		t.Fatalf("authorization change lacks specific skip reason: %+v", summary)
	}
	stored, err := fixture.private.UpstreamKeySecret(context.Background(), configstore.CanonicalHost(fixture.upstream.URL), "91", "7")
	if err != nil || stored != nil {
		t.Fatal("credential from old authorization was saved")
	}
}
