package business_test

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"
)

func TestGroupHistoryRatesUseStableIdentityAndLatestCatalogWithoutDuplicatingEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", sqliteutil.DSN(path, ""))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`
		INSERT INTO upstream_identities(upstream_id,created_at,updated_at) VALUES('one','now','now'),('two','now','now');
		INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at) VALUES
		('one.example','one',1,'now'),('alias.example','one',0,'now'),('two.example','two',1,'now');
		INSERT INTO upstream_groups(host,group_id,name,raw_rate,effective_rate,updated_at) VALUES
		('one.example','7','同名组','0.5','0.25','2026-09-17'),
		('alias.example','7','同名组','0.5','0.1234567890123456789','2026-09-18'),
		('two.example','7','同名组','0','0','2026-09-18');
		INSERT INTO upstream_group_change_events(upstream_id,group_id,group_name,change_type,changed_at) VALUES
		('one','7','同名组','added','2026-09-18T01:00:00Z'),
		('two','7','同名组','removed','2026-09-18T02:00:00Z'),
		('one','missing','无目录组','removed','2026-09-18T03:00:00Z');`)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := store.AllUpstreamGroupHistory(t.Context(), 10)
	if err != nil {
		t.Fatal(err)
	}
	assertHistoryRates(t, rows, []*string{nil, ptrHistoryRate("0"), ptrHistoryRate("0.1234567890123456789")})
	rows, err = store.UpstreamGroupHistory(t.Context(), "alias.example", 10)
	if err != nil {
		t.Fatal(err)
	}
	assertHistoryRates(t, rows, []*string{nil, ptrHistoryRate("0.1234567890123456789")})
}

func TestGroupHistoryUsesLatestRechargeConvertedRate(t *testing.T) {
	store, err := business.Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{
		Host: "history.example", BaseURL: "https://history.example", UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", RechargeRate: "2",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, groups := range [][]business.UpstreamCatalogGroup{
		{},
		{{GroupID: "7", Name: "测试组", RawRate: ptrHistoryRate("0.25")}},
		{{GroupID: "7", Name: "测试组", RawRate: ptrHistoryRate("0.75")}},
		{}, {},
	} {
		_, err := store.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{
			Host: "history.example", Catalog: &business.UpstreamCatalogSnapshot{Groups: groups}, AuthenticationOK: true,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	rows, err := store.AllUpstreamGroupHistory(t.Context(), 10)
	if err != nil {
		t.Fatal(err)
	}
	assertHistoryRates(t, rows, []*string{ptrHistoryRate("0.375"), ptrHistoryRate("0.375")})
}

func ptrHistoryRate(value string) *string { return &value }

func assertHistoryRates(t *testing.T, rows []business.UpstreamGroupChange, expected []*string) {
	t.Helper()
	data, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	var payload []map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != len(expected) {
		t.Fatalf("history events=%d, want %d", len(payload), len(expected))
	}
	for index, rate := range expected {
		actual, present := payload[index]["effective_rate"]
		if !present || (rate == nil && actual != nil) || (rate != nil && actual != *rate) {
			t.Fatalf("event %d effective_rate=%v, expected %v (present=%v)", index, actual, rate, present)
		}
	}
}
