package business

import (
	"context"
	"testing"
)

func TestGroupProbeModelsReturnsModelsSharedByAccountsWithSyncedCatalogs(t *testing.T) {
	store := openReadModelFixture(t)
	ctx := context.Background()
	statements := []string{
		`UPDATE accounts SET metadata_json='{"known_models":["gpt-5.2","gpt-4.1","gpt-5.2"]}' WHERE id='41'`,
		`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES
			('43','second','{"known_models":["claude-sonnet-4-6","gpt-4.1"]}','now'),
			('44','not-synced','{}','now')`,
		`INSERT INTO account_groups(account_id,group_name,group_id,group_rate) VALUES
			('43','codex','1','0.1'),('44','codex','1','0.1')`,
	}
	for _, statement := range statements {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}

	result, err := store.GroupProbeModels(ctx, "1")
	if err != nil {
		t.Fatal(err)
	}
	if result.GroupID != "1" || result.GroupName != "codex" {
		t.Fatalf("unexpected group identity: %#v", result)
	}
	if result.AccountCount != 3 || result.AccountsWithModels != 2 || result.Complete {
		t.Fatalf("unexpected model coverage: %#v", result)
	}
	if len(result.Models) != 1 || result.Models[0] != "gpt-4.1" {
		t.Fatalf("unexpected shared models: %#v", result.Models)
	}
}
