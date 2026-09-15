package business_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"
)

func TestDisablingBreakerRuleRemovesBindingInvalidNotifications(t *testing.T) {
	for _, status := range []string{"firing", "recovered"} {
		t.Run(status, func(t *testing.T) {
			store, db := openAlertRuleStore(t)
			ctx := context.Background()
			if _, err := db.ExecContext(ctx, `INSERT INTO alert_incidents(
				incident_key,event_type,object_kind,object_id,cause_code,status,first_seen_at,last_seen_at
			) VALUES('binding-41','account.binding_invalid','account','41','BINDING_INVALID',?,'2026-09-01T00:00:00Z','2026-09-01T00:00:00Z')`, status); err != nil {
				t.Fatal(err)
			}
			policy := business.DefaultAlertPolicy()
			policy.RoutingBreakerEnabled = false
			policy.RecoveryNotificationTypes = []string{"routing_breaker"}
			updateAlertRulePolicy(t, store, policy)
			queue, err := store.NotificationQueueDetails(ctx, "fixture-channel", true)
			if err != nil {
				t.Fatal(err)
			}
			if len(queue.ConsumerItems) != 0 {
				t.Fatalf("disabled binding-invalid rule retained notification: %#v", queue.ConsumerItems)
			}
		})
	}
}

func TestDisablingDegradedSubtypeClosesPendingRecovery(t *testing.T) {
	for _, cause := range []string{"ROUTING_DEGRADED_LATENCY:延迟超标", "ROUTING_DEGRADED:旧版原因"} {
		t.Run(cause, func(t *testing.T) {
			store, db := openAlertRuleStore(t)
			ctx := context.Background()
			if _, err := db.ExecContext(ctx, `INSERT INTO alert_incidents(
				incident_key,event_type,object_kind,object_id,cause_code,status,first_seen_at,last_seen_at
			) VALUES('degraded-41','account.routing_degraded','account','41',?,'recovered','2026-09-01T00:00:00Z','2026-09-01T00:00:00Z')`, cause); err != nil {
				t.Fatal(err)
			}
			policy := business.DefaultAlertPolicy()
			policy.RoutingDegradedTypes = []string{"health_score"}
			policy.RecoveryNotificationTypes = []string{"routing_degraded"}
			updateAlertRulePolicy(t, store, policy)
			queue, err := store.NotificationQueueDetails(ctx, "fixture-channel", true)
			if err != nil {
				t.Fatal(err)
			}
			if len(queue.ConsumerItems) != 0 {
				t.Fatalf("disabled degraded subtype retained recovery: %#v", queue.ConsumerItems)
			}
		})
	}
}

func openAlertRuleStore(t *testing.T) (*business.Store, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "alert-rules.db")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", sqliteutil.DSN(path, ""))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return store, db
}

func updateAlertRulePolicy(t *testing.T, store *business.Store, policy business.AlertPolicy) {
	t.Helper()
	raw, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateAlertPolicy(context.Background(), document); err != nil {
		t.Fatal(err)
	}
}
