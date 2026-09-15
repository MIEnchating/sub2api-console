package configstore_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestWorkbenchExecutionCleanupSummaryOmitsCredentialsAndDeleteChecksTargetAndRevision(t *testing.T) {
	store, err := configstore.Open(filepath.Join(t.TempDir(), "config.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	input := configstore.WorkbenchExecution{ID: "cleanup-execution", TargetURL: "https://target.example", TargetFingerprint: strings.Repeat("a", 64), Items: []configstore.WorkbenchExecutionItem{{Index: 0, AccountID: "101", Identity: `identity:["workspace-101","user-101"]`, Credentials: json.RawMessage(`{"access_token":"private-execution-token"}`), Snapshot: json.RawMessage(`{"credentials":{"refresh_token":"rt_private_snapshot"}}`)}}}
	if err := store.SaveWorkbenchExecution(ctx, input); err != nil {
		t.Fatal(err)
	}
	items, err := store.WorkbenchExecutionSummaries(ctx, input.TargetFingerprint)
	if err != nil || len(items) != 1 || items[0].Revision != 1 || items[0].Items[0].AccountID != "101" {
		t.Fatalf("cleanup summary = %+v %v", items, err)
	}
	raw, _ := json.Marshal(items)
	if strings.Contains(string(raw), "private-execution-token") || strings.Contains(string(raw), "rt_private_snapshot") {
		t.Fatal("cleanup summary selected credentials")
	}
	if err := store.DeleteWorkbenchExecution(ctx, strings.Repeat("b", 64), input.ID, 1); !errors.Is(err, configstore.ErrWorkbenchExecution) {
		t.Fatal("wrong target deleted execution", err)
	}
	if err := store.DeleteWorkbenchExecution(ctx, input.TargetFingerprint, input.ID, 2); !errors.Is(err, configstore.ErrWorkbenchExecution) {
		t.Fatal("stale revision deleted execution", err)
	}
	if err := store.DeleteWorkbenchExecution(ctx, input.TargetFingerprint, input.ID, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := store.WorkbenchExecution(ctx, input.ID); !errors.Is(err, configstore.ErrWorkbenchExecution) {
		t.Fatal("cleanup execution remained", err)
	}
}
