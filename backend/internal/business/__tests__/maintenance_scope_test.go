package business_test

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func maintenanceScopeFixture(t testing.TB) (*business.Store, *sql.DB) {
	t.Helper()
	store, db := pricingProjectionFixture(t)
	for _, statement := range []string{
		`INSERT INTO upstreams(host,base_url,upstream_type,auth_status,metadata_json,updated_at)
		VALUES('maintenance.example','https://maintenance.example','sub2api','ready','{"site_name":"Maintenance"}','now')`,
		`INSERT INTO upstream_identities(upstream_id,created_at,updated_at) VALUES('fixture-upstream','now','now')`,
		`INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at)
		VALUES('maintenance.example','fixture-upstream',1,'now')`,
		`INSERT INTO accounts(id,name,multiplier,metadata_json,updated_at) VALUES
		('2','same-name','0.1','{"platform":" OpenAI "}','now'),
		('10','same-name','0.2','{}','now'),('30','unbound','0.3','{}','now')`,
		`INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,updated_at)
		VALUES('2','maintenance.example','first','fixture','default','now'),
		('2','maintenance.example','second','fixture','default','now'),
		('10','maintenance.example','third','fixture','default','now')`,
		`INSERT INTO binding_identities(binding_id,upstream_id,upstream_key_id,updated_at)
		SELECT id,'fixture-upstream',upstream_key_id,'now' FROM bindings`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	return store, db
}

func TestMaintenanceScopePreservesSelectionAndNumericOrder(t *testing.T) {
	store, _ := maintenanceScopeFixture(t)
	for _, fixture := range []struct {
		name      string
		ids       []string
		wantNames map[string]string
		wantBound []string
	}{
		{"nil-all", nil, map[string]string{"2": "same-name", "10": "same-name", "30": "unbound"}, []string{"2", "2", "10"}},
		{"empty-all", []string{}, map[string]string{"2": "same-name", "10": "same-name", "30": "unbound"}, []string{"2", "2", "10"}},
		{"single", []string{"2"}, map[string]string{"2": "same-name"}, []string{"2", "2"}},
		{"trim-deduplicate-sort", []string{"10", "\u20032\t", "2", "30", "missing"}, map[string]string{"2": "same-name", "10": "same-name", "30": "unbound"}, []string{"2", "2", "10"}},
		{"unbound", []string{"30"}, map[string]string{"30": "unbound"}, []string{}},
		{"blank-not-all", []string{" "}, map[string]string{}, []string{}},
		{"literal-not-expression", []string{"2' OR 1=1 --", "same-name"}, map[string]string{}, []string{}},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			names, err := store.AccountNamesForMaintenance(t.Context(), fixture.ids)
			if err != nil || !reflect.DeepEqual(names, fixture.wantNames) {
				t.Fatalf("names=%v err=%v want=%v", names, err, fixture.wantNames)
			}
			bound, err := store.BoundAccountsForMaintenance(t.Context(), fixture.ids)
			if err != nil {
				t.Fatal(err)
			}
			ids := make([]string, 0, len(bound))
			for _, item := range bound {
				ids = append(ids, item.AccountID)
			}
			if !reflect.DeepEqual(ids, fixture.wantBound) {
				t.Fatalf("bound IDs=%v want=%v", ids, fixture.wantBound)
			}
		})
	}
}

func TestMaintenanceScopeSkipsUnselectedCorruptMetadataButRejectsSelected(t *testing.T) {
	store, db := maintenanceScopeFixture(t)
	if _, err := db.Exec(`UPDATE accounts SET metadata_json='invalid' WHERE id='10'`); err != nil {
		t.Fatal(err)
	}
	bound, err := store.BoundAccountsForMaintenance(t.Context(), []string{"2"})
	if err != nil || len(bound) != 2 || bound[0].Platform != "openai" {
		t.Fatalf("bound=%+v err=%v", bound, err)
	}
	for _, ids := range [][]string{nil, {"10"}, {"2", "10"}} {
		if _, err := store.BoundAccountsForMaintenance(t.Context(), ids); err == nil || !strings.Contains(err.Error(), "账号 10 的元数据记录损坏") {
			t.Fatalf("ids=%v corrupt selected metadata error=%v", ids, err)
		}
	}
	if _, err := db.Exec(`UPDATE upstreams SET metadata_json='null'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.BoundAccountsForMaintenance(t.Context(), []string{"2"}); err == nil || !strings.Contains(err.Error(), "上游 maintenance.example 的元数据记录损坏") {
		t.Fatalf("corrupt selected upstream metadata error=%v", err)
	}
}

func TestMaintenanceScopeLargeSelectionReturnsEveryRequestedAccount(t *testing.T) {
	store, db := maintenanceScopeFixture(t)
	if _, err := db.Exec(`WITH RECURSIVE n(x) AS (VALUES(100) UNION ALL SELECT x+1 FROM n WHERE x<1199)
	INSERT INTO accounts(id,name,updated_at) SELECT CAST(x AS TEXT),'bulk','now' FROM n`); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 1100)
	for i := range ids {
		ids[i] = strconv.Itoa(i + 100)
	}
	names, err := store.AccountNamesForMaintenance(t.Context(), ids)
	if err != nil || len(names) != 1100 || names["100"] != "bulk" || names["1199"] != "bulk" {
		t.Fatalf("count=%d err=%v", len(names), err)
	}
	bound, err := store.BoundAccountsForMaintenance(t.Context(), append(ids, "2"))
	if err != nil || len(bound) != 2 || bound[0].AccountID != "2" || bound[1].AccountID != "2" {
		t.Fatalf("bound=%+v err=%v", bound, err)
	}
}

func TestMaintenanceScopeCancelledReadReturnsCancellation(t *testing.T) {
	store, _ := maintenanceScopeFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.AccountNamesForMaintenance(ctx, []string{"2"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("names error=%v", err)
	}
	if _, err := store.BoundAccountsForMaintenance(ctx, []string{"2"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("bindings error=%v", err)
	}
}
