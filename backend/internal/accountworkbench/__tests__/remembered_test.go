package accountworkbench_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestRememberedAccountsSurviveSessionRevocationAndRequireCurrentVersionToDelete(t *testing.T) {
	f := importedRememberedFixture(t, false)
	service, store, owner, token := f.service, f.store, f.owner, f.token
	records, err := service.RememberedAccounts(t.Context(), owner)
	if err != nil || len(records) != 1 || records[0].AccountID != "41" {
		t.Fatalf("remembered import: %v %+v", err, records)
	}
	raw, _ := json.Marshal(records)
	if strings.Contains(string(raw), "rt_run-private") || strings.Contains(string(raw), "access_token") {
		t.Fatal("credentials exposed")
	}
	if err := store.RevokeSession(t.Context(), token); err != nil {
		t.Fatal(err)
	}
	if err := service.Cleanup(t.Context()); err != nil {
		t.Fatal(err)
	}
	nextOwner, _ := previewOwner(t, store)
	records, err = service.RememberedAccounts(t.Context(), nextOwner)
	if err != nil || len(records) != 1 {
		t.Fatalf("saved accounts disappeared after login: %v %+v", err, records)
	}
	if err := service.DeleteRememberedAccount(t.Context(), nextOwner, records[0].ID, records[0].Revision+1); err == nil {
		t.Fatal("stale deletion accepted")
	}
	if err := service.DeleteRememberedAccount(t.Context(), owner, records[0].ID, records[0].Revision); err == nil {
		t.Fatal("revoked session accepted")
	}
	if err := store.ConfigureTarget(context.Background(), "https://other.invalid", "test-other", 10); err != nil {
		t.Fatal(err)
	}
	other, err := service.RememberedAccounts(t.Context(), nextOwner)
	if err != nil || len(other) != 0 {
		t.Fatalf("other target sees saved account: %v %+v", err, other)
	}
	if err := service.DeleteRememberedAccount(t.Context(), nextOwner, records[0].ID, records[0].Revision); err == nil {
		t.Fatal("other target deleted saved account")
	}
	if err := store.ConfigureTarget(context.Background(), "https://isolated.invalid", "test-admin", 10); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteRememberedAccount(t.Context(), nextOwner, records[0].ID, records[0].Revision); err != nil {
		t.Fatal(err)
	}
	records, err = service.RememberedAccounts(t.Context(), nextOwner)
	if err != nil || len(records) != 0 {
		t.Fatalf("deleted record retained: %v", err)
	}
}
