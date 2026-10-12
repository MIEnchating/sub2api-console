package business_test

import (
	"reflect"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
)

func TestDatabaseContractJSONScopesPreserveOrderAndDeduplicateMemberships(t *testing.T) {
	for _, config := range dbtest.Configs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store, err := business.OpenDatabase(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			if err := store.Bootstrap(t.Context()); err != nil {
				t.Fatal(err)
			}
			groups := []map[string]any{{"id": "91", "name": "scope-first"}, {"id": "92", "name": "scope-second"}}
			accounts := []map[string]any{{"id": "81", "name": "scope-a", "group_ids": []any{"91", "92"}}, {"id": "82", "name": "scope-b", "group_ids": []any{"92"}}}
			if _, err := store.SyncManagementSnapshot(t.Context(), accounts, groups, "contract"); err != nil {
				t.Fatal(err)
			}
			defer store.SyncCompleteManagementSnapshot(t.Context(), nil, nil, "contract")
			scope, err := store.DetectionTaskScope(t.Context(), []string{"92", "91", "92"})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(scope.AccountIDs, []string{"81", "82"}) || !reflect.DeepEqual(scope.GroupIDsByAccount["81"], []string{"91", "92"}) {
				t.Fatalf("scope: %+v", scope)
			}
			models, err := store.AccountModelCatalogs(t.Context(), []string{"82", "81"})
			if err != nil {
				t.Fatal(err)
			}
			if len(models) != 2 || models[0].AccountID != "82" || models[1].AccountID != "81" {
				t.Fatalf("catalog order: %+v", models)
			}
			protections, err := store.AccountMutationProtections(t.Context(), []string{"82", "81", "82"})
			if err != nil || len(protections) != 2 {
				t.Fatalf("protections: %+v %v", protections, err)
			}
			if _, err := store.DetectionTaskScope(t.Context(), []string{"missing"}); err == nil {
				t.Fatal("missing group accepted")
			}
		})
	}
}
