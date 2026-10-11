package accountworkbench_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func BenchmarkMaintenanceStatusWithPrivateAttempts(b *testing.B) {
	store, err := configstore.Open(filepath.Join(b.TempDir(), "workbench.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = store.Close() })
	if err := store.ConfigureTarget(b.Context(), "https://isolated.invalid", "test-admin", 10); err != nil {
		b.Fatal(err)
	}
	token, err := store.CreateSession(b.Context(), "test-admin", time.Hour, time.Now().UTC())
	if err != nil {
		b.Fatal(err)
	}
	ownerHash := sha256.Sum256([]byte(token))
	owner := hex.EncodeToString(ownerHash[:])
	targetHash := sha256.Sum256([]byte("https://isolated.invalid\x00test-admin"))
	key := "maintenance:" + hex.EncodeToString(targetHash[:])
	attempts := make(map[string]any, 100)
	for i := range 100 {
		attempts[fmt.Sprintf("account-%03d", i)] = map[string]any{
			"phase": "pending_upload", "credentials": map[string]any{"access_token": strings.Repeat("x", 8192)},
		}
	}
	raw, err := json.Marshal(map[string]any{"public": map[string]any{"enabled": true, "interval_minutes": 5, "results": []any{}}, "attempts": attempts})
	if err != nil {
		b.Fatal(err)
	}
	if _, err := store.SaveWorkbenchDocument(b.Context(), key, 0, raw); err != nil {
		b.Fatal(err)
	}
	service := accountworkbench.New(store)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := service.Maintenance(b.Context(), owner); err != nil {
			b.Fatal(err)
		}
	}
}
