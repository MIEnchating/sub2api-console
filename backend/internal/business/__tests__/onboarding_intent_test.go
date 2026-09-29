package business_test

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestVersionNineUpgradePreservesPendingWriteMarkers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pending-upgrade.sqlite3")
	store, err := business.Open(path)
	if err != nil {
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
	if _, err := db.Exec(`ALTER TABLE onboarding_pending DROP COLUMN frozen_intent_json;
		PRAGMA user_version=9;
		INSERT INTO onboarding_pending(operation_id,upstream_host,upstream_type,upstream_key_id,upstream_key_name,
		upstream_group_id,upstream_group_name,local_group_id,local_group_name,local_group_ids_json,multiplier,
		intent_hash,reason,key_commit_unknown,account_commit_unknown,created_at,updated_at)
		VALUES('cancelled','upstream.test','sub2api','91','original-marker','6','pro','3','codex','["3"]','0.2',
		'original-hash','cancelled',0,1,'original-time','original-time')`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		store, err = business.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		var key, marker, hash, snapshot, created string
		var unknown bool
		if err := db.QueryRow(`SELECT upstream_key_id,upstream_key_name,intent_hash,frozen_intent_json,account_commit_unknown,created_at FROM onboarding_pending WHERE operation_id='cancelled'`).Scan(&key, &marker, &hash, &snapshot, &unknown, &created); err != nil {
			t.Fatal(err)
		}
		if key != "91" || marker != "original-marker" || hash != "original-hash" || snapshot != "" || !unknown || created != "original-time" {
			t.Fatalf("upgrade changed pending operation: key=%s marker=%s hash=%s snapshot=%q unknown=%t created=%s", key, marker, hash, snapshot, unknown, created)
		}
	}
}

func TestPendingSnapshotCanBeBackfilledButCannotBeReplaced(t *testing.T) {
	store, err := business.Open(filepath.Join(t.TempDir(), "snapshot.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.CreateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{Host: "upstream.test", BaseURL: "https://upstream.test", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1"}); err != nil {
		t.Fatal(err)
	}
	marker := "original-marker"
	pending := business.PendingOnboarding{
		OperationID: "cancelled", UpstreamHost: "upstream.test", UpstreamType: "sub2api", UpstreamKeyName: &marker,
		UpstreamGroupID: "6", UpstreamGroupName: "pro", LocalGroupID: "3", LocalGroupName: "codex", LocalGroupIDs: []string{"3"},
		Multiplier: "0.2", IntentHash: strings.Repeat("a", 64),
	}
	if err := store.SavePendingOnboarding(t.Context(), pending); err != nil {
		t.Fatal(err)
	}
	pending.FrozenIntentJSON = `{"concurrency":7}`
	if err := store.SavePendingOnboarding(t.Context(), pending); err != nil {
		t.Fatal(err)
	}
	pending.FrozenIntentJSON = `{"concurrency":9}`
	pending.UpstreamKeyID = "91"
	if err := store.SavePendingOnboarding(t.Context(), pending); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.PendingOnboarding(t.Context(), "upstream.test", "6", []string{"3"})
	if err != nil || loaded == nil {
		t.Fatalf("load pending: %v %v", loaded, err)
	}
	if loaded.FrozenIntentJSON != `{"concurrency":7}` || loaded.UpstreamKeyID != "91" {
		t.Fatalf("snapshot replaced or write state lost: %+v", loaded)
	}
}
