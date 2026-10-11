package business_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestClearRuntimeBlocksPreservesMetadataCommittedAfterCallerSnapshot(t *testing.T) {
	store, db := pricingProjectionFixture(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at)
		VALUES('41','account','{"platform":"old","last_error":"blocked"}','before')`); err != nil {
		t.Fatal(err)
	}
	err := store.WithReadSnapshot(t.Context(), func(ctx context.Context) error {
		if _, err := db.Exec(`UPDATE accounts SET metadata_json='{"platform":"new","custom_flag":true,"last_error":"blocked"}' WHERE id='41'`); err != nil {
			return err
		}
		return store.ClearRoutingRuntimeBlocks(ctx, "41")
	})
	if err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := db.QueryRow(`SELECT metadata_json FROM accounts WHERE id='41'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["platform"] != "new" || metadata["custom_flag"] != true {
		t.Fatalf("committed metadata was overwritten: %s", raw)
	}
	if _, present := metadata["last_error"]; present {
		t.Fatalf("runtime block was retained: %s", raw)
	}
}

func TestClearRuntimeBlocksPreservesUnrelatedJSONNumbersExactly(t *testing.T) {
	store, db := pricingProjectionFixture(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at)
		VALUES('41','account','{"external_id":9007199254740993,"settings":{"ratio":0.1234567890123456789},"last_error":"blocked","rate_limit_reset_at":"later"}','before')`); err != nil {
		t.Fatal(err)
	}
	if err := store.ClearRoutingRuntimeBlocks(t.Context(), "41"); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := db.QueryRow(`SELECT metadata_json FROM accounts WHERE id='41'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		t.Fatal(err)
	}
	if string(metadata["external_id"]) != "9007199254740993" || string(metadata["settings"]) != `{"ratio":0.1234567890123456789}` {
		t.Fatalf("unrelated numbers changed: %s", raw)
	}
	if len(metadata) != 2 {
		t.Fatalf("runtime fields remain: %s", raw)
	}
}

func TestClearRuntimeBlocksRejectsInvalidMetadataWithoutChangingAccount(t *testing.T) {
	for _, raw := range []string{"null", "[]", "invalid", `{} {}`} {
		t.Run(raw, func(t *testing.T) {
			store, db := pricingProjectionFixture(t)
			if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','account',?,'before')`, raw); err != nil {
				t.Fatal(err)
			}
			if err := store.ClearRoutingRuntimeBlocks(t.Context(), "41"); err == nil || !strings.Contains(err.Error(), "metadata 配置无效") {
				t.Fatalf("invalid metadata error=%v", err)
			}
			var stored, updated string
			if err := db.QueryRow(`SELECT metadata_json,updated_at FROM accounts WHERE id='41'`).Scan(&stored, &updated); err != nil {
				t.Fatal(err)
			}
			if stored != raw || updated != "before" {
				t.Fatalf("invalid metadata was changed: %q %q", stored, updated)
			}
		})
	}
}

func TestClearRuntimeBlocksRejectsMissingInvalidAndCancelledTargets(t *testing.T) {
	store, _ := pricingProjectionFixture(t)
	if err := store.ClearRoutingRuntimeBlocks(t.Context(), "41"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing account error=%v", err)
	}
	if err := store.ClearRoutingRuntimeBlocks(t.Context(), "name"); err == nil {
		t.Fatal("account name accepted as stable ID")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := store.ClearRoutingRuntimeBlocks(ctx, "41"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled write error=%v", err)
	}
}
