package logs

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type fakeBusiness struct {
	runs   []business.RunRecord
	events []business.RunEvent
	audits []business.AuditEvent
}

func (f fakeBusiness) RunRecords(context.Context, *int) ([]business.RunRecord, error) {
	return f.runs, nil
}
func (f fakeBusiness) Events(context.Context, *int) ([]business.RunEvent, error) {
	return f.events, nil
}
func (f fakeBusiness) AuditEvents(context.Context, *int, bool) ([]business.AuditEvent, error) {
	return f.audits, nil
}

type fakeTasks []taskstore.Task

func (f fakeTasks) ListLogSummaries(context.Context, *int) ([]taskstore.Task, error) { return f, nil }
func (f fakeTasks) SearchLogs(context.Context, string, *int) ([]taskstore.Task, error) {
	return f, nil
}

func TestUnifiedLogsLinksRunAndEventToTask(t *testing.T) {
	now := "2026-08-26T10:00:00Z"
	service := New(fakeBusiness{
		runs:   []business.RunRecord{{RunKey: "run-1", TaskName: "同步", Status: textPointer("succeeded"), UpdatedAt: now, Payload: map[string]any{}}},
		events: []business.RunEvent{{ID: 1, EventType: "sync.finished", CreatedAt: now, Status: "succeeded", Summary: "完成", Payload: map[string]any{"run_key": "run-1"}}},
		audits: []business.AuditEvent{{ID: 1, OperationID: "run-1", OperationType: "sync", State: "succeeded", Phase: "writeback", Writeback: true, CreatedAt: now, GroupNames: []string{}}},
	}, fakeTasks{{ID: "task-1", Skill: "console", Operation: "sync", Status: "succeeded", Progress: 100, Message: "完成", Result: map[string]any{"run_key": "run-1"}, CreatedAt: now, UpdatedAt: now}})
	page, err := service.Query(context.Background(), Query{Kind: "all", State: "all", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].RelatedCount != 3 {
		t.Fatalf("unexpected linked logs: %#v", page)
	}
	if page.Counts["task"] != 1 || page.Counts["event"] != 1 || page.Counts["change"] != 1 {
		t.Fatalf("unexpected counts: %#v", page.Counts)
	}
}

func TestUnifiedLogsLinksTaskIDEventToTask(t *testing.T) {
	now := "2026-08-26T10:00:00Z"
	service := New(fakeBusiness{
		events: []business.RunEvent{{ID: 1, EventType: "routing.applied", CreatedAt: now, Status: "succeeded", Summary: "账号已写回", Payload: map[string]any{"task_id": "task-1", "account_id": "41"}}},
	}, fakeTasks{{ID: "task-1", Skill: "console", Operation: "inspection", Status: "succeeded", Progress: 100, Message: "完成", Result: map[string]any{}, CreatedAt: now, UpdatedAt: now}})

	page, err := service.Query(context.Background(), Query{Kind: "all", State: "all", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || page.Items[0].RelatedCount != 1 {
		t.Fatalf("task_id event was not linked to task: %#v", page)
	}
}

func TestUnifiedLogsGroupsAtomicWritebacksUnderInspectionTask(t *testing.T) {
	now := "2026-09-06T10:00:00Z"
	taskID := "inspection-task-42"
	actor := "自动巡检"
	remoteConfirmed, readbackConfirmed := true, true
	service := New(fakeBusiness{audits: []business.AuditEvent{
		{ID: 1, OperationID: "routing-writeback-one", OperationType: "routing.writeback", State: "succeeded", Phase: "readback",
			TaskID: &taskID, Actor: &actor, RemoteConfirmed: &remoteConfirmed, ReadbackConfirmed: &readbackConfirmed,
			ObjectID: textPointer("41"), FieldName: textPointer("priority"), Writeback: true, CreatedAt: now},
		{ID: 2, OperationID: "routing-writeback-two", OperationType: "routing.writeback", State: "succeeded", Phase: "readback",
			TaskID: &taskID, Actor: &actor, RemoteConfirmed: &remoteConfirmed, ReadbackConfirmed: &readbackConfirmed,
			ObjectID: textPointer("42"), FieldName: textPointer("priority"), Writeback: true, CreatedAt: now},
	}}, fakeTasks{{
		ID: taskID, Skill: "sub2api-auto-inspection", Operation: "automatic-inspection", Status: "succeeded",
		Progress: 100, Message: "巡检完成", Result: map[string]any{}, CreatedAt: now, UpdatedAt: now,
	}})

	changes, err := service.Query(context.Background(), Query{Kind: "change", State: "all", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if changes.Total != 1 || changes.Items[0].RelatedCount != 2 || changes.Items[0].SourceID != taskID {
		t.Fatalf("atomic writebacks were not grouped by task: %#v", changes)
	}
	if changes.Items[0].Details["task_id"] != taskID || changes.Items[0].Details["operation_id"] != nil {
		t.Fatalf("group identity is ambiguous: %#v", changes.Items[0].Details)
	}
	rows, ok := changes.Items[0].Details["changes"].([]business.AuditEvent)
	if !ok || len(rows) != 2 || rows[0].OperationID == rows[1].OperationID {
		t.Fatalf("atomic operation IDs were not preserved: %#v", changes.Items[0].Details)
	}

	all, err := service.Query(context.Background(), Query{Kind: "all", State: "all", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if all.Total != 1 || all.Items[0].ID != "task:"+taskID || all.Items[0].RelatedCount != 1 {
		t.Fatalf("writeback group was not linked to parent task: %#v", all)
	}
}

func TestUnifiedLogsKeepsWritebacksFromDifferentTasksSeparate(t *testing.T) {
	firstTask, secondTask := "inspection-task-one", "inspection-task-two"
	service := New(fakeBusiness{audits: []business.AuditEvent{
		{ID: 1, OperationID: "routing-writeback-one", OperationType: "routing.writeback", State: "succeeded", Phase: "readback", TaskID: &firstTask, ObjectID: textPointer("41"), Writeback: true, CreatedAt: "2026-09-06T10:00:00Z"},
		{ID: 2, OperationID: "routing-writeback-two", OperationType: "routing.writeback", State: "succeeded", Phase: "readback", TaskID: &secondTask, ObjectID: textPointer("42"), Writeback: true, CreatedAt: "2026-09-06T10:00:00Z"},
	}}, fakeTasks{})

	page, err := service.Query(context.Background(), Query{Kind: "change", State: "all", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 {
		t.Fatalf("different inspection tasks were merged: %#v", page)
	}
}

func TestUnifiedLogsValidatesFiltersAndPaginates(t *testing.T) {
	service := New(fakeBusiness{}, fakeTasks{})
	if _, err := service.Query(context.Background(), Query{Kind: "json", State: "all", Page: 1, PageSize: 20}); err == nil {
		t.Fatal("invalid kind must fail")
	}
	page, err := service.Query(context.Background(), Query{Kind: "all", State: "failed", Search: "anything", Page: 2, PageSize: 20})
	if err != nil || page.Total != 0 || page.Items == nil {
		t.Fatalf("unexpected empty page: %#v err=%v", page, err)
	}
	if _, err := service.Query(context.Background(), Query{Kind: "event", State: "all", Level: "debug", Page: 1, PageSize: 20}); err == nil {
		t.Fatal("invalid event level must fail")
	}
}

type countingBusiness struct {
	loads atomic.Int64
}

func (f *countingBusiness) RunRecords(context.Context, *int) ([]business.RunRecord, error) {
	f.loads.Add(1)
	return []business.RunRecord{}, nil
}

func (f *countingBusiness) Events(context.Context, *int) ([]business.RunEvent, error) {
	f.loads.Add(1)
	return []business.RunEvent{}, nil
}

func (f *countingBusiness) AuditEvents(context.Context, *int, bool) ([]business.AuditEvent, error) {
	f.loads.Add(1)
	return []business.AuditEvent{}, nil
}

type countingTasks struct {
	loads atomic.Int64
}

func (f *countingTasks) ListLogSummaries(context.Context, *int) ([]taskstore.Task, error) {
	f.loads.Add(1)
	return []taskstore.Task{}, nil
}

func (f *countingTasks) SearchLogs(context.Context, string, *int) ([]taskstore.Task, error) {
	f.loads.Add(1)
	return []taskstore.Task{}, nil
}

func TestUnifiedLogsReuseFreshPage(t *testing.T) {
	businessReader := &countingBusiness{}
	taskReader := &countingTasks{}
	service := New(businessReader, taskReader)
	query := Query{Kind: "all", State: "all", Page: 1, PageSize: 20}
	for index := 0; index < 2; index++ {
		if _, err := service.Query(context.Background(), query); err != nil {
			t.Fatal(err)
		}
	}
	if businessReader.loads.Load() != 3 || taskReader.loads.Load() != 1 {
		t.Fatalf("fresh snapshot was reloaded: business=%d tasks=%d", businessReader.loads.Load(), taskReader.loads.Load())
	}
}

func TestUnifiedLogsFiltersEventLevelAndGroup(t *testing.T) {
	service := New(fakeBusiness{events: []business.RunEvent{
		{ID: 1, EventType: "routing.degraded", CreatedAt: "2026-08-28T10:00:00Z", Status: "warning", Summary: "账号已降级", Payload: map[string]any{"group_name": "codex"}},
		{ID: 2, EventType: "routing.fused", CreatedAt: "2026-08-28T10:01:00Z", Status: "failed", Summary: "账号已停止调度", Payload: map[string]any{"groups": []any{"grok"}}},
		{ID: 3, EventType: "routing.recovered", CreatedAt: "2026-08-28T10:02:00Z", Status: "succeeded", Summary: "账号已恢复", Payload: map[string]any{"group_id": "7", "group_names": []string{"codex", "grok"}}},
	}}, fakeTasks{})

	page, err := service.Query(context.Background(), Query{
		Kind: "event", State: "all", Level: "warning", Group: "codex", Page: 1, PageSize: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || page.Items[0].SourceID != "1" {
		t.Fatalf("unexpected event filter result: %#v", page)
	}
	page, err = service.Query(context.Background(), Query{
		Kind: "event", State: "all", Level: "info", GroupID: "7", Page: 1, PageSize: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || page.Items[0].SourceID != "3" {
		t.Fatalf("group ID did not match event: %#v", page)
	}
}

func TestUnifiedLogsAggregatesObjectEventsFromSameOperation(t *testing.T) {
	service := New(fakeBusiness{events: []business.RunEvent{
		{ID: 1, EventType: "routing.applied", CreatedAt: "2026-09-05T15:16:55.100Z", Status: "succeeded", Summary: "账号 901 自动写回已生效", Payload: map[string]any{"batch_id": "routing-round-1", "account_id": "901", "actor": "自动巡检"}},
		{ID: 2, EventType: "routing.applied", CreatedAt: "2026-09-05T15:16:55.900Z", Status: "succeeded", Summary: "账号 906 自动写回已生效", Payload: map[string]any{"batch_id": "routing-round-1", "account_id": "906", "actor": "自动巡检"}},
		{ID: 3, EventType: "routing.apply_failed", CreatedAt: "2026-09-05T15:16:56.100Z", Status: "failed", Summary: "账号 917 自动写回失败", Payload: map[string]any{"batch_id": "routing-round-1", "account_id": "917", "actor": "自动巡检"}},
	}}, fakeTasks{})

	page, err := service.Query(context.Background(), Query{Kind: "event", State: "all", Level: "all", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("同一操作批次没有聚合：%#v", page)
	}
	entry := page.Items[0]
	if entry.Title != "routing.writeback.batch" || entry.Status != "partial" || entry.Summary != "共 3 个账号：成功 2，失败 1" {
		t.Fatalf("聚合摘要不正确：%#v", entry)
	}
	if entry.ObjectLabel == nil || *entry.ObjectLabel != "3 个账号" || entry.Actor == nil || *entry.Actor != "自动巡检" {
		t.Fatalf("聚合对象或执行人不正确：%#v", entry)
	}
	children, ok := entry.Details["events"].([]Entry)
	if !ok || len(children) != 3 || entry.RelatedCount != 3 {
		t.Fatalf("聚合后没有保留原始事件：%#v", entry.Details)
	}
}

func TestUnifiedLogsKeepsDifferentEventFamiliesFromSameTaskSeparate(t *testing.T) {
	service := New(fakeBusiness{events: []business.RunEvent{
		{ID: 1, EventType: "routing.applied", CreatedAt: "2026-09-05T15:16:55Z", Status: "succeeded", Summary: "账号 901 自动写回已生效", Payload: map[string]any{"task_id": "inspection-1", "account_id": "901"}},
		{ID: 2, EventType: "routing.degraded", CreatedAt: "2026-09-05T15:16:56Z", Status: "warning", Summary: "账号 906 已降级", Payload: map[string]any{"task_id": "inspection-1", "account_id": "906"}},
	}}, fakeTasks{})

	page, err := service.Query(context.Background(), Query{Kind: "event", State: "all", Level: "all", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 {
		t.Fatalf("同一任务的不同事件族被错误聚合：%#v", page)
	}
}

func TestUnifiedLogsDoesNotAggregateLegacyObjectEventsByTime(t *testing.T) {
	service := New(fakeBusiness{events: []business.RunEvent{
		{ID: 1, EventType: "upstream.sync", CreatedAt: "2026-09-05T15:15:29.100Z", Status: "succeeded", Summary: "上游同步完成：alpha.example", Payload: map[string]any{"host": "alpha.example", "actor": "自动巡检"}},
		{ID: 2, EventType: "upstream.sync", CreatedAt: "2026-09-05T15:15:29.900Z", Status: "succeeded", Summary: "上游同步完成：beta.example", Payload: map[string]any{"host": "beta.example", "actor": "自动巡检"}},
		{ID: 3, EventType: "upstream.sync", CreatedAt: "2026-09-05T15:15:31Z", Status: "succeeded", Summary: "上游同步完成：gamma.example", Payload: map[string]any{"host": "gamma.example", "actor": "自动巡检"}},
	}}, fakeTasks{})

	page, err := service.Query(context.Background(), Query{Kind: "event", State: "all", Level: "all", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 || len(page.Items) != 3 {
		t.Fatalf("缺少任务标识的事件不应按时间聚合：%#v", page)
	}
}

func TestUnifiedLogsKeepsDifferentOperationBatchesSeparate(t *testing.T) {
	service := New(fakeBusiness{events: []business.RunEvent{
		{ID: 1, EventType: "routing.applied", CreatedAt: "2026-09-05T15:16:55Z", Status: "succeeded", Summary: "账号 901 自动写回已生效", Payload: map[string]any{"batch_id": "routing-round-1", "account_id": "901", "actor": "自动巡检"}},
		{ID: 2, EventType: "routing.applied", CreatedAt: "2026-09-05T15:16:55Z", Status: "succeeded", Summary: "账号 906 自动写回已生效", Payload: map[string]any{"batch_id": "routing-round-2", "account_id": "906", "actor": "自动巡检"}},
	}}, fakeTasks{})

	page, err := service.Query(context.Background(), Query{Kind: "event", State: "all", Level: "all", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 {
		t.Fatalf("不同操作批次被误合并：%#v", page)
	}
}

func TestUnifiedLogsDoesNotGuessLegacyGroupsForArbitraryEvents(t *testing.T) {
	service := New(fakeBusiness{events: []business.RunEvent{
		{ID: 1, EventType: "account.updated", CreatedAt: "2026-09-05T15:16:55.100Z", Status: "succeeded", Summary: "账号 901 已更新", Payload: map[string]any{"account_id": "901", "actor": "控制台"}},
		{ID: 2, EventType: "account.updated", CreatedAt: "2026-09-05T15:16:55.900Z", Status: "succeeded", Summary: "账号 906 已更新", Payload: map[string]any{"account_id": "906", "actor": "控制台"}},
	}}, fakeTasks{})

	page, err := service.Query(context.Background(), Query{Kind: "event", State: "all", Level: "all", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 {
		t.Fatalf("缺少稳定批次 ID 的普通事件被误合并：%#v", page)
	}
}

func TestUnifiedLogsDoesNotAggregateEventsByDatabaseTimestamp(t *testing.T) {
	createdAt := "2026-09-05T15:16:55.123456789Z"
	service := New(fakeBusiness{events: []business.RunEvent{
		{ID: 1, EventType: "account.updated", CreatedAt: createdAt, Status: "succeeded", Summary: "账号 901 已更新", Payload: map[string]any{"account_id": "901", "actor": "控制台"}},
		{ID: 2, EventType: "account.updated", CreatedAt: createdAt, Status: "succeeded", Summary: "账号 906 已更新", Payload: map[string]any{"account_id": "906", "actor": "控制台"}},
	}}, fakeTasks{})

	page, err := service.Query(context.Background(), Query{Kind: "event", State: "all", Level: "all", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("事件不应根据数据库时间戳猜测批次：%#v", page)
	}
}

func TestUnifiedLogsAggregatesOperationalEventFamilies(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
	}{
		{name: "routing state changes", eventType: "routing.fused"},
		{name: "active probe failures", eventType: "probe.failed"},
		{name: "automatic cleanup decisions", eventType: "cleanup_queued"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := New(fakeBusiness{events: []business.RunEvent{
				{ID: 1, EventType: test.eventType, CreatedAt: "2026-09-05T15:16:55.100Z", Status: "warning", Summary: "账号 901 状态已更新", Payload: map[string]any{"account_id": "901", "actor": "自动巡检"}},
				{ID: 2, EventType: test.eventType, CreatedAt: "2026-09-05T15:16:55.900Z", Status: "warning", Summary: "账号 906 状态已更新", Payload: map[string]any{"account_id": "906", "actor": "自动巡检"}},
			}}, fakeTasks{})

			page, err := service.Query(context.Background(), Query{Kind: "event", State: "all", Level: "all", Page: 1, PageSize: 20})
			if err != nil {
				t.Fatal(err)
			}
			if page.Total != 2 || len(page.Items) != 2 {
				t.Fatalf("%s events without task identity must remain separate: %#v", test.eventType, page)
			}
		})
	}
}

func TestUnifiedLogsFiltersEventDetailsBeforeAggregation(t *testing.T) {
	service := New(fakeBusiness{events: []business.RunEvent{
		{ID: 1, EventType: "routing.applied", CreatedAt: "2026-09-05T15:16:55Z", Status: "succeeded", Summary: "账号 901 自动写回已生效", Payload: map[string]any{"batch_id": "routing-round-1", "account_id": "901"}},
		{ID: 2, EventType: "routing.apply_failed", CreatedAt: "2026-09-05T15:16:56Z", Status: "failed", Summary: "账号 917 自动写回失败", Payload: map[string]any{"batch_id": "routing-round-1", "account_id": "917"}},
	}}, fakeTasks{})

	page, err := service.Query(context.Background(), Query{Kind: "event", State: "all", Level: "error", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || page.Items[0].SourceID != "2" {
		t.Fatalf("事件筛选没有先作用于原始明细：%#v", page)
	}
}

func TestUnifiedLogsGroupsConfirmedRemoteReadsAndExplainsWriteReadbackFailure(t *testing.T) {
	actor := "自动巡检"
	taskID := "inspection-task-42"
	remoteFalse, readbackTrue, remoteTrue, readbackFalse := false, true, true, false
	failure := "账号自动执行后读回不一致：schedulable"
	service := New(fakeBusiness{audits: []business.AuditEvent{
		{ID: 1, OperationID: "read-1", OperationType: "routing.writeback", State: "succeeded", Phase: "readback", Actor: &actor,
			TaskID:          &taskID,
			RemoteConfirmed: &remoteFalse, ReadbackConfirmed: &readbackTrue, ObjectID: textPointer("41"), Writeback: false, CreatedAt: "2026-08-28T10:00:00.100Z"},
		{ID: 2, OperationID: "read-2", OperationType: "routing.writeback", State: "succeeded", Phase: "readback", Actor: &actor,
			TaskID:          &taskID,
			RemoteConfirmed: &remoteFalse, ReadbackConfirmed: &readbackTrue, ObjectID: textPointer("42"), Writeback: false, CreatedAt: "2026-08-28T10:00:00.900Z"},
		{ID: 3, OperationID: "write-1", OperationType: "routing.writeback", State: "failed", Phase: "remote-readback", Actor: &actor,
			RemoteConfirmed: &remoteTrue, ReadbackConfirmed: &readbackFalse, ObjectID: textPointer("43"), Error: &failure, Writeback: true, CreatedAt: "2026-08-28T10:00:01Z"},
	}}, fakeTasks{})

	page, err := service.Query(context.Background(), Query{Kind: "change", State: "all", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || page.Counts["change"] != 2 {
		t.Fatalf("同一轮远程读取没有聚合：%#v", page)
	}
	if page.Items[0].Title != "写后复核" || page.Items[0].Summary != "远程写入已提交，写后读取不一致："+failure {
		t.Fatalf("写后复核失败语义不清晰：%#v", page.Items[0])
	}
	if page.Items[1].Title != "远程读取复核" || page.Items[1].Summary != "读取并复核 2 个账号，远程状态已符合调度目标" {
		t.Fatalf("远程读取聚合语义不清晰：%#v", page.Items[1])
	}
}

func TestUnifiedLogsDoesNotGroupLegacyAuditsByTimestamp(t *testing.T) {
	actor := "自动巡检"
	remoteConfirmed, readbackConfirmed := false, true
	service := New(fakeBusiness{audits: []business.AuditEvent{
		{ID: 1, OperationID: "read-1", OperationType: "routing.writeback", State: "succeeded", Phase: "readback", Actor: &actor,
			RemoteConfirmed: &remoteConfirmed, ReadbackConfirmed: &readbackConfirmed, ObjectID: textPointer("41"), Writeback: false, CreatedAt: "2026-08-28T10:00:00.100Z"},
		{ID: 2, OperationID: "read-2", OperationType: "routing.writeback", State: "succeeded", Phase: "readback", Actor: &actor,
			RemoteConfirmed: &remoteConfirmed, ReadbackConfirmed: &readbackConfirmed, ObjectID: textPointer("42"), Writeback: false, CreatedAt: "2026-08-28T10:00:00.900Z"},
	}}, fakeTasks{})

	page, err := service.Query(context.Background(), Query{Kind: "change", State: "all", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 {
		t.Fatalf("legacy audits without task identity were grouped by timestamp: %#v", page)
	}
}

func TestAuditActionSummaryDistinguishesAccountDeleteFailureStages(t *testing.T) {
	failure := "delete failed"
	falseValue := false
	tests := []struct {
		name      string
		phase     string
		writeback bool
		after     map[string]any
		title     string
		summary   string
	}{
		{
			name: "no remote write after already absent key", phase: "management-target-check", writeback: false,
			after: map[string]any{"upstream_key_deleted": true, "upstream_key_delete_requested": false},
			title: "远程删除未发出", summary: "上游 Key 已确认不存在，未发出后续远程删除：" + failure,
		},
		{
			name: "confirmed upstream key write", phase: "management-target-check", writeback: true,
			after: map[string]any{"upstream_key_deleted": true, "upstream_key_delete_requested": true},
			title: "部分删除已确认", summary: "上游 Key 删除已确认，后续删除未完成：" + failure,
		},
		{
			name: "management account remains readable", phase: "management-readback-still-readable", writeback: true,
			title: "管理账号仍存在", summary: "管理 DELETE 已发出，读回确认账号仍存在：" + failure,
		},
		{
			name: "management account readback failed", phase: "management-readback", writeback: true,
			title: "管理账号删除结果未知", summary: "管理 DELETE 已发出，但删除后读回失败：" + failure,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			title, summary := auditActionSummary([]business.AuditEvent{{
				OperationType: "account.delete", Phase: test.phase, State: "failed", Error: &failure,
				RemoteConfirmed: &falseValue, ReadbackConfirmed: &falseValue, Writeback: test.writeback, After: test.after,
			}}, 1, 1, &failure)
			if title != test.title || summary != test.summary {
				t.Fatalf("title=%q summary=%q", title, summary)
			}
		})
	}
}

func textPointer(value string) *string { return &value }
