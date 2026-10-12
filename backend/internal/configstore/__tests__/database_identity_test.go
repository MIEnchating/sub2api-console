package configstore_test

import (
	"encoding/json"
	"testing"
)

func TestDatabaseContractWorkbenchUsesExactBytePrefixes(t *testing.T) {
	for _, config := range databaseConfigs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store := openContractStore(t, config)
			for _, prefix := range []string{"bytes:\x7f", "bytes:\xff\xff", "bytes:文", "bytes:%_"} {
				key := prefix + "item"
				if _, err := store.SaveWorkbenchDocument(t.Context(), key, 0, json.RawMessage(`{"public":{}}`)); err != nil {
					t.Fatal(err)
				}
				rows, err := store.WorkbenchDocumentPage(t.Context(), prefix, "", 2)
				if err != nil || len(rows) != 1 || rows[0].ID != key {
					t.Fatalf("byte prefix %q: %v %v", prefix, rows, err)
				}
			}
		})
	}
}
