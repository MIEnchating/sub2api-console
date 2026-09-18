package routing_test

import (
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func TestOutOfScopeAccountOnlyReceivesReleaseTargetWithCapturedBaseline(t *testing.T) {
	for _, scope := range []string{"account_type", "group", "account"} {
		for _, captured := range []bool{false, true} {
			name := scope + "/never-managed"
			if captured {
				name = scope + "/previously-managed"
			}
			t.Run(name, func(t *testing.T) {
				store, db := controlScopeStore(t)
				accountType := "apikey"
				scopePolicy := map[string]any{"account_types": []any{"apikey"}}
				policy := map[string]any{"advanced_policy": map[string]any{"scope": scopePolicy}}
				switch scope {
				case "account_type":
					accountType = "oauth"
				case "group":
					policy["excluded_group_ids"] = []any{"7"}
				case "account":
					scopePolicy["excluded_account_ids"] = []any{"1169"}
				}
				_, err := db.Exec(`INSERT INTO accounts(id,name,multiplier,schedulable,metadata_json,updated_at)
					VALUES('1169','outside-scope','1',1,json_object('type',?),'now');
					INSERT INTO account_groups(account_id,group_name,group_id) VALUES('1169','codex','7')`, accountType)
				if err != nil {
					t.Fatal(err)
				}
				if captured {
					if _, err := db.Exec(`INSERT INTO routing_baselines(account_id,captured_at) VALUES('1169','now')`); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := store.UpdatePolicy(t.Context(), policy, "test"); err != nil {
					t.Fatal(err)
				}
				result, err := routing.NewService(store).Calculate(t.Context(), routing.Scope{}, true)
				if err != nil {
					t.Fatal(err)
				}
				target, found := result.AccountTargets["1169"]
				if found != captured || (found && !target.ReleaseControl) {
					t.Fatalf("only previously managed accounts need a release target: captured=%v targets=%+v", captured, result.AccountTargets)
				}
				if scope == "account_type" && (result.Accounts != 0 || result.HealthEvaluations != 0) {
					t.Fatalf("OAuth account entered API key scoring: %+v", result)
				}
			})
		}
	}
}
