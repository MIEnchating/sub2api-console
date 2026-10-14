package business_test

import (
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
)

func TestDatabaseContractProbeBindingPreservesDetectionIdentity(t *testing.T) {
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
			db, err := dbconn.Open(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := db.ExecContext(t.Context(), `INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('780042','detection-contract','{"platform":"openai","account_type":"oauth","base_url":"https://isolated.example"}','now')`); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if _, err := db.ExecContext(t.Context(), `DELETE FROM accounts WHERE id='780042'`); err != nil {
					t.Error(err)
				}
			}()
			value, err := store.ProbeAccountBinding(t.Context(), "780042")
			if err != nil {
				t.Fatal(err)
			}
			if value.ID != "780042" || value.Name != "detection-contract" || value.Platform == nil || *value.Platform != "openai" ||
				value.AccountType == nil || *value.AccountType != "oauth" || value.BaseURL == nil || *value.BaseURL != "https://isolated.example" || len(value.Bindings) != 0 {
				t.Fatalf("detection identity: %+v", value)
			}
		})
	}
}
