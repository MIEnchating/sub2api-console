package upstreamconfig_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamconfig"
)

const addressSourceHost = "subclaude.example.test"
const addressAliasHost = "192.0.2.44:8080"
const addressAliasURL = "http://192.0.2.44:8080"

// Only the remote authentication boundary is replaced; all identity and
// credential persistence uses real stores in temporary databases.
type addressVerifier struct{ failure error }

func (v *addressVerifier) Verify(context.Context, configstore.AuthRecord) error {
	return v.failure
}

func (v *addressVerifier) Login(_ context.Context, record configstore.AuthRecord, _ configstore.VaultEntry) (configstore.AuthRecord, error) {
	if v.failure != nil {
		return record, v.failure
	}
	token, refresh := "verified-access-token", "verified-refresh-token"
	record.AccessToken, record.RefreshToken = &token, &refresh
	return record, nil
}

type addressFixture struct {
	service    *upstreamconfig.Service
	repository *business.Store
	private    *configstore.Store
	db         *sql.DB
	verifier   *addressVerifier
	input      upstreamconfig.Input
	original   upstreamconfig.Configuration
}

func newAddressFixture(t *testing.T, baseURL string) *addressFixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "business.db")
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	if err := repository.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	private, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = private.Close() })
	db, err := sql.Open("sqlite", sqliteutil.DSN(path, ""))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	name, entry, username, password := "测试上游", "测试登录资料", "operator@example.test", "isolated-password"
	if err := private.SaveVaultEntry(t.Context(), configstore.VaultEntry{Entry: entry, Username: &username, Password: &password}, nil); err != nil {
		t.Fatal(err)
	}
	fixture := &addressFixture{repository: repository, private: private, db: db, verifier: &addressVerifier{}}
	fixture.service = upstreamconfig.New(repository, private, fixture.verifier)
	fixture.input = upstreamconfig.Input{
		Host: addressSourceHost, Name: &name, BaseURL: baseURL, AccountBaseURL: "https://models.example.test/v1",
		UpstreamType: "sub2api", AuthMode: "sub2api_user_login", RechargeRate: "1",
		Entry: &entry, Present: map[string]bool{"entry": true},
	}
	fixture.original, err = fixture.service.Create(t.Context(), fixture.input, "test")
	if err != nil {
		t.Fatal(err)
	}
	return fixture
}

func (f *addressFixture) addAlias(t *testing.T, stored bool) {
	t.Helper()
	_, err := f.db.ExecContext(t.Context(), `INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at)
		VALUES(?,?,0,'now')`, addressAliasHost, f.original.UpstreamID)
	if err != nil {
		t.Fatal(err)
	}
	if stored {
		_, err := f.db.ExecContext(t.Context(), `INSERT INTO upstreams(host,base_url,upstream_type,auth_mode,auth_status,metadata_json,updated_at)
			VALUES(?,?,'sub2api','sub2api_user_token','已鉴权','{}','now')`, addressAliasHost, addressAliasURL)
		if err != nil {
			t.Fatal(err)
		}
	}
}
