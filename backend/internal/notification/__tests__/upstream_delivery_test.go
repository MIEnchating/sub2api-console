package notification_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/notification"
)

func TestSameUpstreamAlertsDeliverOneMessageAndPersistEveryIncident(t *testing.T) {
	fixture := newUpstreamDeliveryFixture(t)

	result, err := fixture.service.Deliver(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Sent != 3 || result.Batches != 1 || len(fixture.sender.messages) != 1 {
		t.Fatalf("same upstream alerts were not merged: result=%+v messages=%v", result, fixture.sender.messages)
	}
	if len(result.MessageIDs) != 1 || result.MessageIDs[0] != "message-1" {
		t.Fatalf("merged delivery must return its single message ID: %+v", result.MessageIDs)
	}
	for _, expected := range []string{"upstream.example", "上游鉴权失败", "上游余额不足", "上游倍率同步失败"} {
		if !strings.Contains(fixture.sender.messages[0], expected) {
			t.Errorf("merged delivery omitted %q: %s", expected, fixture.sender.messages[0])
		}
	}
	var sent int
	if err := fixture.db.QueryRow(`SELECT COUNT(*) FROM alert_deliveries d JOIN alert_incidents i USING(incident_key)
		WHERE d.status='sent' AND d.attempts=1 AND i.delivery_status='已发送'`).Scan(&sent); err != nil {
		t.Fatal(err)
	}
	if sent != 3 {
		t.Fatalf("merged message must preserve each incident's delivery record: sent=%d", sent)
	}
}

func TestDeliveredUpstreamAlertsAreSkippedOnNextDelivery(t *testing.T) {
	fixture := newUpstreamDeliveryFixture(t)
	if _, err := fixture.service.Deliver(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	messageCount := len(fixture.sender.messages)

	result, err := fixture.service.Deliver(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Attempted != 0 || result.Skipped != 3 || len(fixture.sender.messages) != messageCount {
		t.Fatalf("merged incidents were sent again: result=%+v messages=%v", result, fixture.sender.messages)
	}
}

func TestFailedUpstreamMessageRetriesAllIncidentsTogether(t *testing.T) {
	fixture := newUpstreamDeliveryFixture(t)
	fixture.sender.fail = true
	failed, err := fixture.service.Deliver(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Failed != 3 || failed.Batches != 1 {
		t.Fatalf("one failed upstream message must fail all its incidents: %+v", failed)
	}
	var failedCount int
	if err := fixture.db.QueryRow(`SELECT COUNT(*) FROM alert_deliveries d JOIN alert_incidents i USING(incident_key)
		WHERE d.status='failed' AND d.attempts=1 AND i.delivery_status='发送失败' AND d.last_error='测试发送失败'`).Scan(&failedCount); err != nil {
		t.Fatal(err)
	}
	if failedCount != 3 {
		t.Fatalf("failure was not persisted for every incident: failed=%d", failedCount)
	}
	fixture.sender.fail = false

	retried, err := fixture.service.Deliver(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if retried.Sent != 3 || retried.Batches != 1 || len(fixture.sender.messages) != 2 {
		t.Fatalf("failed incidents were not retried in one message: result=%+v messages=%v", retried, fixture.sender.messages)
	}
	var retriedCount int
	if err := fixture.db.QueryRow(`SELECT COUNT(*) FROM alert_deliveries d JOIN alert_incidents i USING(incident_key)
		WHERE d.status='sent' AND d.attempts=2 AND d.last_error IS NULL AND i.delivery_status='已发送'`).Scan(&retriedCount); err != nil {
		t.Fatal(err)
	}
	if retriedCount != 3 {
		t.Fatalf("retry did not update every incident: retried=%d", retriedCount)
	}
}

func TestBalanceDeliveryLeavesOtherTypesAndOtherUpstreamsPending(t *testing.T) {
	fixture := newUpstreamDeliveryFixture(t)
	ctx := context.Background()
	if _, err := fixture.db.ExecContext(ctx, `INSERT INTO upstreams(host,base_url,upstream_type,auth_status,balance,metadata_json,updated_at)
		VALUES('other.example','https://other.example','sub2api','已鉴权',1,'{}','2026-09-14T10:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.repository.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}

	result, err := fixture.service.DeliverBalance(ctx, "upstream.example")
	if err != nil {
		t.Fatal(err)
	}
	if result.Sent != 1 || result.Batches != 1 || len(fixture.sender.messages) != 1 {
		t.Fatalf("unexpected scoped balance delivery: %+v", result)
	}
	message := fixture.sender.messages[0]
	if !strings.Contains(message, "上游余额不足") || !strings.Contains(message, "upstream.example") {
		t.Fatalf("balance notification omitted its target: %s", message)
	}
	for _, unrelated := range []string{"other.example", "上游鉴权失败", "上游倍率同步失败"} {
		if strings.Contains(message, unrelated) {
			t.Errorf("scoped balance delivery included %q: %s", unrelated, message)
		}
	}
	plan, err := fixture.repository.PrepareAlertDelivery(ctx, business.NotificationChannelKey("qqbot", "test-target"), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Pending) != 3 || plan.Skipped != 1 {
		t.Fatalf("scoped delivery consumed unrelated incidents: %+v", plan)
	}
	for _, incident := range plan.Pending {
		if incident.ObjectID == "upstream.example" && incident.EventType == "upstream.balance" {
			t.Fatalf("delivered balance remains pending: %+v", incident)
		}
	}
}

type upstreamDeliveryFixture struct {
	repository *business.Store
	db         *sql.DB
	service    *notification.Service
	sender     *upstreamMessageSender
}

func newUpstreamDeliveryFixture(t *testing.T) upstreamDeliveryFixture {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "upstream-alerts.sqlite3")
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	if err := repository.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.ExecContext(ctx, `INSERT INTO operational_snapshots(namespace,state_key,value_json,updated_at)
		VALUES('sub2api','sub2api-notify-rules.json','{"enabled":true,"channels":[{"type":"qqbot","enabled":true}]}','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO upstreams(host,base_url,upstream_type,auth_status,balance,metadata_json,updated_at)
		VALUES('upstream.example','https://upstream.example','sub2api','认证过期',1,'{"rate_sync_status":"failed"}','2026-09-14T10:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}
	sender := &upstreamMessageSender{}
	return upstreamDeliveryFixture{
		repository: repository, db: db, sender: sender,
		service: notification.New(repository, upstreamDeliverySettings{}, sender),
	}
}

type upstreamDeliverySettings struct{}

func (upstreamDeliverySettings) NotificationSettings(context.Context) (configstore.NotificationSettings, error) {
	return configstore.NotificationSettings{
		AppID: "test-app", ClientSecret: "test-secret", HomeChannel: "test-target", HomeChannelType: "c2c",
	}, nil
}

type upstreamMessageSender struct {
	messages []string
	fail     bool
}

func (s *upstreamMessageSender) Send(_ context.Context, _ configstore.NotificationSettings, messages []string) []notification.SendOutcome {
	outcomes := make([]notification.SendOutcome, 0, len(messages))
	for _, message := range messages {
		s.messages = append(s.messages, message)
		outcome := notification.SendOutcome{Detail: "测试发送失败"}
		if !s.fail {
			messageID := fmt.Sprintf("message-%d", len(s.messages))
			outcome = notification.SendOutcome{Success: true, Detail: "测试发送成功", MessageID: &messageID}
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes
}
