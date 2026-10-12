package business_test

import "testing"

func TestMissingBindingCleanupRequiresConfirmedUnambiguousCatalog(t *testing.T) {
	for _, scenario := range []string{"confirmed-key", "confirmed-group", "suspected", "active", "sync-failed", "auth-failed", "ambiguous", "identity-mismatch", "unconfirmed"} {
		t.Run(scenario, func(t *testing.T) {
			store, db := concurrencyStore(t)
			_, err := db.Exec(`UPDATE upstreams SET metadata_json=json_object('catalog_status','已同步');
    INSERT INTO bindings(id,local_account_id,upstream_host,upstream_key_id,upstream_key_name,upstream_group_id,local_group,updated_at) VALUES(1,'41','fixture.example','10','key','65','group','now');
    INSERT INTO binding_identities(binding_id,upstream_id,upstream_key_id,upstream_group_id,updated_at) SELECT 1,upstream_id,'10','65','now' FROM upstream_identity_hosts WHERE host='fixture.example';
    INSERT INTO upstream_catalog_entities(upstream_id,entity_kind,entity_id,parent_entity_id,name,lifecycle_state,missing_observations,confirmed_missing_at,updated_at) SELECT upstream_id,'key','10','65','key','missing',4,'2026-10-12T00:00:00Z','now' FROM upstream_identity_hosts WHERE host='fixture.example';
    INSERT INTO upstream_catalog_entities(upstream_id,entity_kind,entity_id,name,lifecycle_state,updated_at) SELECT upstream_id,'group','65','group','active','now' FROM upstream_identity_hosts WHERE host='fixture.example'`)
			if err != nil {
				t.Fatal(err)
			}
			changes := map[string]string{
				"confirmed-group":   `UPDATE upstream_catalog_entities SET lifecycle_state='active' WHERE entity_kind='key'; UPDATE upstream_catalog_entities SET lifecycle_state='missing',missing_observations=4,confirmed_missing_at='2026-10-12T00:00:00Z' WHERE entity_kind='group'`,
				"suspected":         `UPDATE upstream_catalog_entities SET lifecycle_state='suspected',missing_observations=1 WHERE entity_kind='key'`,
				"active":            `UPDATE upstream_catalog_entities SET lifecycle_state='active'`,
				"sync-failed":       `UPDATE upstreams SET metadata_json=json_object('catalog_status','同步失败')`,
				"auth-failed":       `UPDATE upstreams SET auth_status='鉴权失效'`,
				"ambiguous":         `INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,updated_at) VALUES('41','fixture.example','11','other','group','now')`,
				"identity-mismatch": `UPDATE bindings SET upstream_key_id='11'`,
				"unconfirmed":       `UPDATE upstream_catalog_entities SET confirmed_missing_at=NULL`,
			}
			if change := changes[scenario]; change != "" {
				if _, err := db.Exec(change); err != nil {
					t.Fatal(err)
				}
			}
			id := "41"
			bindings, err := store.ConfirmedMissingBindings(t.Context(), &id)
			if err != nil {
				t.Fatal(err)
			}
			want := scenario == "confirmed-key" || scenario == "confirmed-group"
			if (bindings[id] != "") != want {
				t.Fatalf("unexpected deletion eligibility: %v", bindings)
			}
			if want {
				before := bindings[id]
				if _, err := db.Exec(`UPDATE bindings SET upstream_group_id='66'; UPDATE binding_identities SET upstream_group_id='66'`); err != nil {
					t.Fatal(err)
				}
				after, err := store.ConfirmedMissingBindings(t.Context(), &id)
				if err != nil || after[id] == before {
					t.Fatalf("rebinding retained old authorization: %v %v", after, err)
				}
			}
		})
	}
}

func TestBindingCleanupPolicyDefaultsValidationAndPersistence(t *testing.T) {
	store, _ := concurrencyStore(t)
	snapshot, err := store.PolicySnapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	section := snapshot.AdvancedPolicy["binding_cleanup"].(map[string]any)
	if section["enabled"] != false {
		t.Fatal("destructive feature enabled by default")
	}
	for _, invalid := range []map[string]any{{"enabled": "yes"}, {"max_per_round": 0}, {"max_per_round": 1.5}, {"max_per_round": 10001}} {
		if _, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"binding_cleanup": invalid}}, "test"); err == nil {
			t.Fatalf("accepted %v", invalid)
		}
	}
	if _, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"binding_cleanup": map[string]any{"enabled": true, "max_per_round": 3}}}, "test"); err != nil {
		t.Fatal(err)
	}
	snapshot, err = store.PolicySnapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	section = snapshot.AdvancedPolicy["binding_cleanup"].(map[string]any)
	if section["enabled"] != true || section["max_per_round"] != int64(3) {
		t.Fatalf("settings not persisted: %v", section)
	}
}
