package business_test

import (
	"testing"
	"time"
)

func TestExcludedUpstreamIsOmittedFromTaskDirectories(t *testing.T) {
	store, db := concurrencyStore(t)
	_, err := db.Exec(`INSERT INTO accounts(id,name,upstream_host,updated_at) VALUES('41','excluded','fixture.example','now'),('42','included','other.example','now');
 INSERT INTO local_groups(name,remote_id,updated_at) VALUES('group','7','now');
 INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','group','7'),('42','group','7');
 UPDATE upstreams SET auth_status='鉴权失效' WHERE host='fixture.example'`)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := store.UpstreamExclusionPreview(t.Context(), "fixture.example")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetUpstreamExcluded(t.Context(), preview.UpstreamID, true, "test"); err != nil {
		t.Fatal(err)
	}
	t.Run("model automation", func(t *testing.T) {
		ids, err := store.AccountIDsByGroups(t.Context(), []string{"7"})
		if err != nil || len(ids) != 1 || ids[0] != "42" {
			t.Fatalf("task scope: %v %v", ids, err)
		}
	})
	t.Run("detection", func(t *testing.T) {
		scope, err := store.DetectionTaskScope(t.Context(), []string{"7"})
		if err != nil || len(scope.AccountIDs) != 1 || scope.AccountIDs[0] != "42" {
			t.Fatalf("detection scope: %+v %v", scope, err)
		}
	})
	t.Run("upstream sync", func(t *testing.T) {
		targets, err := store.UpstreamSyncTargets(t.Context(), nil)
		if err != nil || len(targets.Hosts) != 0 {
			t.Fatalf("sync scope: %+v %v", targets, err)
		}
	})
	t.Run("auth recovery", func(t *testing.T) {
		hosts, err := store.AuthRecoveryRequiredHosts(t.Context(), time.Now())
		if err != nil || len(hosts) != 0 {
			t.Fatalf("auth scope: %v %v", hosts, err)
		}
	})
	t.Run("group count", func(t *testing.T) {
		groups, err := store.GroupMembershipOptions(t.Context())
		if err != nil || len(groups) != 1 || groups[0].AccountCount != 1 {
			t.Fatalf("group scope: %+v %v", groups, err)
		}
	})
	if err := store.SetUpstreamExcluded(t.Context(), preview.UpstreamID, false, "test"); err != nil {
		t.Fatal(err)
	}
	ids, err := store.AccountIDsByGroups(t.Context(), []string{"7"})
	if err != nil || len(ids) != 2 {
		t.Fatalf("restored scope: %v %v", ids, err)
	}
}
