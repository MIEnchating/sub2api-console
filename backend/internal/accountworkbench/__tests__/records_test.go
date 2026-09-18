package accountworkbench_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestRunDirectoryIsSessionScopedAndNeverReturnsPrivateSources(t *testing.T) {
	service, store := fixture(t, `{}`)
	ctx := context.Background()
	owner, _ := previewOwner(t, store)
	other, _ := previewOwner(t, store)
	id := "11111111111111111111111111111111"
	raw, _ := json.Marshal(map[string]any{"owner": owner, "public": accountworkbench.Run{ID: id, Status: "completed", Items: []accountworkbench.RunItem{{InputItem: accountworkbench.InputItem{ID: "row", Email: "owner@example.test"}, Status: "completed"}}}, "items": []any{map[string]any{"credentials": map[string]any{"refresh_token": "private-rotation"}, "login_source": "private-password"}}, "exports": []any{map[string]any{"credentials": "private-export"}}})
	if _, err := store.SaveWorkbenchDocument(ctx, "run:"+owner+":"+id, 0, raw); err != nil {
		t.Fatal(err)
	}
	records, err := service.Runs(ctx, owner)
	if err != nil || len(records) != 1 {
		t.Fatal("owner record unavailable", err)
	}
	public, _ := json.Marshal(records)
	if strings.Contains(string(public), "private-") || strings.Contains(string(public), owner) {
		t.Fatal("private execution data leaked")
	}
	records, err = service.Runs(ctx, other)
	if err != nil || len(records) != 0 {
		t.Fatal("another session saw record")
	}
	if _, err = service.Run(ctx, other, id); err == nil {
		t.Fatal("another session read record by ID")
	}
}
