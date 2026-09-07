package business

import (
	"context"
	"strings"
	"testing"
)

func TestModelCheckConfigurationPersistsJSONAndWritesRedactedAudit(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	raw := []byte(`{"active":{"id":"profile-1"},"secret_marker":"must-not-enter-audit"}`)
	if err := store.SaveModelCheckConfiguration(ctx, raw, "operator", "draft.saved"); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadModelCheckConfiguration(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if string(loaded) != string(raw) {
		t.Fatalf("loaded=%s", loaded)
	}
	var operationType, actor string
	var before, after *string
	if err := store.db.QueryRow(`SELECT operation_type,actor,before_json,after_json FROM operation_audit
		WHERE object_type='model-check-profile' ORDER BY source_id LIMIT 1`).Scan(&operationType, &actor, &before, &after); err != nil {
		t.Fatal(err)
	}
	if operationType != "model-check.profile.draft.saved" || actor != "operator" || before != nil || after == nil ||
		strings.Contains(*after, "secret_marker") || strings.Contains(*after, "must-not-enter-audit") {
		t.Fatalf("audit type=%q actor=%q before=%v after=%v", operationType, actor, before, after)
	}
}
