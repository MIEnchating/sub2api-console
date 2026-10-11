package business_test

import (
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestMissingUpstreamClosesLegacyAlertsWithoutRecovery(t *testing.T) {
	for _, scoped := range []bool{false, true} {
		for _, status := range []string{"firing", "suppressed", "recovered"} {
			name := "full/" + status
			if scoped {
				name = "balance/" + status
			}
			t.Run(name, func(t *testing.T) {
				store, db := openAlertRuleStore(t)
				if _, err := db.Exec(`INSERT INTO alert_incidents(incident_key,event_type,object_kind,object_id,cause_code,status,first_seen_at,last_seen_at)
					VALUES('deleted-balance','upstream.balance','host','deleted.example','BALANCE:5',?,'before','before')`, status); err != nil {
					t.Fatal(err)
				}
				var err error
				if scoped {
					_, err = store.EvaluateBalanceAlertIncidents(t.Context(), "deleted.example")
				} else {
					_, err = store.EvaluateAlertIncidents(t.Context())
				}
				if err != nil {
					t.Fatal(err)
				}
				var actual, detail string
				if err := db.QueryRow(`SELECT status,delivery_status FROM alert_incidents WHERE incident_key='deleted-balance'`).Scan(&actual, &detail); err != nil {
					t.Fatal(err)
				}
				if actual != "closed" || detail != "上游已删除，告警已关闭" {
					t.Fatalf("missing upstream retained alert: status=%q detail=%q", actual, detail)
				}
			})
		}
	}
}

func TestDeletingUpstreamClosesAliasAlertsBeforeNextEvaluation(t *testing.T) {
	store, db := openAlertRuleStore(t)
	for _, statement := range []string{
		`INSERT INTO upstreams(host,base_url,upstream_type,auth_status,metadata_json,updated_at) VALUES
			('primary.example','https://primary.example','sub2api','已鉴权','{}','before'),
			('alias.example','https://alias.example','sub2api','已鉴权','{}','before'),
			('retained.example','https://retained.example','sub2api','已鉴权','{}','before')`,
		`INSERT INTO upstream_identities(upstream_id,created_at,updated_at) VALUES('deleted-id','before','before'),('retained-id','before','before')`,
		`INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at) VALUES
			('primary.example','deleted-id',1,'before'),('alias.example','deleted-id',0,'before'),('retained.example','retained-id',1,'before')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	for _, host := range []string{"primary.example", "alias.example", "retained.example"} {
		for _, event := range []string{"upstream.balance", "upstream.auth", "upstream.configuration", "upstream.rate_sync"} {
			if _, err := db.Exec(`INSERT INTO alert_incidents(incident_key,event_type,object_kind,object_id,cause_code,status,first_seen_at,last_seen_at)
				VALUES(?,?,'host',?,'fixture','firing','before','before')`, host+event, event, host); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := store.DeleteUpstreamProjection(t.Context(), "primary.example", nil, business.UpstreamDeleteAudit{}); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"primary.example", "alias.example", "retained.example"} {
		want := "closed"
		if host == "retained.example" {
			want = "firing"
		}
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM alert_incidents WHERE object_id=? AND status=?`, host, want).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 4 {
			t.Fatalf("host %s: got %d alerts in state %s, want 4", host, count, want)
		}
	}
}
