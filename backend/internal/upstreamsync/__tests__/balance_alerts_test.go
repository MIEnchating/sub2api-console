package upstreamsync_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/alerting"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/notification"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

type balanceReader struct {
	read func(context.Context, string) (business.UpstreamBalanceObservation, error)
}

func (r balanceReader) ReadBalance(ctx context.Context, auth configstore.AuthRecord) (business.UpstreamBalanceObservation, error) {
	return r.read(ctx, auth.Host)
}

func (balanceReader) ReadCatalog(context.Context, configstore.AuthRecord) (business.UpstreamCatalogSnapshot, error) {
	return business.UpstreamCatalogSnapshot{}, nil
}

type privateRecords struct{}

func (privateRecords) AuthRecord(_ context.Context, host string) (*configstore.AuthRecord, error) {
	return &configstore.AuthRecord{Host: host, AuthMode: "token", UpstreamType: "sub2api"}, nil
}

func (privateRecords) SaveAuthRecord(context.Context, configstore.AuthRecord, map[string]bool) error {
	return errors.New("unexpected credential write")
}

type notificationSettings struct{}

func (notificationSettings) NotificationSettings(context.Context) (configstore.NotificationSettings, error) {
	return configstore.NotificationSettings{AppID: "test-app", ClientSecret: "test-secret", HomeChannel: "test-target", HomeChannelType: "c2c"}, nil
}

type messageSender struct {
	mu       sync.Mutex
	messages []string
	fail     bool
	sent     chan string
}

func (s *messageSender) Send(_ context.Context, _ configstore.NotificationSettings, messages []string) []notification.SendOutcome {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, messages...)
	if s.sent != nil {
		for _, message := range messages {
			s.sent <- message
		}
	}
	outcomes := make([]notification.SendOutcome, len(messages))
	for i := range outcomes {
		outcomes[i] = notification.SendOutcome{Success: !s.fail, Detail: "test delivery"}
	}
	return outcomes
}

func TestBatchBalanceSyncSendsRecoveryWhileAnotherHostIsStillReading(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	slowStarted := make(chan struct{})
	releaseSlow := make(chan struct{})
	fixture := newBalanceFixture(t, balanceReader{read: func(ctx context.Context, host string) (business.UpstreamBalanceObservation, error) {
		if host == "slow.example" {
			close(slowStarted)
			select {
			case <-releaseSlow:
			case <-ctx.Done():
				return business.UpstreamBalanceObservation{}, ctx.Err()
			}
		} else {
			select {
			case <-slowStarted:
			case <-ctx.Done():
				return business.UpstreamBalanceObservation{}, ctx.Err()
			}
		}
		balance := "100"
		return business.UpstreamBalanceObservation{RawBalance: &balance, Status: "已读取"}, nil
	}}, "recharged.example", "slow.example")
	fixture.sender.sent = make(chan string, 4)
	done := make(chan error, 1)
	go func() {
		_, err := fixture.syncer.SyncAllNow(ctx, upstreamsync.Scope{Balance: true}, "test")
		done <- err
	}()
	defer func() {
		close(releaseSlow)
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	select {
	case message := <-fixture.sender.sent:
		if !strings.Contains(message, "recharged.example") || !strings.Contains(message, "上游余额已恢复") {
			t.Fatalf("unexpected notification before slow Host completes: %s", message)
		}
	case <-ctx.Done():
		t.Fatal("recovery was blocked by another Host's balance request")
	}
}

func TestBalanceSyncDoesNotReconcileOrDeliverUnrelatedIncidents(t *testing.T) {
	fixture := newBalanceFixture(t, balanceReader{read: func(context.Context, string) (business.UpstreamBalanceObservation, error) {
		balance := "100"
		return business.UpstreamBalanceObservation{RawBalance: &balance, Status: "已读取"}, nil
	}}, "recharged.example", "other.example")
	if _, err := fixture.db.Exec(`INSERT INTO alert_incidents VALUES('pending-routing','account.routing_breaker','account','41','ROUTING_BREAKER','firing','before','before',NULL,NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.db.Exec(`UPDATE upstreams SET balance=100,mapped_balance='100' WHERE host='other.example'`); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.syncer.SyncHost(context.Background(), "recharged.example", upstreamsync.Scope{Balance: true}, "test"); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"console:balance:other.example:5", "pending-routing"} {
		var status string
		if err := fixture.db.QueryRow(`SELECT status FROM alert_incidents WHERE incident_key=?`, key).Scan(&status); err != nil || status != "firing" {
			t.Fatalf("unrelated incident was changed: key=%s status=%s err=%v", key, status, err)
		}
	}
	if len(fixture.sender.messages) != 1 || !strings.Contains(fixture.sender.messages[0], "recharged.example") {
		t.Fatalf("balance sync delivered unrelated incidents: %v", fixture.sender.messages)
	}
}

func TestFailedBalanceReadDoesNotEmitRecoveryFromAnOldBalance(t *testing.T) {
	fixture := newBalanceFixture(t, balanceReader{read: func(context.Context, string) (business.UpstreamBalanceObservation, error) {
		return business.UpstreamBalanceObservation{}, errors.New("balance request failed")
	}}, "recharged.example")
	result, err := fixture.syncer.SyncHost(context.Background(), "recharged.example", upstreamsync.Scope{Balance: true}, "test")
	if err != nil || result.Status != "failed" {
		t.Fatalf("unexpected sync result=%+v err=%v", result, err)
	}
	if len(fixture.sender.messages) != 0 {
		t.Fatalf("failed read emitted recovery: %v", fixture.sender.messages)
	}
}

func TestNotificationFailurePreservesSyncedBalanceAndNextInspectionRetries(t *testing.T) {
	fixture := newBalanceFixture(t, balanceReader{read: func(context.Context, string) (business.UpstreamBalanceObservation, error) {
		balance := "100"
		return business.UpstreamBalanceObservation{RawBalance: &balance, Status: "已读取"}, nil
	}}, "recharged.example")
	fixture.sender.fail = true
	result, err := fixture.syncer.SyncHost(context.Background(), "recharged.example", upstreamsync.Scope{Balance: true}, "test")
	if err != nil || result.Status != "succeeded" || result.Balance == nil || *result.Balance != "100" {
		t.Fatalf("notification failure invalidated balance: result=%+v err=%v", result, err)
	}
	var deliveryStatus string
	if err := fixture.db.QueryRow(`SELECT status FROM alert_deliveries WHERE incident_key='console:balance:recharged.example:5'`).Scan(&deliveryStatus); err != nil || deliveryStatus != "failed" {
		t.Fatalf("failed notification was not recorded: status=%s err=%v", deliveryStatus, err)
	}
	fixture.sender.fail = false
	if _, err := fixture.alerts.Evaluate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(fixture.sender.messages) != 2 || !strings.Contains(fixture.sender.messages[1], "上游余额已恢复") {
		t.Fatalf("next evaluation did not retry recovery: %v", fixture.sender.messages)
	}
}

type balanceFixture struct {
	syncer *upstreamsync.Service
	alerts *alerting.Service
	store  *business.Store
	db     *sql.DB
	sender *messageSender
}

func newBalanceFixture(t *testing.T, reader balanceReader, hosts ...string) balanceFixture {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "balance-alerts.sqlite3")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(10000)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`INSERT INTO operational_snapshots(namespace,state_key,value_json,updated_at)
		VALUES('sub2api','sub2api-notify-rules.json','{"enabled":true,"channels":[{"type":"qqbot","enabled":true}]}','now')`); err != nil {
		t.Fatal(err)
	}
	for _, host := range hosts {
		if _, err := db.Exec(`INSERT INTO upstreams(host,base_url,upstream_type,auth_status,balance,raw_balance,mapped_balance,metadata_json,updated_at)
			VALUES(?,?,'sub2api','已鉴权',1,'1','1','{}','2026-09-09T03:00:00Z')`, host, "https://"+host); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO upstream_identities(upstream_id,created_at,updated_at) VALUES(?,'before','before')`, host); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at) VALUES(?,?,1,'before')`, host, host); err != nil {
			t.Fatal(err)
		}
	}
	sender := &messageSender{}
	alerts := alerting.New(store, notification.New(store, notificationSettings{}, sender))
	if _, err := alerts.Evaluate(ctx); err != nil {
		t.Fatal(err)
	}
	if len(sender.messages) != len(hosts) {
		t.Fatalf("fixture must first deliver the low balance alerts: %v", sender.messages)
	}
	sender.messages = nil
	syncer := upstreamsync.New(store, privateRecords{}, reader, nil, nil)
	syncer.UseBalanceAlerts(alerts.EvaluateBalance)
	return balanceFixture{syncer: syncer, alerts: alerts, store: store, db: db, sender: sender}
}

func TestBalanceRecoveryRespectsDisabledNotificationPolicies(t *testing.T) {
	for _, policyField := range []string{"enabled", "balance_enabled", "delivery_enabled", "notify_recovery"} {
		t.Run(policyField, func(t *testing.T) {
			fixture := newBalanceFixture(t, balanceReader{read: func(context.Context, string) (business.UpstreamBalanceObservation, error) {
				balance := "100"
				return business.UpstreamBalanceObservation{RawBalance: &balance, Status: "已读取"}, nil
			}}, "recharged.example")
			encoded, err := json.Marshal(business.DefaultAlertPolicy())
			if err != nil {
				t.Fatal(err)
			}
			var policy map[string]any
			if err := json.Unmarshal(encoded, &policy); err != nil {
				t.Fatal(err)
			}
			policy[policyField] = false
			if _, err := fixture.store.UpdateAlertPolicy(context.Background(), policy); err != nil {
				t.Fatal(err)
			}
			result, err := fixture.syncer.SyncHost(context.Background(), "recharged.example", upstreamsync.Scope{Balance: true}, "test")
			if err != nil || result.Status != "succeeded" {
				t.Fatalf("sync failed: result=%+v err=%v", result, err)
			}
			if len(fixture.sender.messages) != 0 {
				t.Fatalf("disabled %s still sent a notification: %v", policyField, fixture.sender.messages)
			}
		})
	}
}

func TestRechargeRemainingBelowThresholdSendsNewThresholdAlertInsteadOfRecovery(t *testing.T) {
	fixture := newBalanceFixture(t, balanceReader{read: func(context.Context, string) (business.UpstreamBalanceObservation, error) {
		balance := "8"
		return business.UpstreamBalanceObservation{RawBalance: &balance, Status: "已读取"}, nil
	}}, "recharged.example")
	result, err := fixture.syncer.SyncHost(context.Background(), "recharged.example", upstreamsync.Scope{Balance: true}, "test")
	if err != nil || result.Status != "succeeded" {
		t.Fatalf("sync failed: result=%+v err=%v", result, err)
	}
	if len(fixture.sender.messages) != 1 || !strings.Contains(fixture.sender.messages[0], "余额达到或低于 10") || strings.Contains(fixture.sender.messages[0], "已恢复") {
		t.Fatalf("partial recharge emitted false recovery: %v", fixture.sender.messages)
	}
}

func TestCatalogOnlySyncDoesNotEvaluateRecoveryUsingExistingBalance(t *testing.T) {
	fixture := newBalanceFixture(t, balanceReader{read: func(context.Context, string) (business.UpstreamBalanceObservation, error) {
		return business.UpstreamBalanceObservation{}, errors.New("balance read must not be used")
	}}, "recharged.example")
	if _, err := fixture.db.Exec(`UPDATE upstreams SET balance=100,raw_balance='100',mapped_balance='100'`); err != nil {
		t.Fatal(err)
	}
	result, err := fixture.syncer.SyncHost(context.Background(), "recharged.example", upstreamsync.Scope{Catalog: true}, "test")
	if err != nil || result.Status != "succeeded" {
		t.Fatalf("catalog sync failed: result=%+v err=%v", result, err)
	}
	if len(fixture.sender.messages) != 0 {
		t.Fatalf("catalog-only sync used an old balance to send recovery: %v", fixture.sender.messages)
	}
}

func TestBalanceSyncAfterRechargeSendsRecoveryWithoutAnotherInspection(t *testing.T) {
	fixture := newBalanceFixture(t, balanceReader{read: func(context.Context, string) (business.UpstreamBalanceObservation, error) {
		balance := "100"
		return business.UpstreamBalanceObservation{RawBalance: &balance, Status: "已读取"}, nil
	}}, "recharged.example")

	result, err := fixture.syncer.SyncHost(context.Background(), "recharged.example", upstreamsync.Scope{Balance: true}, "test")
	if err != nil || result.Status != "succeeded" {
		t.Fatalf("balance sync failed: result=%+v err=%v", result, err)
	}
	if len(fixture.sender.messages) != 1 || !strings.Contains(fixture.sender.messages[0], "上游余额已恢复") {
		t.Fatalf("successful balance sync must send recovery before another inspection; messages=%v", fixture.sender.messages)
	}
	if _, err := fixture.alerts.Evaluate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(fixture.sender.messages) != 1 {
		t.Fatalf("later inspection duplicated recovery: %v", fixture.sender.messages)
	}
}
