package business_test

import (
	"encoding/json"
	"testing"
)

func TestAccountReadModelsExposeUpstreamExclusionWithoutChangingScheduling(t *testing.T) {
	store, db := concurrencyStore(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,upstream_host,schedulable,updated_at) VALUES('41','excluded','fixture.example',1,'now'),('42','available','other.example',1,'now')`); err != nil {
		t.Fatal(err)
	}
	preview, err := store.UpstreamExclusionPreview(t.Context(), "fixture.example")
	if err != nil {
		t.Fatal(err)
	}
	for _, excluded := range []bool{true, false} {
		if err := store.SetUpstreamExcluded(t.Context(), preview.UpstreamID, excluded, "test"); err != nil {
			t.Fatal(err)
		}
		accounts, err := store.Accounts(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if len(accounts) != 2 {
			t.Fatalf("directory lost accounts: %d", len(accounts))
		}
		for _, account := range accounts {
			data, err := json.Marshal(account)
			if err != nil {
				t.Fatal(err)
			}
			var payload map[string]any
			if err := json.Unmarshal(data, &payload); err != nil {
				t.Fatal(err)
			}
			if payload["upstream_excluded"] != (excluded && account.ID == "41") {
				t.Fatalf("account %s exclusion: %v", account.ID, payload["upstream_excluded"])
			}
			if account.Schedulable == nil || !*account.Schedulable {
				t.Fatal("exclusion changed scheduling")
			}
		}
		state, err := store.AccountControlState(t.Context(), "41")
		if err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(state)
		if err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["upstream_excluded"] != excluded {
			t.Fatalf("control exclusion: %v", payload["upstream_excluded"])
		}
	}
}
