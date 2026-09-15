package configstore_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"
)

func settingsScopeFixture(t testing.TB) *configstore.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "scoped-settings.db")
	store, err := configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.ConfigureTarget(context.Background(), "https://isolated.invalid", "fixture-key", 45); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", sqliteutil.DSN(path, ""))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, key := range []string{"account_workbench.execution.fixture", "uptime_kuma.template.fixture"} {
		if _, err := db.Exec(`INSERT INTO settings(key,value) VALUES(?,?)`, key, strings.Repeat("private-fixture", 1<<16)); err != nil {
			t.Fatal(err)
		}
	}
	return store
}

func TestScopedSettingsReadPreservesTargetAndCompleteSortedKeyMetadata(t *testing.T) {
	store := settingsScopeFixture(t)
	target, err := store.TargetSettings(context.Background())
	if err != nil || target.BaseURL != "https://isolated.invalid" || target.AdminKey != "fixture-key" || target.TimeoutSeconds != 45 {
		t.Fatalf("target settings changed with unrelated private records: %#v, %v", target, err)
	}
	runtime, err := store.RuntimeSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"account_workbench.execution.fixture", "target.admin_key", "target.base_url", "target.timeout_seconds", "uptime_kuma.template.fixture"}
	if !reflect.DeepEqual(runtime.Keys, want) {
		t.Fatalf("runtime key metadata changed: %v", runtime.Keys)
	}
}

func BenchmarkTargetSettingsWithUnrelatedPrivatePayloads(b *testing.B) {
	store := settingsScopeFixture(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := store.TargetSettings(ctx); err != nil {
			b.Fatal(err)
		}
	}
}
