package logs

import (
	"context"
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

func (f fakeTasks) List(context.Context, *int) ([]taskstore.Task, error) { return f, nil }

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

func TestUnifiedLogsGroupsConfirmedRemoteReadsAndExplainsWriteReadbackFailure(t *testing.T) {
	actor := "自动巡检"
	remoteFalse, readbackTrue, remoteTrue, readbackFalse := false, true, true, false
	failure := "账号自动执行后读回不一致：schedulable"
	service := New(fakeBusiness{audits: []business.AuditEvent{
		{ID: 1, OperationID: "read-1", OperationType: "routing.writeback", State: "succeeded", Phase: "readback", Actor: &actor,
			RemoteConfirmed: &remoteFalse, ReadbackConfirmed: &readbackTrue, ObjectID: textPointer("41"), Writeback: false, CreatedAt: "2026-08-28T10:00:00.100Z"},
		{ID: 2, OperationID: "read-2", OperationType: "routing.writeback", State: "succeeded", Phase: "readback", Actor: &actor,
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

func textPointer(value string) *string { return &value }
