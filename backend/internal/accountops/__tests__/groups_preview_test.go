package accountops_test

import (
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountops"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

func groupPreviewStore(t testing.TB) (*business.Store, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "groups.db")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetMode(t.Context(), runtimepolicy.Full); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','目标','{"platform":"openai"}','now');
		INSERT INTO local_groups(name,remote_id,platform,updated_at) VALUES('标准','6','openai','now'),('混合','9','composite','now'),('其他','12','anthropic','now');
		INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','标准','6')`); err != nil {
		t.Fatal(err)
	}
	return store, db
}

func TestGroupPreviewRejectsMissingAccount(t *testing.T) {
	store, _ := groupPreviewStore(t)
	service := accountops.New(settingsTarget{endpoint: "https://management.invalid"}, store, &settingsTasks{})
	if _, err := service.AccountGroups(t.Context(), "99"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing account: %v", err)
	}
}

func TestGroupPreviewRejectsMonitoringMode(t *testing.T) {
	store, _ := groupPreviewStore(t)
	if _, err := store.SetMode(t.Context(), runtimepolicy.Monitoring); err != nil {
		t.Fatal(err)
	}
	service := accountops.New(settingsTarget{endpoint: "https://management.invalid"}, store, &settingsTasks{})
	if _, err := service.AccountGroups(t.Context(), "41"); err == nil || !strings.Contains(err.Error(), "完全模式") {
		t.Fatalf("monitoring mode: %v", err)
	}
}

func TestGroupPreviewDoesNotDependOnAccountEvidenceHistory(t *testing.T) {
	store, db := groupPreviewStore(t)
	if _, err := db.Exec(`DROP TABLE health_samples`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Account(t.Context(), "41"); err == nil {
		t.Fatal("fixture must make the full account detail unavailable")
	}
	service := accountops.New(settingsTarget{endpoint: "https://management.invalid"}, store, &settingsTasks{})
	preview, err := service.AccountGroups(t.Context(), "41")
	if err != nil {
		t.Fatal(err)
	}
	if preview.AccountID != "41" || !reflect.DeepEqual(preview.CurrentGroupIDs, []string{"6"}) || !reflect.DeepEqual(preview.Groups, []accountops.AccountGroupOption{{ID: "6", Name: "标准"}, {ID: "9", Name: "混合"}}) || preview.TargetVersion == "" {
		t.Fatalf("preview=%+v", preview)
	}
}

func BenchmarkGroupPreviewWithUnrelatedAccounts(b *testing.B) {
	store, db := groupPreviewStore(b)
	metadata := `{"platform":"openai","notes":"` + strings.Repeat("fixture ", 1024) + `"}`
	if _, err := db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<1000)
		INSERT INTO accounts(id,name,metadata_json,updated_at) SELECT CAST(x AS TEXT),'fixture',?,'now' FROM n WHERE x<>41`, metadata); err != nil {
		b.Fatal(err)
	}
	service := accountops.New(settingsTarget{endpoint: "https://management.invalid"}, store, &settingsTasks{})
	b.ReportAllocs()
	for b.Loop() {
		preview, err := service.AccountGroups(b.Context(), "41")
		if err != nil || len(preview.Groups) != 2 || len(preview.CurrentGroupIDs) != 1 {
			b.Fatalf("preview=%+v err=%v", preview, err)
		}
	}
}
