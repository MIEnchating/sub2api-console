package configstore_test

import (
	"encoding/json"
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"testing"
)

func TestDatabaseContractWorkbenchPreservesJSONAndInsertionOrder(t *testing.T) {
	for _, config := range databaseConfigs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store := openContractStore(t, config)
			raw := json.RawMessage(`{"public":{"number":9007199254740993},"private":"fixture"}`)
			for _, id := range []string{"contract:文:b", "contract:文:a"} {
				if _, err := store.SaveWorkbenchDocument(t.Context(), id, 0, raw); err != nil {
					t.Fatal(err)
				}
			}
			got, revision, err := store.WorkbenchDocument(t.Context(), "contract:文:b")
			if err != nil || revision != 1 || string(got) != string(raw) {
				t.Fatalf("original JSON: %s %d %v", got, revision, err)
			}
			public, _, err := store.WorkbenchDocumentPublic(t.Context(), "contract:文:b")
			if err != nil {
				t.Fatal(err)
			}
			var summary map[string]json.RawMessage
			if err := json.Unmarshal(public, &summary); err != nil || string(summary["number"]) != "9007199254740993" {
				t.Fatalf("public JSON precision: %s %v", public, err)
			}
			list, err := store.WorkbenchDocuments(t.Context(), "contract:文:", 2)
			if err != nil || len(list) != 2 || list[0].ID != "contract:文:a" {
				t.Fatalf("insertion order: %v %v", list, err)
			}
			page, err := store.WorkbenchDocumentPage(t.Context(), "contract:文:", "", 2)
			if err != nil || len(page) != 2 || page[0].ID != "contract:文:a" {
				t.Fatalf("key page: %v %v", page, err)
			}
			if _, err := store.SaveWorkbenchDocument(t.Context(), "contract:文:b", 0, raw); !errors.Is(err, configstore.ErrWorkbenchVersion) {
				t.Fatalf("duplicate document: %v", err)
			}
		})
	}
}
