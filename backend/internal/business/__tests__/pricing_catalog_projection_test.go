package business_test

import (
	"database/sql"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func pricingProjectionFixture(t testing.TB) (*business.Store, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pricing.db")
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
	return store, db
}

func TestPricingCatalogPlatformProjectionPreservesTextAndFallbackSemantics(t *testing.T) {
	store, db := pricingProjectionFixture(t)
	cases := []struct{ metadata, want string }{
		{`{"platform":" OpenAI "}`, "openai"},
		{`{"platform":"\u2003Anthropic\t"}`, "anthropic"},
		{`{"platform":"openai","platform":"gemini"}`, "gemini"},
		{`{"platform":"openai","platform":null}`, "fallback"},
		{`{"platform":123}`, "fallback"},
		{`{"platform":true}`, "fallback"},
		{`{"platform":{"name":"openai"}}`, "fallback"},
		{`{"platform":["openai"]}`, "fallback"},
		{`{"platform":"   "}`, "fallback"},
		{`{}`, "fallback"},
		{`[{"platform":"openai"}]`, "fallback"},
		{`null`, "fallback"},
		{`{"platform":"openai"} {}`, "fallback"},
		{`invalid`, "fallback"},
		{`{"platform":"openai","notes":` + strings.Repeat("[", 1001) + "0" + strings.Repeat("]", 1001) + "}", "openai"},
	}
	if _, err := db.Exec(`INSERT INTO local_groups(name,remote_id,platform,updated_at) VALUES('默认','6','fallback','now')`); err != nil {
		t.Fatal(err)
	}
	for index, fixture := range cases {
		id := strconv.Itoa(index + 1)
		if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES(?,'fixture',?,'now')`, id, fixture.metadata); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO account_groups(account_id,group_name,group_id) VALUES(?,'默认','6')`, id); err != nil {
			t.Fatal(err)
		}
	}
	catalog, err := store.PricingCatalog(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Accounts) != len(cases) {
		t.Fatalf("accounts=%d", len(catalog.Accounts))
	}
	for index, account := range catalog.Accounts {
		if account.ID != strconv.Itoa(index+1) || account.Platform != cases[index].want || !account.GroupsValid || len(account.GroupIDs) != 1 || account.GroupIDs[0] != "6" {
			t.Errorf("metadata=%s account=%+v want platform=%q", cases[index].metadata, account, cases[index].want)
		}
		scoped, err := store.PricingCatalogForAccount(t.Context(), account.ID)
		if err != nil || len(scoped.Accounts) != 1 || scoped.Accounts[0].Platform != cases[index].want {
			t.Errorf("scoped metadata=%s catalog=%+v err=%v want platform=%q", cases[index].metadata, scoped, err, cases[index].want)
		}
	}
}

func BenchmarkPricingCatalogWithLargeUnrelatedMetadata(b *testing.B) {
	store, db := pricingProjectionFixture(b)
	metadata := `{"platform":"openai","notes":"` + strings.Repeat("fixture ", 1024) + `"}`
	if _, err := db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<1000)
		INSERT INTO accounts(id,name,metadata_json,updated_at) SELECT CAST(x AS TEXT),'fixture',?,'now' FROM n`, metadata); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		catalog, err := store.PricingCatalog(b.Context())
		if err != nil || len(catalog.Accounts) != 1000 || catalog.Accounts[0].Platform != "openai" {
			b.Fatalf("accounts=%d err=%v", len(catalog.Accounts), err)
		}
	}
}
