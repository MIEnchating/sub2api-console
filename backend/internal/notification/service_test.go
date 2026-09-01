package notification

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	_ "modernc.org/sqlite"
)

type responseRoundTripper struct {
	responses []string
	urls      []string
}

func (r *responseRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	r.urls = append(r.urls, request.URL.String())
	response := r.responses[0]
	r.responses = r.responses[1:]
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(response)),
		Header:     make(http.Header),
		Request:    request,
	}, nil
}

func TestQQBotTokenIsRequestedOnceForAWholeMessageBatch(t *testing.T) {
	transport := &responseRoundTripper{responses: []string{
		`{"access_token":"batch-token"}`,
		`{"id":"message-1"}`,
		`{"id":"message-2"}`,
	}}
	sender := NewQQBotSender(&http.Client{Transport: transport})
	outcomes := sender.Send(context.Background(), configstore.NotificationSettings{
		AppID: "app", ClientSecret: "secret", HomeChannel: "target", HomeChannelType: "c2c",
	}, []string{"first", "second"})

	if len(outcomes) != 2 || !outcomes[0].Success || !outcomes[1].Success {
		t.Fatalf("unexpected outcomes: %#v", outcomes)
	}
	tokenRequests := 0
	for _, endpoint := range transport.urls {
		if endpoint == tokenEndpoint {
			tokenRequests++
		}
	}
	if tokenRequests != 1 || len(transport.urls) != 3 {
		t.Fatalf("token must be reused for the batch: %#v", transport.urls)
	}
}

func TestQQBotCanonicalNullMessageIDDoesNotReviveLegacyID(t *testing.T) {
	transport := &responseRoundTripper{responses: []string{
		`{"access_token":"batch-token"}`,
		`{"id":null,"message_id":"legacy-id"}`,
	}}
	sender := NewQQBotSender(&http.Client{Transport: transport})
	outcomes := sender.Send(context.Background(), configstore.NotificationSettings{
		AppID: "app", ClientSecret: "secret", HomeChannel: "target", HomeChannelType: "c2c",
	}, []string{"test"})
	if len(outcomes) != 1 || outcomes[0].Success || outcomes[0].Detail != "QQBot 发送响应缺少消息 ID" {
		t.Fatalf("unexpected outcome: %#v", outcomes)
	}
}

func TestQQBotSenderCopiesClientAndRejectsRedirects(t *testing.T) {
	originalRedirect := func(*http.Request, []*http.Request) error { return nil }
	client := &http.Client{Timeout: 3 * time.Second, CheckRedirect: originalRedirect}
	sender := NewQQBotSender(client)

	if sender.client == client || sender.client.Timeout != client.Timeout {
		t.Fatalf("sender did not copy the supplied client: sender=%p client=%p", sender.client, client)
	}
	if client.CheckRedirect == nil || client.CheckRedirect(nil, nil) != nil {
		t.Fatal("constructor mutated the caller-owned redirect policy")
	}
	if err := sender.client.CheckRedirect(nil, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("sender follows credential-bearing POST redirects: %v", err)
	}
}

func TestQQBotSenderRejectsOversizedResponse(t *testing.T) {
	transport := &responseRoundTripper{responses: []string{
		`{"access_token":"batch-token"}` + strings.Repeat(" ", maximumQQBotResponseSize),
	}}
	sender := NewQQBotSender(&http.Client{Transport: transport})
	outcomes := sender.Send(context.Background(), configstore.NotificationSettings{
		AppID: "app", ClientSecret: "secret", HomeChannel: "target", HomeChannelType: "c2c",
	}, []string{"test"})

	if len(outcomes) != 1 || outcomes[0].Success || outcomes[0].Detail != "QQBot 响应过大" {
		t.Fatalf("oversized response was accepted: %#v", outcomes)
	}
}

func TestAlertDeliverySendsSmallAlertSetsSeparatelyAndDoesNotHoldTransactionDuringSend(t *testing.T) {
	path := createAlertDatabase(t)
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repository.Close() })
	settings := &staticSettings{value: configstore.NotificationSettings{
		AppID: "app", ClientSecret: "secret", HomeChannel: "target", HomeChannelType: "c2c",
	}}
	sender := &concurrentWriteSender{path: path}
	service := New(repository, settings, sender)

	result, err := service.Deliver(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Sent != 2 || result.Failed != 0 || result.Batches != 2 {
		t.Fatalf("unexpected delivery result: %#v", result)
	}
	if len(sender.messages) != 2 || !strings.Contains(sender.messages[0], "Sub2API · 告警通知") ||
		!strings.Contains(sender.messages[0], "| 项目 | 内容 |") ||
		!strings.Contains(sender.messages[0], "first.example") || !strings.Contains(sender.messages[1], "second.example") {
		t.Fatalf("small alert set was not sent separately: %#v", sender.messages)
	}
	database, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var marker string
	if err := database.QueryRow(`SELECT value_json FROM app_state WHERE key='network-callback'`).Scan(&marker); err != nil {
		t.Fatalf("concurrent DB write during network callback failed: %v", err)
	}
	var sent int
	if err := database.QueryRow(`SELECT COUNT(*) FROM alert_deliveries WHERE status='sent'`).Scan(&sent); err != nil || sent != 2 {
		t.Fatalf("delivery results not persisted: sent=%d err=%v", sent, err)
	}
}

func TestZeroRepeatIntervalDoesNotResendPersistentIncidents(t *testing.T) {
	path := createAlertDatabase(t)
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	settings := &staticSettings{value: configstore.NotificationSettings{
		AppID: "app", ClientSecret: "secret", HomeChannel: "target", HomeChannelType: "c2c",
	}}
	sender := &concurrentWriteSender{path: path}
	service := New(repository, settings, sender)

	first, err := service.Deliver(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Deliver(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if first.Sent != 2 || second.Sent != 0 || second.Skipped != 2 || len(sender.messages) != 2 {
		t.Fatalf("zero repeat interval resent persistent incidents: first=%#v second=%#v messages=%d", first, second, len(sender.messages))
	}
}

func TestStateChangeCooldownDelaysFlappingNotification(t *testing.T) {
	path := createAlertDatabase(t)
	channelKey := business.NotificationChannelKey("qqbot", "target")
	database, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := database.Exec(`UPDATE alert_incidents SET status='recovered' WHERE incident_key='incident-1';
		INSERT INTO alert_deliveries(incident_key,channel_key,status,attempts,delivered_at,updated_at)
		VALUES('incident-1',?,'transition',1,?,?)`, channelKey, now, now); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	settings := &staticSettings{value: configstore.NotificationSettings{
		AppID: "app", ClientSecret: "secret", HomeChannel: "target", HomeChannelType: "c2c",
	}}
	sender := &concurrentWriteSender{path: path}
	service := New(repository, settings, sender)

	cooling, err := service.Deliver(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if cooling.Sent != 1 || cooling.Skipped != 1 || len(sender.messages) != 1 {
		t.Fatalf("cooldown did not suppress the changed incident: result=%#v messages=%#v", cooling, sender.messages)
	}
	details, err := repository.NotificationQueueDetails(context.Background(), channelKey, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(details.ConsumerItems) != 2 || details.ConsumerItems[0].QueueStatus != "状态变化冷却中" {
		t.Fatalf("cooldown queue state is missing: %#v", details.ConsumerItems)
	}

	old := time.Now().UTC().Add(-31 * time.Minute).Format(time.RFC3339Nano)
	verification, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verification.ExecContext(context.Background(), `UPDATE alert_deliveries SET delivered_at=? WHERE incident_key='incident-1'`, old); err != nil {
		verification.Close()
		t.Fatal(err)
	}
	stillCooling, err := service.Deliver(context.Background(), false)
	if err != nil {
		verification.Close()
		t.Fatal(err)
	}
	if stillCooling.Sent != 0 || stillCooling.Skipped != 2 || len(sender.messages) != 1 {
		verification.Close()
		t.Fatalf("old delivery time bypassed the latest transition cooldown: result=%#v messages=%#v", stillCooling, sender.messages)
	}
	if _, err := verification.ExecContext(context.Background(), `UPDATE alert_deliveries SET updated_at=? WHERE incident_key='incident-1'`, old); err != nil {
		verification.Close()
		t.Fatal(err)
	}
	if err := verification.Close(); err != nil {
		t.Fatal(err)
	}
	ready, err := service.Deliver(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if ready.Sent != 1 || len(sender.messages) != 2 {
		t.Fatalf("changed incident was not sent after cooldown: result=%#v messages=%#v", ready, sender.messages)
	}
}

func TestNewRoutingDegradedIncidentWaitsForStateChangeCooldown(t *testing.T) {
	path := createAlertDatabase(t)
	database, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := database.Exec(`INSERT INTO alert_incidents VALUES(
		'console:routing:degraded:41:codex','account.routing_degraded','account','41',
		'ROUTING_DEGRADED:健康分下降','firing',?,?,NULL,NULL)`,
		now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	channelKey := business.NotificationChannelKey("qqbot", "target")

	cooling, err := repository.PrepareAlertDelivery(context.Background(), channelKey, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(cooling.Pending) != 2 || cooling.Skipped != 1 {
		t.Fatalf("new degraded incident bypassed observation window: %#v", cooling)
	}

	verification, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	old := now.Add(-31 * time.Minute).Format(time.RFC3339Nano)
	if _, err := verification.Exec(`UPDATE alert_incidents SET first_seen_at=?
		WHERE incident_key='console:routing:degraded:41:codex'`, old); err != nil {
		verification.Close()
		t.Fatal(err)
	}
	if err := verification.Close(); err != nil {
		t.Fatal(err)
	}

	ready, err := repository.PrepareAlertDelivery(context.Background(), channelKey, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(ready.Pending) != 3 || ready.Skipped != 0 {
		t.Fatalf("stable degraded incident was not released after cooldown: %#v", ready)
	}
}

func TestRoutingDegradedDeliveriesShareChannelDigestCooldown(t *testing.T) {
	path := createAlertDatabase(t)
	channelKey := business.NotificationChannelKey("qqbot", "target")
	database, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	stable := now.Add(-31 * time.Minute).Format(time.RFC3339Nano)
	recent := now.Format(time.RFC3339Nano)
	if _, err := database.Exec(`DELETE FROM alert_incidents;
		INSERT INTO alert_incidents VALUES
		('console:routing:degraded:41:alpha','account.routing_degraded','account','41','ROUTING_DEGRADED:健康分下降','closed',?,?,NULL,NULL),
		('console:routing:degraded:42:beta','account.routing_degraded','account','42','ROUTING_DEGRADED:延迟超标','firing',?,?,NULL,NULL),
		('auth-now','upstream.auth','host','api.example','AUTH','firing',?,?,NULL,NULL)`,
		stable, stable, stable, stable, recent, recent); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO alert_deliveries(
		incident_key,channel_key,status,attempts,delivered_at,updated_at
	) VALUES('console:routing:degraded:41:alpha',?,'sent',1,?,?)`, channelKey, recent, recent); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })

	cooling, err := repository.PrepareAlertDelivery(context.Background(), channelKey, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(cooling.Pending) != 1 || cooling.Pending[0].IncidentKey != "auth-now" || cooling.Skipped != 1 {
		t.Fatalf("recent degraded delivery did not hold later degraded changes only: %#v", cooling)
	}
	details, err := repository.NotificationQueueDetails(context.Background(), channelKey, true)
	if err != nil {
		t.Fatal(err)
	}
	var delayed *business.NotificationQueueItem
	for index := range details.ConsumerItems {
		if details.ConsumerItems[index].IncidentKey == "console:routing:degraded:42:beta" {
			delayed = &details.ConsumerItems[index]
			break
		}
	}
	if delayed == nil || delayed.QueueStatus != "降级告警汇总冷却中" ||
		!strings.Contains(delayed.QueueReason, "统一汇总发送") {
		t.Fatalf("digest cooldown is not visible in queue details: %#v", details.ConsumerItems)
	}

	database, err = sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE alert_deliveries SET delivered_at=?,updated_at=?
		WHERE incident_key='console:routing:degraded:41:alpha'`, stable, stable); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	ready, err := repository.PrepareAlertDelivery(context.Background(), channelKey, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(ready.Pending) != 2 || ready.Skipped != 0 {
		t.Fatalf("pending degraded changes were not released after digest cooldown: %#v", ready)
	}
}

func TestConcurrentAlertDeliveriesSendPersistentIncidentOnlyOnce(t *testing.T) {
	path := createAlertDatabase(t)
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	settings := &staticSettings{value: configstore.NotificationSettings{
		AppID: "app", ClientSecret: "secret", HomeChannel: "target", HomeChannelType: "c2c",
	}}
	sender := &blockingBatchSender{started: make(chan struct{}), release: make(chan struct{})}
	service := New(repository, settings, sender)
	results := make(chan business.AlertDeliveryResult, 2)
	errorsFound := make(chan error, 2)
	deliver := func() {
		result, deliverErr := service.Deliver(context.Background(), false)
		results <- result
		errorsFound <- deliverErr
	}
	go deliver()
	<-sender.started
	if !service.Status().ConsumerActive {
		t.Fatal("consumer was not reported as active during delivery")
	}
	go deliver()
	close(sender.release)

	totalSent := 0
	for range 2 {
		totalSent += (<-results).Sent
		if deliverErr := <-errorsFound; deliverErr != nil {
			t.Fatal(deliverErr)
		}
	}
	if totalSent != 2 || sender.Calls() != 1 {
		t.Fatalf("concurrent delivery duplicated notifications: sent=%d calls=%d", totalSent, sender.Calls())
	}
	if service.Status().ConsumerActive {
		t.Fatal("consumer remained active after delivery")
	}
	snapshot, err := repository.NotificationQueueSnapshot(
		context.Background(),
		business.NotificationChannelKey("qqbot", "target"),
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ProducerFiring != 2 || snapshot.ConsumerPending != 0 || snapshot.ConsumerFailed != 0 {
		t.Fatalf("unexpected queue snapshot after delivery: %#v", snapshot)
	}
	details, err := repository.NotificationQueueDetails(
		context.Background(),
		business.NotificationChannelKey("qqbot", "target"),
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(details.ConsumerItems) != 2 || details.ConsumerItems[0].QueueStatus != "本轮不发送" ||
		details.ConsumerItems[0].QueueReason != "持续告警已通知，策略设置为只发送一次" {
		t.Fatalf("sent incidents do not explain why this round is skipped: %#v", details.ConsumerItems)
	}
}

func TestNotificationQueueDetailsMatchSnapshotAndIncludeRetryState(t *testing.T) {
	path := createAlertDatabase(t)
	database, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	channelKey := business.NotificationChannelKey("qqbot", "target")
	if _, err := database.Exec(`INSERT INTO alert_deliveries(
		incident_key,channel_key,status,attempts,last_error,delivered_at,updated_at
	) VALUES('incident-1',?,'failed',3,'timeout',NULL,'2026-08-28T12:00:00Z')`, channelKey); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE alert_incidents SET delivery_status='发送失败',last_error='timeout'
		WHERE incident_key='incident-1'`); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })

	details, err := repository.NotificationQueueDetails(context.Background(), channelKey, true)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := repository.NotificationQueueSnapshot(context.Background(), channelKey, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(details.ProducerFiring) != 2 || len(details.ConsumerPending) != 2 || len(details.ConsumerFailed) != 1 || len(details.ConsumerItems) != 2 {
		t.Fatalf("unexpected queue details: %#v", details)
	}
	failed := details.ConsumerFailed[0]
	if failed.IncidentKey != "incident-1" || failed.DeliveryAttempts != 3 || failed.LastError == nil || *failed.LastError != "timeout" ||
		failed.QueueStatus != "发送失败，等待重试" || failed.QueueReason != "timeout" {
		t.Fatalf("failed queue item lost delivery context: %#v", failed)
	}
	if snapshot.ProducerFiring != len(details.ProducerFiring) ||
		snapshot.ConsumerPending != len(details.ConsumerPending) ||
		snapshot.ConsumerFailed != len(details.ConsumerFailed) {
		t.Fatalf("snapshot and details diverged: snapshot=%#v details=%#v", snapshot, details)
	}
}

func TestNotificationQueueDetailsExplainSuppressedDeliveries(t *testing.T) {
	path := createAlertDatabase(t)
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	rawPolicy, err := json.Marshal(business.DefaultAlertPolicy())
	if err != nil {
		t.Fatal(err)
	}
	policy := map[string]any{}
	if err := json.Unmarshal(rawPolicy, &policy); err != nil {
		t.Fatal(err)
	}
	policy["delivery_enabled"] = false
	if _, err := repository.UpdateAlertPolicy(context.Background(), policy); err != nil {
		t.Fatal(err)
	}

	details, err := repository.NotificationQueueDetails(
		context.Background(),
		business.NotificationChannelKey("qqbot", "target"),
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(details.ConsumerItems) != 2 || len(details.ConsumerPending) != 0 || len(details.ConsumerFailed) != 0 {
		t.Fatalf("suppressed deliveries were counted as pending: %#v", details)
	}
	for _, item := range details.ConsumerItems {
		if item.QueueStatus != "已抑制" || item.QueueReason != "告警通知发送已关闭" {
			t.Fatalf("suppression reason is missing: %#v", item)
		}
	}
}

func TestNotificationBatchesMergeOnlyWhenAlertCountReachesThreshold(t *testing.T) {
	incidents := make([]business.AlertIncident, 10)
	for index := range incidents {
		incidents[index] = business.AlertIncident{
			IncidentKey: fmt.Sprintf("incident-%d", index), EventType: "upstream.auth",
			ObjectKind: "host", ObjectID: fmt.Sprintf("host-%d.example", index),
			CauseCode: "AUTH", Status: "firing", LastSeenAt: "2026-08-26T08:00:00Z",
		}
	}

	separate := NotificationBatches(incidents[:9], 10)
	merged := NotificationBatches(incidents, 10)

	if len(separate) != 9 {
		t.Fatalf("alerts below threshold were merged: batches=%d", len(separate))
	}
	if len(merged) != 1 || len(merged[0].Incidents) != 10 || !strings.Contains(merged[0].Message, "告警汇总（10项）") {
		t.Fatalf("alerts at threshold were not merged: %#v", merged)
	}
}

func TestNotificationBatchesMergeSmallRoutingDegradationDigest(t *testing.T) {
	incidents := make([]business.AlertIncident, 3)
	for index := range incidents {
		group := fmt.Sprintf("group-%d", index%2)
		incidents[index] = business.AlertIncident{
			IncidentKey: fmt.Sprintf("console:routing:degraded:%d:%s", index, group),
			EventType:   "account.routing_degraded", ObjectKind: "account", ObjectID: fmt.Sprint(index),
			CauseCode: "ROUTING_DEGRADED:健康分下降", Status: "firing",
			LastSeenAt: "2026-08-30T12:00:00Z",
		}
	}

	batches := NotificationBatches(incidents, 10)
	if len(batches) != 1 || len(batches[0].Incidents) != 3 {
		t.Fatalf("small degradation wave was split into separate notifications: %#v", batches)
	}
	if !strings.Contains(batches[0].Message, "3 个账号") ||
		strings.Count(batches[0].Message, "账号进入降级状态") != 1 {
		t.Fatalf("small degradation wave was not rendered as one digest: %s", batches[0].Message)
	}
}

func TestNotificationBatchesMergeRelatedSurvivorAlertAndRecoveryBelowThreshold(t *testing.T) {
	name := "鲨鱼辣椒-0.8"
	for _, status := range []string{"firing", "recovered"} {
		incidents := []business.AlertIncident{
			{
				IncidentKey: "console:routing:survivor:323:A-CCMAX(特价渠道)", EventType: "account.routing_survivor",
				ObjectKind: "account", ObjectID: "323", ObjectName: &name, CauseCode: "ROUTING_SURVIVOR:凭据失效",
				Status: status, LastSeenAt: "2026-08-30T12:59:53Z",
			},
			{
				IncidentKey: "console:routing:group-survivor:A-CCMAX(特价渠道)", EventType: "group.routing_survivor",
				ObjectKind: "group", ObjectID: "A-CCMAX(特价渠道)", CauseCode: "GROUP_SURVIVOR_ONLY",
				Status: status, LastSeenAt: "2026-08-30T12:59:53Z",
			},
		}

		batches := NotificationBatches(incidents, 10)
		if len(batches) != 1 || len(batches[0].Incidents) != 2 {
			t.Fatalf("status=%s related survivor incidents were not merged: %#v", status, batches)
		}
		message := batches[0].Message
		for _, expected := range []string{"分组仅剩保底账号", "A-CCMAX\\(特价渠道\\)", "关联账号", "鲨鱼辣椒-0.8", "\\#323", "凭据失效"} {
			if !strings.Contains(message, expected) {
				t.Fatalf("status=%s merged notification missing %q: %s", status, expected, message)
			}
		}
		if status == "recovered" && !strings.Contains(message, "Sub2API · 恢复通知") {
			t.Fatalf("related recovery lost its recovery title: %s", message)
		}
		if strings.Contains(message, "| 类型 | 对象 | 原因 | 状态 |") {
			t.Fatalf("related pair should use one readable vertical notification: %s", message)
		}
	}
}

func TestNotificationBatchesMergeAllBreakerAccountsIntoUnavailableGroup(t *testing.T) {
	first, second := "账号一", "账号二"
	incidents := []business.AlertIncident{
		{
			IncidentKey: "console:routing:breaker:41:codex", EventType: "account.routing_breaker",
			ObjectKind: "account", ObjectID: "41", ObjectName: &first, CauseCode: "ROUTING_BREAKER:凭据失效",
			Status: "firing", LastSeenAt: "2026-08-30T12:00:00Z",
		},
		{
			IncidentKey: "console:routing:group-unavailable:codex", EventType: "group.routing_unavailable",
			ObjectKind: "group", ObjectID: "codex", CauseCode: "GROUP_UNAVAILABLE",
			Status: "firing", LastSeenAt: "2026-08-30T12:00:00Z",
		},
		{
			IncidentKey: "console:routing:breaker:42:codex", EventType: "account.routing_breaker",
			ObjectKind: "account", ObjectID: "42", ObjectName: &second, CauseCode: "ROUTING_BREAKER:余额不足",
			Status: "firing", LastSeenAt: "2026-08-30T12:00:00Z",
		},
	}

	batches := NotificationBatches(incidents, 10)
	if len(batches) != 1 || len(batches[0].Incidents) != 3 {
		t.Fatalf("group outage and breaker accounts were not merged: %#v", batches)
	}
	for _, expected := range []string{"分组无可调度账号", "账号一", "凭据失效", "账号二", "余额不足"} {
		if !strings.Contains(batches[0].Message, expected) {
			t.Fatalf("merged group outage missing %q: %s", expected, batches[0].Message)
		}
	}
}

func TestNotificationBatchesMergeBindingInvalidAccountIntoUnavailableGroup(t *testing.T) {
	name := "账号一"
	incidents := []business.AlertIncident{
		{
			IncidentKey: "console:routing:binding-invalid:41:codex", EventType: "account.binding_invalid",
			ObjectKind: "account", ObjectID: "41", ObjectName: &name,
			CauseCode: "BINDING_INVALID:上游 Key key-1 已确认删除（连续 2 次完整同步未返回）",
			Status:    "firing", LastSeenAt: "2026-08-30T12:00:00Z",
		},
		{
			IncidentKey: "console:routing:group-unavailable:codex", EventType: "group.routing_unavailable",
			ObjectKind: "group", ObjectID: "codex", CauseCode: "GROUP_UNAVAILABLE",
			Status: "firing", LastSeenAt: "2026-08-30T12:00:00Z",
		},
	}
	batches := NotificationBatches(incidents, 10)
	if len(batches) != 1 || len(batches[0].Incidents) != 2 {
		t.Fatalf("binding invalid account was not merged with group outage: %#v", batches)
	}
	for _, expected := range []string{"分组无可调度账号", "账号一", "Key key-1 已确认删除"} {
		if !strings.Contains(batches[0].Message, expected) {
			t.Fatalf("merged binding notification missing %q: %s", expected, batches[0].Message)
		}
	}
}

func TestNotificationBatchesDoNotMergeDifferentGroupsOrDifferentStatuses(t *testing.T) {
	incidents := []business.AlertIncident{
		{
			IncidentKey: "console:routing:survivor:41:alpha", EventType: "account.routing_survivor",
			ObjectKind: "account", ObjectID: "41", CauseCode: "ROUTING_SURVIVOR", Status: "firing",
			LastSeenAt: "2026-08-30T12:00:00Z",
		},
		{
			IncidentKey: "console:routing:group-survivor:beta", EventType: "group.routing_survivor",
			ObjectKind: "group", ObjectID: "beta", CauseCode: "GROUP_SURVIVOR_ONLY", Status: "firing",
			LastSeenAt: "2026-08-30T12:00:00Z",
		},
		{
			IncidentKey: "console:routing:group-survivor:alpha", EventType: "group.routing_survivor",
			ObjectKind: "group", ObjectID: "alpha", CauseCode: "GROUP_SURVIVOR_ONLY", Status: "recovered",
			LastSeenAt: "2026-08-30T12:00:00Z",
		},
	}

	if batches := NotificationBatches(incidents, 10); len(batches) != 3 {
		t.Fatalf("unrelated incidents were incorrectly merged: %#v", batches)
	}
}

func TestNotificationBatchesCollapseRelatedRowsInsideLargeSummary(t *testing.T) {
	incidents := []business.AlertIncident{
		{
			IncidentKey: "console:routing:survivor:41:codex", EventType: "account.routing_survivor",
			ObjectKind: "account", ObjectID: "41", CauseCode: "ROUTING_SURVIVOR:保底强留：凭据失效", Status: "firing",
			LastSeenAt: "2026-08-30T12:00:00Z",
		},
		{
			IncidentKey: "console:routing:group-survivor:codex", EventType: "group.routing_survivor",
			ObjectKind: "group", ObjectID: "codex", CauseCode: "GROUP_SURVIVOR_ONLY", Status: "firing",
			LastSeenAt: "2026-08-30T12:00:00Z",
		},
	}
	for index := 0; index < 8; index++ {
		incidents = append(incidents, business.AlertIncident{
			IncidentKey: fmt.Sprintf("auth-%d", index), EventType: "upstream.auth", ObjectKind: "host",
			ObjectID: fmt.Sprintf("host-%d.example", index), CauseCode: "AUTH", Status: "firing",
			LastSeenAt: "2026-08-30T12:00:00Z",
		})
	}

	batches := NotificationBatches(incidents, 10)
	if len(batches) != 1 || len(batches[0].Incidents) != 10 {
		t.Fatalf("large related summary lost delivery incidents: %#v", batches)
	}
	message := batches[0].Message
	if !strings.Contains(message, "告警汇总（9项）") || strings.Count(message, "分组仅剩保底账号") != 1 || strings.Contains(message, "账号被保底强留") {
		t.Fatalf("large summary did not collapse its related rows: %s", message)
	}
	if strings.Contains(message, "保底强留：保底强留") {
		t.Fatalf("related account reason retained a duplicated prefix: %s", message)
	}
}

func TestNotificationBatchesCollapseMassRoutingDegradationIntoOneDigest(t *testing.T) {
	incidents := make([]business.AlertIncident, 0, 25)
	for index := 0; index < 24; index++ {
		group := fmt.Sprintf("group-%d", index%4)
		incidents = append(incidents, business.AlertIncident{
			IncidentKey: fmt.Sprintf("console:routing:degraded:%d:%s", index, group),
			EventType:   "account.routing_degraded", ObjectKind: "account", ObjectID: fmt.Sprint(index),
			CauseCode: "ROUTING_DEGRADED:健康分下降", Status: "firing",
			LastSeenAt: "2026-08-30T12:00:00Z",
		})
	}
	incidents = append(incidents, business.AlertIncident{
		IncidentKey: "probe-1", EventType: "account.probe", ObjectKind: "account", ObjectID: "99",
		CauseCode: "PROBE:上游超时", Status: "firing", LastSeenAt: "2026-08-30T12:00:01Z",
	})

	batches := NotificationBatches(incidents, 10)
	if len(batches) != 1 || len(batches[0].Incidents) != 25 {
		t.Fatalf("mass degradation was split into multiple notifications: %#v", batches)
	}
	message := batches[0].Message
	if !strings.Contains(message, "24 个账号") || !strings.Contains(message, "4 个分组") {
		t.Fatalf("mass degradation digest is missing its scope: %s", message)
	}
	if strings.Count(message, "账号进入降级状态") != 1 {
		t.Fatalf("mass degradation was not collapsed into one row: %s", message)
	}
	if !strings.Contains(message, "账号主动探测失败") {
		t.Fatalf("non-degradation alert was lost from the digest: %s", message)
	}
}

func TestNotificationBatchesKeepEvidenceDecisionAndApplyFailureSeparate(t *testing.T) {
	incidents := []business.AlertIncident{
		{
			IncidentKey: "console:probe:41:codex", EventType: "account.probe", ObjectKind: "account",
			ObjectID: "41", CauseCode: "PROBE:上游返回 401", Status: "firing", LastSeenAt: "2026-08-30T12:00:00Z",
		},
		{
			IncidentKey: "console:routing:breaker:41:codex", EventType: "account.routing_breaker", ObjectKind: "account",
			ObjectID: "41", CauseCode: "ROUTING_BREAKER:凭据失效", Status: "firing", LastSeenAt: "2026-08-30T12:00:00Z",
		},
		{
			IncidentKey: "console:routing:apply:41", EventType: "routing.apply_failure", ObjectKind: "account",
			ObjectID: "41", CauseCode: "APPLY_FAILED:network timeout", Status: "firing", LastSeenAt: "2026-08-30T12:00:00Z",
		},
	}

	if batches := NotificationBatches(incidents, 10); len(batches) != 3 {
		t.Fatalf("evidence, decision, and remote apply alerts must keep separate lifecycles: %#v", batches)
	}
}

func TestAlertDeliverySendsRelatedRoutingIncidentsOnceAndFinalizesBoth(t *testing.T) {
	path := createAlertDatabase(t)
	database, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`DELETE FROM alert_incidents;
		INSERT INTO accounts(id,name) VALUES('323','鲨鱼辣椒-0.8');
		INSERT INTO alert_incidents VALUES
		('console:routing:survivor:323:A-CCMAX(特价渠道)','account.routing_survivor','account','323','ROUTING_SURVIVOR:凭据失效','recovered','2026-08-30T12:00:00Z','2026-08-30T12:59:53Z',NULL,NULL),
		('console:routing:group-survivor:A-CCMAX(特价渠道)','group.routing_survivor','group','A-CCMAX(特价渠道)','GROUP_SURVIVOR_ONLY','recovered','2026-08-30T12:00:00Z','2026-08-30T12:59:53Z',NULL,NULL)`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	settings := &staticSettings{value: configstore.NotificationSettings{
		AppID: "app", ClientSecret: "secret", HomeChannel: "target", HomeChannelType: "c2c",
	}}
	sender := &concurrentWriteSender{path: path}
	service := New(repository, settings, sender)

	result, err := service.Deliver(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Sent != 2 || result.Batches != 1 || len(sender.messages) != 1 {
		t.Fatalf("related incidents produced duplicate sends: result=%#v messages=%#v", result, sender.messages)
	}
	verification, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer verification.Close()
	var sent int
	if err := verification.QueryRow(`SELECT COUNT(*) FROM alert_deliveries WHERE status='sent'`).Scan(&sent); err != nil || sent != 2 {
		t.Fatalf("both related incidents were not finalized: sent=%d err=%v", sent, err)
	}
}

func TestNotificationMessageUsesAccountNameAndExactBalanceBoundary(t *testing.T) {
	name := "主账号"
	message := BatchMessage([]business.AlertIncident{{
		IncidentKey: "balance", EventType: "upstream.balance", ObjectKind: "account",
		ObjectID: "41", ObjectName: &name, CauseCode: "BALANCE:5", Status: "firing",
		LastSeenAt: "2026-08-26T08:00:00Z",
	}})
	if !strings.Contains(message, "主账号") || !strings.Contains(message, "\\#41") || !strings.Contains(message, "余额达到或低于 5") {
		t.Fatalf("notification message is ambiguous: %s", message)
	}
}

func TestSingleNotificationUsesVerticalDetailsTable(t *testing.T) {
	message := BatchMessage([]business.AlertIncident{{
		IncidentKey: "auth", EventType: "upstream.auth", ObjectKind: "host", ObjectID: "api.example",
		CauseCode: "AUTH:令牌已过期", Status: "firing", LastSeenAt: "2026-08-26T08:00:00Z",
	}})
	for _, expected := range []string{
		"## Sub2API · 告警通知", "| 项目 | 内容 |", "| 告警类型 | 上游鉴权失效 |",
		"| 告警对象 | 上游：api.example |", "| 原因 | 鉴权已失效：令牌已过期 |",
		"| 状态 | 告警中 |", "| 时间（北京时间） | 2026-08-26 16:00:00 |",
	} {
		if !strings.Contains(message, expected) {
			t.Fatalf("single notification missing %q: %s", expected, message)
		}
	}
	if strings.Contains(message, "| 类型 | 对象 | 原因 | 状态 |") || strings.Contains(message, "（1项）") {
		t.Fatalf("single notification still uses the horizontal summary format: %s", message)
	}
}

func TestMultipleNotificationsKeepHorizontalSummaryTable(t *testing.T) {
	message := BatchMessage([]business.AlertIncident{
		{IncidentKey: "first", EventType: "upstream.auth", ObjectKind: "host", ObjectID: "first.example", CauseCode: "AUTH", Status: "firing", LastSeenAt: "2026-08-26T08:00:00Z"},
		{IncidentKey: "second", EventType: "upstream.balance", ObjectKind: "host", ObjectID: "second.example", CauseCode: "BALANCE:5", Status: "firing", LastSeenAt: "2026-08-26T08:00:01Z"},
	})
	if !strings.Contains(message, "告警汇总（2项）") || !strings.Contains(message, "| 类型 | 对象 | 原因 | 状态 | 时间（北京时间） |") {
		t.Fatalf("multiple notifications lost the horizontal summary format: %s", message)
	}
}

func TestNotificationMessageDistinguishesProbeGroups(t *testing.T) {
	name := "主账号"
	message := BatchMessage([]business.AlertIncident{{
		IncidentKey: "console:probe:41:codex", EventType: "account.probe", ObjectKind: "account",
		ObjectID: "41", ObjectName: &name, CauseCode: "PROBE", Status: "firing",
		LastSeenAt: "2026-08-26T08:00:00Z",
	}})
	if !strings.Contains(message, "分组：codex") {
		t.Fatalf("probe group is missing from notification: %s", message)
	}
}

func TestNotificationMessageShowsRateSyncFailureReason(t *testing.T) {
	message := BatchMessage([]business.AlertIncident{{
		IncidentKey: "rate", EventType: "upstream.rate_sync", ObjectKind: "host", ObjectID: "api.example",
		CauseCode: "RATE_SYNC:上游分组 auto 倍率不是有限数值", Status: "firing", LastSeenAt: "2026-08-26T08:00:00Z",
	}})
	if !strings.Contains(message, "上游分组 auto 倍率不是有限数值") {
		t.Fatalf("rate-sync reason is missing: %s", message)
	}
}

func TestNotificationMessageExplainsRoutingDecisionAndApplyFailure(t *testing.T) {
	name := "主账号"
	message := BatchMessage([]business.AlertIncident{
		{
			IncidentKey: "console:routing:breaker:41:codex", EventType: "account.routing_breaker",
			ObjectKind: "account", ObjectID: "41", ObjectName: &name,
			CauseCode: "ROUTING_BREAKER:连续网关错误", Status: "firing", LastSeenAt: "2026-08-26T08:00:00Z",
		},
		{
			IncidentKey: "console:routing:apply:41", EventType: "routing.apply_failure",
			ObjectKind: "account", ObjectID: "41", ObjectName: &name,
			CauseCode: "APPLY_FAILED:network timeout", Status: "firing", LastSeenAt: "2026-08-26T08:00:01Z",
		},
	})
	for _, expected := range []string{"账号触发熔断判定", "分组：codex", "连续网关错误", "自动执行失败", "network timeout"} {
		if !strings.Contains(message, expected) {
			t.Fatalf("routing notification missing %q: %s", expected, message)
		}
	}
}

func TestRecoverySuppressedByPolicyIsNotReplayedAfterReenable(t *testing.T) {
	path := createAlertDatabase(t)
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	ctx := context.Background()
	disabledRecovery := defaultAlertPolicyPayload(t)
	disabledRecovery["notify_recovery"] = false
	if _, err := repository.UpdateAlertPolicy(ctx, disabledRecovery); err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.ExecContext(ctx, `UPDATE alert_incidents SET status='recovered' WHERE incident_key='incident-1'`); err != nil {
		t.Fatal(err)
	}
	settings := &staticSettings{value: configstore.NotificationSettings{
		AppID: "app", ClientSecret: "secret", HomeChannel: "target", HomeChannelType: "c2c",
	}}
	sender := &concurrentWriteSender{path: path}
	service := New(repository, settings, sender)

	first, err := service.Deliver(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if first.Suppressed != 1 {
		t.Fatalf("recovery was not suppressed: %#v", first)
	}
	enabledRecovery := defaultAlertPolicyPayload(t)
	enabledRecovery["notify_recovery"] = true
	if _, err := repository.UpdateAlertPolicy(ctx, enabledRecovery); err != nil {
		t.Fatal(err)
	}
	second, err := service.Deliver(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if second.Attempted != 0 || second.Suppressed != 1 || len(sender.messages) != 1 {
		t.Fatalf("stale recovery was replayed after re-enable: result=%#v messages=%#v", second, sender.messages)
	}
}

func defaultAlertPolicyPayload(t *testing.T) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(business.DefaultAlertPolicy())
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

type staticSettings struct {
	value configstore.NotificationSettings
}

func (s *staticSettings) NotificationSettings(context.Context) (configstore.NotificationSettings, error) {
	return s.value, nil
}

type concurrentWriteSender struct {
	path     string
	messages []string
}

type blockingBatchSender struct {
	mu      sync.Mutex
	calls   int
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *blockingBatchSender) Send(_ context.Context, _ configstore.NotificationSettings, messages []string) []SendOutcome {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	s.once.Do(func() { close(s.started) })
	<-s.release
	outcomes := make([]SendOutcome, len(messages))
	for index := range outcomes {
		outcomes[index] = SendOutcome{Success: true, Detail: "sent"}
	}
	return outcomes
}

func (s *blockingBatchSender) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func TestNotificationTestPersistsConfirmedOutcomeAfterNetworkSend(t *testing.T) {
	path := createAlertDatabase(t)
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repository.Close() })
	settings := &staticSettings{value: configstore.NotificationSettings{
		AppID: "app", ClientSecret: "secret", HomeChannel: "target", HomeChannelType: "c2c",
	}}
	sender := &concurrentWriteSender{path: path}
	service := New(repository, settings, sender)

	result, err := service.Test(context.Background(), "Sub2API Console 通知测试", false)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Sent || result.MessageID == nil || *result.MessageID != "message-1" || !result.Persisted || result.RuntimeEventID >= 0 {
		t.Fatalf("unexpected test result: %#v", result)
	}
	database, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var status, payload string
	if err := database.QueryRow(`SELECT status,payload_json FROM runtime_events WHERE source_id=?`, result.RuntimeEventID).Scan(&status, &payload); err != nil {
		t.Fatal(err)
	}
	if status != "succeeded" || !strings.Contains(payload, `"message_id":"message-1"`) {
		t.Fatalf("unexpected persisted event: status=%s payload=%s", status, payload)
	}
}

func (s *concurrentWriteSender) Send(_ context.Context, _ configstore.NotificationSettings, messages []string) []SendOutcome {
	s.messages = append(s.messages, messages...)
	database, err := sql.Open("sqlite", "file:"+s.path+"?_pragma=busy_timeout%28100%29")
	if err != nil {
		return []SendOutcome{{Detail: err.Error()}}
	}
	defer database.Close()
	if _, err := database.Exec(`INSERT INTO app_state(key,value_json,updated_at) VALUES('network-callback','{}','now')
		ON CONFLICT(key) DO UPDATE SET updated_at=excluded.updated_at`); err != nil {
		return []SendOutcome{{Detail: err.Error()}}
	}
	outcomes := make([]SendOutcome, len(messages))
	for index := range messages {
		id := fmt.Sprintf("message-%d", index+1)
		outcomes[index] = SendOutcome{Success: true, Detail: "sent", MessageID: &id}
	}
	return outcomes
}

func createAlertDatabase(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "alerts.sqlite3")
	database, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`CREATE TABLE app_state(key TEXT PRIMARY KEY,value_json TEXT NOT NULL,updated_at TEXT NOT NULL)`,
		`INSERT INTO app_state(key,value_json,updated_at) VALUES('config','{"keys":[],"mode":"完全模式"}','now')`,
		`CREATE TABLE policy_nodes(id INTEGER PRIMARY KEY AUTOINCREMENT,policy_key TEXT NOT NULL,parent_id INTEGER,key_name TEXT,list_index INTEGER,node_type TEXT NOT NULL,scalar_value TEXT,updated_at TEXT NOT NULL)`,
		`CREATE TABLE operational_snapshots(namespace TEXT NOT NULL,state_key TEXT NOT NULL,value_json TEXT NOT NULL,updated_at TEXT NOT NULL,PRIMARY KEY(namespace,state_key))`,
		`CREATE TABLE accounts(id TEXT PRIMARY KEY,name TEXT NOT NULL)`,
		`INSERT INTO operational_snapshots VALUES('sub2api','sub2api-notify-rules.json','{"enabled":true,"channels":[{"type":"qqbot","enabled":true}]}','now')`,
		`CREATE TABLE alert_incidents(incident_key TEXT PRIMARY KEY,event_type TEXT NOT NULL,object_kind TEXT NOT NULL,object_id TEXT NOT NULL,cause_code TEXT NOT NULL,status TEXT NOT NULL,first_seen_at TEXT NOT NULL,last_seen_at TEXT NOT NULL,delivery_status TEXT,last_error TEXT)`,
		`INSERT INTO alert_incidents VALUES('incident-1','upstream.auth','host','first.example','AUTH','firing','2026-08-26T08:00:00Z','2026-08-26T08:00:00Z',NULL,NULL)`,
		`INSERT INTO alert_incidents VALUES('incident-2','upstream.balance','host','second.example','BALANCE:5','firing','2026-08-26T08:00:01Z','2026-08-26T08:00:01Z',NULL,NULL)`,
		`CREATE TABLE alert_deliveries(incident_key TEXT NOT NULL,channel_key TEXT NOT NULL,status TEXT NOT NULL,attempts INTEGER NOT NULL DEFAULT 0,last_error TEXT,delivered_at TEXT,updated_at TEXT NOT NULL,PRIMARY KEY(incident_key,channel_key))`,
	}
	for _, statement := range statements {
		if _, err := database.Exec(statement); err != nil {
			database.Close()
			t.Fatalf("fixture statement failed: %v\n%s", err, statement)
		}
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}
