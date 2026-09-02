package business

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalOnboardingGroupIncludesManagementPlatform(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO local_groups(name,remote_id,platform,updated_at)
		VALUES('国模-平价','27','openai','now')`); err != nil {
		t.Fatal(err)
	}

	group, err := store.LocalOnboardingGroup(context.Background(), "27")
	if err != nil {
		t.Fatal(err)
	}
	if group.Platform == nil || *group.Platform != "openai" {
		t.Fatalf("local group platform=%v", group.Platform)
	}
}

func TestPendingOnboardingUsesCanonicalLocalSelectionAndRejectsDuplicateIdentity(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	seedOnboardingIdentity(t, store)

	marker := "operation-a"
	pending := PendingOnboarding{
		OperationID: "operation-a", UpstreamHost: "UPSTREAM.TEST", UpstreamType: "sub2api",
		UpstreamKeyName: &marker, UpstreamGroupID: "6", UpstreamGroupName: "pro",
		LocalGroupID: "2", LocalGroupName: "two", LocalGroupIDs: []string{"10", "2"},
		Multiplier: "0.2", IntentHash: strings.Repeat("a", 64), Reason: "test",
	}
	if err := store.SavePendingOnboarding(context.Background(), pending); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.PendingOnboarding(context.Background(), "https://relay.test/", "6", []string{"2", "10"})
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil || loaded.OperationID != pending.OperationID || strings.Join(loaded.LocalGroupIDs, ",") != "2,10" {
		t.Fatalf("loaded=%#v", loaded)
	}
	var storedSelection string
	if err := store.db.QueryRow(`SELECT local_group_ids_json FROM onboarding_pending WHERE operation_id=?`, pending.OperationID).Scan(&storedSelection); err != nil {
		t.Fatal(err)
	}
	if storedSelection != `["2","10"]` {
		t.Fatalf("stored selection=%q", storedSelection)
	}

	secondMarker := "operation-b"
	pending.OperationID = "operation-b"
	pending.UpstreamHost = "relay.test"
	pending.UpstreamKeyName = &secondMarker
	pending.LocalGroupIDs = []string{"2", "10"}
	if err := store.SavePendingOnboarding(context.Background(), pending); err == nil {
		t.Fatal("duplicate pending identity should violate the unique constraint")
	}
}

func TestPendingOnboardingRejectsMissingFrozenIntent(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	seedOnboardingIdentity(t, store)

	_, err = store.db.Exec(`INSERT INTO onboarding_pending(
		operation_id,upstream_host,upstream_type,upstream_key_id,upstream_key_name,upstream_group_id,upstream_group_name,
		local_group_id,local_group_name,local_group_ids_json,multiplier,intent_hash,reason,created_at,updated_at
	) VALUES('legacy','upstream.test','sub2api','91','legacy-marker','6','pro','3','codex','','0.2','','legacy','now','now')`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PendingOnboarding(context.Background(), "upstream.test", "6", []string{"3"}); err == nil || !strings.Contains(err.Error(), "缺少首次冻结意图") {
		t.Fatalf("error=%v", err)
	}
}

func TestPendingOnboardingRejectsIncompleteIdentityBeforeSelectingAnyRow(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	seedOnboardingIdentity(t, store)

	for _, values := range []struct {
		operationID string
		localID     string
	}{
		{operationID: "incomplete-a", localID: "3"},
		{operationID: "incomplete-b", localID: "4"},
	} {
		_, err := store.db.Exec(`INSERT INTO onboarding_pending(
			operation_id,upstream_host,upstream_type,upstream_key_id,upstream_key_name,upstream_group_id,upstream_group_name,
			local_group_id,local_group_name,local_group_ids_json,multiplier,intent_hash,reason,created_at,updated_at
		) VALUES(?,?,?,?,?,'6','pro',?,?,'','0.2',?,'incomplete','now','now')`,
			values.operationID, "upstream.test", "sub2api", "91", values.operationID, values.localID, values.localID, strings.Repeat("a", 64))
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.PendingOnboarding(context.Background(), "upstream.test", "6", []string{"4", "3"}); err == nil || !strings.Contains(err.Error(), "缺少规范化本地分组集合") {
		t.Fatalf("error=%v", err)
	}
}

func TestUpstreamIdentityMergeUpdatesPendingIdentity(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	seedSeparateOnboardingIdentities(t, store)

	marker := "operation-b"
	pending := PendingOnboarding{
		OperationID: "operation-b", UpstreamHost: "b.example", UpstreamType: "sub2api",
		UpstreamKeyName: &marker, UpstreamGroupID: "6", UpstreamGroupName: "pro",
		LocalGroupID: "3", LocalGroupName: "codex", LocalGroupIDs: []string{"3"},
		Multiplier: "0.2", IntentHash: strings.Repeat("b", 64), Reason: "test",
	}
	if err := store.SavePendingOnboarding(context.Background(), pending); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE upstreams SET metadata_json='{"alias_hosts":["b.example"]}' WHERE host='a.example'`); err != nil {
		t.Fatal(err)
	}
	if err := store.ensureUpstreamIdentities(context.Background()); err != nil {
		t.Fatal(err)
	}
	var upstreamID string
	if err := store.db.QueryRow(`SELECT upstream_id FROM onboarding_pending WHERE operation_id='operation-b'`).Scan(&upstreamID); err != nil {
		t.Fatal(err)
	}
	if upstreamID != "stable-old" {
		t.Fatalf("pending upstream ID=%q", upstreamID)
	}
	loaded, err := store.PendingOnboarding(context.Background(), "a.example", "6", []string{"3"})
	if err != nil || loaded == nil || loaded.OperationID != "operation-b" {
		t.Fatalf("loaded=%#v err=%v", loaded, err)
	}
}

func TestUpstreamIdentityMergeFailsClosedOnConflictingPendingRows(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	seedSeparateOnboardingIdentities(t, store)

	for _, values := range []struct {
		operationID string
		host        string
	}{
		{operationID: "operation-a", host: "a.example"},
		{operationID: "operation-b", host: "b.example"},
	} {
		marker := values.operationID
		if err := store.SavePendingOnboarding(context.Background(), PendingOnboarding{
			OperationID: values.operationID, UpstreamHost: values.host, UpstreamType: "sub2api",
			UpstreamKeyName: &marker, UpstreamGroupID: "6", UpstreamGroupName: "pro",
			LocalGroupID: "3", LocalGroupName: "codex", LocalGroupIDs: []string{"3"},
			Multiplier: "0.2", IntentHash: strings.Repeat("c", 64), Reason: "test",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.db.Exec(`UPDATE upstreams SET metadata_json='{"alias_hosts":["b.example"]}' WHERE host='a.example'`); err != nil {
		t.Fatal(err)
	}
	if err := store.ensureUpstreamIdentities(context.Background()); err == nil {
		t.Fatal("identity merge should fail instead of choosing one pending create intent")
	}
	var identities int
	if err := store.db.QueryRow(`SELECT COUNT(DISTINCT upstream_id) FROM onboarding_pending`).Scan(&identities); err != nil {
		t.Fatal(err)
	}
	if identities != 2 {
		t.Fatalf("pending identities=%d", identities)
	}
}

func TestCommitOnboardingProjectionAcceptsStableCreateWithoutReadback(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.db.Exec(`INSERT INTO upstreams(host,base_url,upstream_type,auth_status,metadata_json,updated_at)
		VALUES('upstream.test','https://upstream.test','sub2api','ready','{}','now')`); err != nil {
		t.Fatal(err)
	}
	if err := store.ensureUpstreamIdentities(context.Background()); err != nil {
		t.Fatal(err)
	}

	err = store.CommitOnboardingProjection(context.Background(), OnboardingProjection{
		OperationID: "operation-a", AccountID: "77", AccountName: "account", Platform: " OpenAI ",
		UpstreamHost: "upstream.test", UpstreamType: "sub2api", UpstreamKeyID: "91", UpstreamKeyName: "key",
		UpstreamGroupID: "6", UpstreamGroupName: "pro", LocalGroupID: "3", LocalGroupName: "codex",
		Multiplier: "0.2", ReadbackConfirmed: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	var accounts int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM accounts`).Scan(&accounts); err != nil {
		t.Fatal(err)
	}
	if accounts != 1 {
		t.Fatalf("accounts=%d", accounts)
	}
	var metadataRaw string
	if err := store.db.QueryRow(`SELECT metadata_json FROM accounts WHERE id='77'`).Scan(&metadataRaw); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(metadataRaw, `"platform":"openai"`) {
		t.Fatalf("confirmed platform was not projected: %s", metadataRaw)
	}
	var remoteConfirmed, readbackConfirmed int
	if err := store.db.QueryRow(`SELECT remote_confirmed,readback_confirmed FROM operation_audit WHERE operation_id='operation-a'`).Scan(&remoteConfirmed, &readbackConfirmed); err != nil {
		t.Fatal(err)
	}
	if remoteConfirmed != 1 || readbackConfirmed != 0 {
		t.Fatalf("remote_confirmed=%d readback_confirmed=%d", remoteConfirmed, readbackConfirmed)
	}
}

func seedOnboardingIdentity(t *testing.T, store *Store) {
	t.Helper()
	_, err := store.db.Exec(`INSERT INTO upstream_identities(upstream_id,created_at,updated_at)
		VALUES('stable-upstream','now','now');
		INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at)
		VALUES('upstream.test','stable-upstream',1,'now'),('relay.test','stable-upstream',0,'now')`)
	if err != nil {
		t.Fatal(err)
	}
}

func seedSeparateOnboardingIdentities(t *testing.T, store *Store) {
	t.Helper()
	_, err := store.db.Exec(`INSERT INTO upstreams(host,base_url,upstream_type,auth_status,metadata_json,updated_at)
		VALUES('a.example','https://a.example','sub2api','ready','{}','now'),
		('b.example','https://b.example','sub2api','ready','{}','now');
		INSERT INTO upstream_identities(upstream_id,created_at,updated_at)
		VALUES('stable-old','2026-01-01T00:00:00Z','now'),('stable-new','2026-02-01T00:00:00Z','now');
		INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at)
		VALUES('a.example','stable-old',1,'now'),('b.example','stable-new',1,'now')`)
	if err != nil {
		t.Fatal(err)
	}
}
