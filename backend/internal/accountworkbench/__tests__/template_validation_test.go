package accountworkbench_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"
)

func templateValidationFixture(t testing.TB, count int) (*accountworkbench.Service, *configstore.Store, *sql.DB, string, string, accountworkbench.RunConfirmation) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "template-validation.db")
	store, err := configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.ConfigureTarget(ctx, "https://isolated.invalid", "test-admin", 10); err != nil {
		t.Fatal(err)
	}
	target, err := store.TargetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	token, err := store.CreateSession(ctx, "test-admin", time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(token))
	owner := hex.EncodeToString(hash[:])
	hash = sha256.Sum256([]byte(target.BaseURL + "\x00" + target.AdminKey))
	key := "templates:" + hex.EncodeToString(hash[:])
	items := make([]accountworkbench.Template, count)
	for i := range items {
		items[i] = accountworkbench.Template{ID: "template", Config: accountworkbench.TemplateConfig{Notes: strings.Repeat("x", 1024), RateMultiplier: "1", GroupIDs: []string{"7"}}}
	}
	raw, err := json.Marshal(accountworkbench.TemplateLibrary{Items: items})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveWorkbenchDocument(ctx, key, 0, raw); err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("a", 32)
	raw, err = json.Marshal(map[string]any{
		"scope": "managed", "owner": owner, "target": target, "template_revision": 1,
		"public": accountworkbench.Run{ID: id, Action: "import", Status: "interrupted", ExpiresAt: time.Now().Add(time.Hour), Items: []accountworkbench.RunItem{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveWorkbenchDocument(ctx, "run:"+owner+":"+id, 0, raw); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", sqliteutil.DSN(path, ""))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return accountworkbench.New(store), store, db, key, owner, accountworkbench.RunConfirmation{ID: id, Revision: 1}
}

const noRetryItems = "没有可重试项；结果不明的提交需先在官方或站点核对"

func TestTemplateValidationAlwaysChecksCurrentContentEvenWhenRevisionIsUnchanged(t *testing.T) {
	service, _, db, key, owner, confirmation := templateValidationFixture(t, 1)
	for _, raw := range []string{`{"items":[]}`, `{"items":[{"config":{"concurrency":"invalid"}}]}`, `{"items":[]}`, `{"items":`, `null`} {
		if _, err := db.Exec(`UPDATE account_workbench_documents SET payload=? WHERE id=?`, raw, key); err != nil {
			t.Fatal(err)
		}
		_, err := service.Retry(t.Context(), owner, confirmation)
		expected := noRetryItems
		if strings.Contains(raw, "invalid") || raw == `{"items":` {
			expected = "模板记录无法读取"
		}
		if err == nil || err.Error() != expected {
			t.Fatalf("payload=%s err=%v want=%s", raw, err, expected)
		}
	}
}

func TestTemplateValidationRejectsUpdatedDeletedOrUnavailableLibraryAfterWarmRead(t *testing.T) {
	for _, change := range []string{"update", "delete", "unavailable"} {
		t.Run(change, func(t *testing.T) {
			service, store, db, key, owner, confirmation := templateValidationFixture(t, 1)
			if _, err := service.Retry(t.Context(), owner, confirmation); err == nil || err.Error() != noRetryItems {
				t.Fatal(err)
			}
			switch change {
			case "update":
				if _, err := store.SaveWorkbenchDocument(t.Context(), key, 1, json.RawMessage(`{"items":[]}`)); err != nil {
					t.Fatal(err)
				}
			case "delete":
				if err := store.DeleteWorkbenchDocument(t.Context(), key, 1); err != nil {
					t.Fatal(err)
				}
			case "unavailable":
				if _, err := db.Exec(`DROP TABLE account_workbench_documents`); err != nil {
					t.Fatal(err)
				}
			}
			_, err := service.Retry(t.Context(), owner, confirmation)
			if err == nil || err.Error() == noRetryItems {
				t.Fatalf("changed library accepted: %v", err)
			}
			if change != "unavailable" && err.Error() != "模板已变化，请重新预览" {
				t.Fatal(err)
			}
		})
	}
}

func TestTemplateValidationSupportsConcurrentChecksAndCancelledReads(t *testing.T) {
	service, _, _, _, owner, confirmation := templateValidationFixture(t, 20)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := service.Retry(t.Context(), owner, confirmation); err == nil || err.Error() != noRetryItems {
				t.Errorf("validation: %v", err)
			}
		})
	}
	wg.Wait()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := service.Retry(ctx, owner, confirmation); err == nil || err.Error() == noRetryItems {
		t.Fatalf("cancelled check accepted: %v", err)
	}
}

func TestTemplateValidationRejectsCorruptedLibraryBeforeClaimingImportPreview(t *testing.T) {
	service, store, db, key, owner, confirmation := templateValidationFixture(t, 1)
	runTasks(t, service)
	preview, err := service.Preview(t.Context(), owner, accountworkbench.PreviewInput{Action: "import", Content: "rt_isolated"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Retry(t.Context(), owner, confirmation); err == nil || err.Error() != noRetryItems {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE account_workbench_documents SET payload='{"items":true}' WHERE id=?`, key); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(t.Context(), owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision}); err == nil || err.Error() != "模板记录无法读取" {
		t.Fatalf("corrupt library accepted: %v", err)
	}
	raw, revision, err := store.WorkbenchDocument(t.Context(), "preview:"+owner)
	if err != nil || revision != preview.Revision {
		t.Fatalf("rejected start changed preview revision: %d %v", revision, err)
	}
	var document struct {
		Claimed bool `json:"claimed"`
	}
	if err := json.Unmarshal(raw, &document); err != nil || document.Claimed {
		t.Fatalf("rejected start claimed preview: %v", err)
	}
}

func BenchmarkTemplateRevisionValidation(b *testing.B) {
	for _, count := range []int{1, 1000} {
		name := "single"
		if count == 1000 {
			name = "large"
		}
		b.Run(name, func(b *testing.B) {
			service, _, _, _, owner, confirmation := templateValidationFixture(b, count)
			if _, err := service.Retry(b.Context(), owner, confirmation); err == nil || err.Error() != noRetryItems {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if _, err := service.Retry(b.Context(), owner, confirmation); err == nil || err.Error() != noRetryItems {
					b.Fatal(err)
				}
			}
		})
	}
}
