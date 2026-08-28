package alerting

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	_ "modernc.org/sqlite"
)

type fakeDeliverer struct{ result business.AlertDeliveryResult }

func (f fakeDeliverer) Deliver(context.Context, bool) (business.AlertDeliveryResult, error) {
	return f.result, nil
}

func TestEvaluationCreatesAndRecoversAuthIncident(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alerts.sqlite3")
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	if err := repository.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO upstreams(host,base_url,upstream_type,auth_status,metadata_json,updated_at)
		VALUES('api.example','https://api.example','sub2api','认证过期','{}','now')`); err != nil {
		t.Fatal(err)
	}
	service := New(repository, fakeDeliverer{result: business.AlertDeliveryResult{Skipped: 1, Configured: true, MessageIDs: []string{}}})
	result, err := service.Evaluate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Findings != 1 || result.RunKey == "" || result.EventID >= 0 || result.Status != "succeeded" {
		t.Fatalf("unexpected evaluation: %#v", result)
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM alert_incidents WHERE incident_key='console:auth:api.example'`).Scan(&status); err != nil || status != "firing" {
		t.Fatalf("auth incident not created: status=%q err=%v", status, err)
	}
	if _, err := db.Exec(`UPDATE upstreams SET auth_status='已鉴权' WHERE host='api.example'`); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Evaluate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT status FROM alert_incidents WHERE incident_key='console:auth:api.example'`).Scan(&status); err != nil || status != "recovered" {
		t.Fatalf("auth incident not recovered: status=%q err=%v", status, err)
	}
}
