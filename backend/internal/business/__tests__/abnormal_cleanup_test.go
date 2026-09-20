package business_test

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

func TestAbnormalCleanupUpgradePreservesExistingAccounts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.sqlite3")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','preserved','{}','now'); DROP TABLE abnormal_cleanup_states`); err != nil {
		t.Fatal(err)
	}
	store, err = business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	account, err := store.Account(t.Context(), "41")
	if err != nil || account.Name != "preserved" {
		t.Fatalf("existing account changed: %+v %v", account, err)
	}
	states, err := store.AbnormalCleanupStates(t.Context(), nil)
	if err != nil || len(states) != 0 {
		t.Fatalf("missing isolated observation table: %v %v", states, err)
	}
}

func TestAbnormalCleanupPolicyValidationAndObservationReset(t *testing.T) {
	store, db := concurrencyStore(t)
	for _, invalid := range []map[string]any{
		{"duration_minutes": 0}, {"duration_minutes": 1.5}, {"duration_minutes": 525601}, {"action": "destroy"}, {"max_per_round": 0}, {"enabled": "yes"},
	} {
		if _, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"abnormal_cleanup": invalid}}, "test"); err == nil {
			t.Fatalf("accepted invalid settings: %v", invalid)
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO abnormal_cleanup_states VALUES('41',?,?); INSERT INTO cleanup_states VALUES('41',?,?)`, now, now, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"abnormal_cleanup": map[string]any{"enabled": true, "duration_minutes": 120, "action": "disable"}}}, "test"); err != nil {
		t.Fatal(err)
	}
	states, err := store.AbnormalCleanupStates(t.Context(), nil)
	if err != nil || len(states) != 0 {
		t.Fatalf("new policy inherited prior observation: %v %v", states, err)
	}
	auth, err := store.CleanupStates(t.Context(), nil)
	if err != nil || len(auth) != 1 {
		t.Fatalf("authentication state changed: %v %v", auth, err)
	}
	if _, err := db.Exec(`INSERT INTO abnormal_cleanup_states VALUES('41',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetMode(t.Context(), runtimepolicy.Monitoring); err != nil {
		t.Fatal(err)
	}
	states, err = store.AbnormalCleanupStates(t.Context(), nil)
	if err != nil || len(states) != 0 {
		t.Fatalf("monitoring mode retained actionable observation: %v %v", states, err)
	}
}

func TestAccountProtectionImmediatelyResetsAbnormalObservation(t *testing.T) {
	store, db := concurrencyStore(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','protected','{}','now'); INSERT INTO abnormal_cleanup_states VALUES('41',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	operation := business.AccountOperation{OperationID: "manual-pause", OperationType: "account.control", State: "succeeded", Phase: "readback", Actor: "test", ObjectID: "41", RemoteConfirmed: true, ReadbackConfirmed: true, Before: map[string]any{}, After: map[string]any{}}
	if err := store.CommitAccountControlReadback(t.Context(), "41", "pause", "test", false, operation); err != nil {
		t.Fatal(err)
	}
	states, err := store.AbnormalCleanupStates(t.Context(), nil)
	if err != nil || len(states) != 0 {
		t.Fatalf("manual protection retained old observation: %v %v", states, err)
	}
}
