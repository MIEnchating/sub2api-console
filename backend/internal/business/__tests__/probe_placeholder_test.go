package business_test

import (
	"encoding/json"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestProbePlaceholderMarkerSurvivesLaterCleanAndOlderSamples(t *testing.T) {
	store, db := concurrencyStore(t)
	seedCapacityAccounts(t, db)
	for _, sample := range []business.ProbeSample{
		{AccountID: "41", GroupName: "a", Result: "通过", ObservedAt: "2026-10-11T12:00:00Z", IgnoredPlaceholder: true},
		{AccountID: "41", GroupName: "a", Result: "通过", ObservedAt: "2026-10-11T13:00:00Z"},
		{AccountID: "41", GroupName: "b", Result: "失败", ObservedAt: "2026-10-11T11:00:00Z", IgnoredPlaceholder: true},
	} {
		if _, err := store.PersistProbeSamples(t.Context(), []business.ProbeSample{sample}); err != nil {
			t.Fatal(err)
		}
	}
	accounts, err := store.Accounts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range accounts {
		raw, _ := json.Marshal(account)
		var fields map[string]any
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		if account.ID == "41" {
			if fields["probe_placeholder_at"] != "2026-10-11T12:00:00.000000000Z" {
				t.Fatalf("missing durable latest marker: %s", raw)
			}
		} else if fields["probe_placeholder_at"] != nil {
			t.Fatalf("marker leaked to account %s", account.ID)
		}
	}
	detail, err := store.Account(t.Context(), "41")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(detail)
	var fields map[string]any
	_ = json.Unmarshal(raw, &fields)
	if fields["probe_placeholder_at"] != "2026-10-11T12:00:00.000000000Z" {
		t.Fatalf("detail lost marker: %s", raw)
	}
}

func TestDeletingMarkedAccountClearsMarkerBeforeIDReuse(t *testing.T) {
	store, db := concurrencyStore(t)
	seedCapacityAccounts(t, db)
	if _, err := store.PersistProbeSamples(t.Context(), []business.ProbeSample{{AccountID: "41", GroupName: "a", Result: "失败", ObservedAt: "2026-10-11T12:00:00Z", IgnoredPlaceholder: true}}); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteAccountProjection(t.Context(), "41", business.AccountOperation{OperationID: "delete-placeholder", OperationType: "account.delete", State: "succeeded", Phase: "readback", Actor: "test", RemoteConfirmed: true, ReadbackConfirmed: true, ObjectID: "41"}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM operational_snapshots WHERE namespace='probe-placeholder' AND state_key='41'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("deleted marker remains: %d, %v", count, err)
	}
}
