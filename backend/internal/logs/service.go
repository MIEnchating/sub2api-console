package logs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

const (
	sourceLimit     = 5000
	cacheFreshness  = 10 * time.Second
	refreshTimeout  = 30 * time.Second
	maximumCacheAge = 5 * time.Minute
)

type BusinessReader interface {
	RunRecords(context.Context, *int) ([]business.RunRecord, error)
	Events(context.Context, *int) ([]business.RunEvent, error)
	AuditEvents(context.Context, *int, bool) ([]business.AuditEvent, error)
}

type TaskReader interface {
	ListLogSummaries(context.Context, *int) ([]taskstore.Task, error)
	SearchLogs(context.Context, string, *int) ([]taskstore.Task, error)
}

type businessLogSearcher interface {
	SearchRunRecords(context.Context, string, *int) ([]business.RunRecord, error)
	SearchEvents(context.Context, string, *int) ([]business.RunEvent, error)
	SearchAuditEvents(context.Context, string, *int) ([]business.AuditEvent, error)
}

type Entry struct {
	ID           string         `json:"id"`
	Kind         string         `json:"kind"`
	OccurredAt   string         `json:"occurred_at"`
	Title        string         `json:"title"`
	Summary      string         `json:"summary"`
	Status       string         `json:"status"`
	Actor        *string        `json:"actor"`
	ObjectLabel  *string        `json:"object_label"`
	Source       string         `json:"source"`
	SourceID     string         `json:"source_id"`
	RelatedCount int            `json:"related_count"`
	Details      map[string]any `json:"details"`
}

type Page struct {
	Items     []Entry        `json:"items"`
	Total     int            `json:"total"`
	Page      int            `json:"page"`
	PageSize  int            `json:"page_size"`
	Counts    map[string]int `json:"counts"`
	Truncated bool           `json:"truncated"`
}

type Query struct {
	Kind, State, Level, Group, GroupID, Search string
	Page, PageSize                             int
}

type Service struct {
	business BusinessReader
	tasks    TaskReader
	runner   taskrunner.Runner

	mu      sync.Mutex
	cache   map[Query]cachedPage
	loading map[Query]chan struct{}
}

type cachedPage struct {
	page     Page
	loadedAt time.Time
}

type logSnapshot struct {
	taskEntries   []Entry
	eventEntries  []Entry
	changeEntries []Entry
	linkedEvents  map[string]struct{}
	linkedChanges map[string]struct{}
	truncated     bool
}

func New(businessReader BusinessReader, taskReader TaskReader) *Service {
	return &Service{
		business: businessReader,
		tasks:    taskReader,
		cache:    map[Query]cachedPage{},
		loading:  map[Query]chan struct{}{},
	}
}

func (s *Service) UseTaskRunner(runner taskrunner.Runner) { s.runner = runner }

func (s *Service) Query(ctx context.Context, query Query) (Page, error) {
	if _, ok := map[string]struct{}{"all": {}, "task": {}, "event": {}, "change": {}}[query.Kind]; !ok {
		return Page{}, errors.New("kind 选项无效")
	}
	if _, ok := map[string]struct{}{"all": {}, "active": {}, "failed": {}, "warning": {}, "succeeded": {}}[query.State]; !ok {
		return Page{}, errors.New("state 选项无效")
	}
	if query.Level == "" {
		query.Level = "all"
	}
	if _, ok := map[string]struct{}{"all": {}, "info": {}, "warning": {}, "error": {}}[query.Level]; !ok {
		return Page{}, errors.New("level 选项无效")
	}
	if query.Page < 1 || query.PageSize < 1 || query.PageSize > 200 {
		return Page{}, errors.New("page 必须大于 0，page_size 必须在 1 到 200 之间")
	}
	return s.currentPage(ctx, query, strings.TrimSpace(query.Search) == "")
}

func pageFromSnapshot(query Query, snapshot *logSnapshot) Page {
	taskEntries := snapshot.taskEntries
	eventEntries := snapshot.eventEntries
	changeEntries := snapshot.changeEntries
	linkedEvents := snapshot.linkedEvents
	linkedChanges := snapshot.linkedChanges
	selected := []Entry{}
	switch query.Kind {
	case "task":
		selected = taskEntries
	case "event":
		selected = eventEntries
	case "change":
		selected = changeEntries
	default:
		selected = append(selected, taskEntries...)
		for _, entry := range eventEntries {
			if _, linked := linkedEvents[entry.ID]; !linked {
				selected = append(selected, entry)
			}
		}
		for _, entry := range changeEntries {
			if _, linked := linkedChanges[entry.ID]; !linked {
				selected = append(selected, entry)
			}
		}
	}
	search := strings.ToLower(strings.TrimSpace(query.Search))
	filtered := make([]Entry, 0, len(selected))
	for _, entry := range selected {
		if query.State != "all" && normalizedState(entry.Status) != query.State {
			continue
		}
		if query.Level != "all" && (entry.Kind != "event" || eventLevel(entry.Status) != query.Level) {
			continue
		}
		if (strings.TrimSpace(query.Group) != "" || strings.TrimSpace(query.GroupID) != "") &&
			(entry.Kind != "event" || !eventMatchesGroup(entry, query.Group, query.GroupID)) {
			continue
		}
		if search != "" {
			encoded, _ := json.Marshal(entry)
			if !strings.Contains(strings.ToLower(string(encoded)), search) {
				continue
			}
		}
		filtered = append(filtered, entry)
	}
	sort.SliceStable(filtered, func(i, j int) bool { return timestamp(filtered[i].OccurredAt).After(timestamp(filtered[j].OccurredAt)) })
	total := len(filtered)
	start := (query.Page - 1) * query.PageSize
	if start > total {
		start = total
	}
	end := start + query.PageSize
	if end > total {
		end = total
	}
	return Page{
		Items: filtered[start:end], Total: total, Page: query.Page, PageSize: query.PageSize,
		Counts:    map[string]int{"task": len(taskEntries), "event": len(eventEntries), "change": len(changeEntries)},
		Truncated: snapshot.truncated,
	}
}

func (s *Service) currentPage(ctx context.Context, query Query, allowStale bool) (Page, error) {
	for {
		s.mu.Lock()
		current, exists := s.cache[query]
		fresh := exists && time.Since(current.loadedAt) < cacheFreshness
		if fresh {
			s.mu.Unlock()
			return current.page, nil
		}
		if exists && allowStale {
			launch := false
			if s.loading[query] == nil {
				s.loading[query] = make(chan struct{})
				launch = true
			}
			s.mu.Unlock()
			if launch {
				if err := taskrunner.Go(s.runner, func(parent context.Context) { s.refreshPage(parent, query) }); err != nil {
					s.finishRefresh(query, Page{}, err)
				}
			}
			return current.page, nil
		}
		if loading := s.loading[query]; loading != nil {
			s.mu.Unlock()
			select {
			case <-ctx.Done():
				return Page{}, ctx.Err()
			case <-loading:
			}
			continue
		}
		s.loading[query] = make(chan struct{})
		s.mu.Unlock()

		loaded, err := s.loadPage(ctx, query)
		s.finishRefresh(query, loaded, err)
		if err != nil {
			return Page{}, err
		}
		return loaded, nil
	}
}

func (s *Service) refreshPage(parent context.Context, query Query) {
	ctx, cancel := context.WithTimeout(parent, refreshTimeout)
	defer cancel()
	loaded, err := s.loadPage(ctx, query)
	s.finishRefresh(query, loaded, err)
}

func (s *Service) finishRefresh(query Query, loaded Page, err error) {
	s.mu.Lock()
	if err == nil {
		now := time.Now()
		s.cache[query] = cachedPage{page: loaded, loadedAt: now}
		for key, value := range s.cache {
			if now.Sub(value.loadedAt) > maximumCacheAge {
				delete(s.cache, key)
			}
		}
	}
	loading := s.loading[query]
	delete(s.loading, query)
	if loading != nil {
		close(loading)
	}
	s.mu.Unlock()
}

func (s *Service) loadPage(ctx context.Context, query Query) (Page, error) {
	search := strings.TrimSpace(query.Search)
	useSummaries := search == ""
	snapshot, err := s.loadSnapshot(ctx, useSummaries, search)
	if err != nil {
		return Page{}, err
	}
	page := pageFromSnapshot(query, snapshot)
	return page, nil
}

func (s *Service) loadSnapshot(ctx context.Context, useTaskSummaries bool, taskSearch string) (*logSnapshot, error) {
	type taskResult struct {
		items []taskstore.Task
		err   error
	}
	type runResult struct {
		items []business.RunRecord
		err   error
	}
	type eventResult struct {
		items []business.RunEvent
		err   error
	}
	type auditResult struct {
		items []business.AuditEvent
		err   error
	}
	taskResults := make(chan taskResult, 1)
	runResults := make(chan runResult, 1)
	eventResults := make(chan eventResult, 1)
	auditResults := make(chan auditResult, 1)
	go func() {
		limit := sourceLimit
		var items []taskstore.Task
		var err error
		if useTaskSummaries {
			items, err = s.tasks.ListLogSummaries(ctx, &limit)
		} else if taskSearch != "" {
			items, err = s.tasks.SearchLogs(ctx, taskSearch, &limit)
		}
		taskResults <- taskResult{items: items, err: err}
	}()
	go func() {
		limit := sourceLimit
		var items []business.RunRecord
		var err error
		if taskSearch != "" {
			if reader, ok := s.business.(businessLogSearcher); ok {
				items, err = reader.SearchRunRecords(ctx, taskSearch, &limit)
			} else {
				items, err = s.business.RunRecords(ctx, &limit)
			}
		} else {
			items, err = s.business.RunRecords(ctx, &limit)
		}
		runResults <- runResult{items: items, err: err}
	}()
	go func() {
		limit := sourceLimit
		var items []business.RunEvent
		var err error
		if taskSearch != "" {
			if reader, ok := s.business.(businessLogSearcher); ok {
				items, err = reader.SearchEvents(ctx, taskSearch, &limit)
			} else {
				items, err = s.business.Events(ctx, &limit)
			}
		} else {
			items, err = s.business.Events(ctx, &limit)
		}
		eventResults <- eventResult{items: items, err: err}
	}()
	go func() {
		limit := sourceLimit
		var items []business.AuditEvent
		var err error
		if taskSearch != "" {
			if reader, ok := s.business.(businessLogSearcher); ok {
				items, err = reader.SearchAuditEvents(ctx, taskSearch, &limit)
			} else {
				items, err = s.business.AuditEvents(ctx, &limit, true)
			}
		} else {
			items, err = s.business.AuditEvents(ctx, &limit, true)
		}
		auditResults <- auditResult{items: items, err: err}
	}()
	taskLoad, runLoad, eventLoad, auditLoad := <-taskResults, <-runResults, <-eventResults, <-auditResults
	if err := errors.Join(taskLoad.err, runLoad.err, eventLoad.err, auditLoad.err); err != nil {
		return nil, err
	}
	tasks, runs, events, audits := taskLoad.items, runLoad.items, eventLoad.items, auditLoad.items
	taskEntries := make([]Entry, 0, len(tasks)+len(runs))
	index := map[string]*Entry{}
	byRunKey := map[string]*Entry{}
	for _, task := range tasks {
		entry := taskEntry(task)
		taskEntries = append(taskEntries, entry)
		item := &taskEntries[len(taskEntries)-1]
		index[item.SourceID] = item
		if runKey := stringValue(task.Result["run_key"]); runKey != "" {
			byRunKey[runKey] = item
		}
	}
	for _, run := range runs {
		entry := runEntry(run)
		if parent := byRunKey[entry.SourceID]; parent != nil {
			attach(parent, "runs", entry)
			index[entry.SourceID] = parent
		} else {
			taskEntries = append(taskEntries, entry)
			index[entry.SourceID] = &taskEntries[len(taskEntries)-1]
		}
	}
	eventEntries := make([]Entry, len(events))
	linkedEvents := map[string]struct{}{}
	for idx, event := range events {
		eventEntries[idx] = eventEntry(event)
		runKey := stringValue(event.Payload["run_key"])
		if parent := index[runKey]; parent != nil && runKey != "" {
			attach(parent, "events", eventEntries[idx])
			linkedEvents[eventEntries[idx].ID] = struct{}{}
		}
	}
	changeEntries := changeEntries(audits)
	linkedChanges := map[string]struct{}{}
	for idx := range changeEntries {
		if parent := index[changeEntries[idx].SourceID]; parent != nil {
			attach(parent, "changes", changeEntries[idx])
			linkedChanges[changeEntries[idx].ID] = struct{}{}
		}
	}
	return &logSnapshot{
		taskEntries: taskEntries, eventEntries: eventEntries, changeEntries: changeEntries,
		linkedEvents: linkedEvents, linkedChanges: linkedChanges,
		truncated: len(tasks) >= sourceLimit || len(runs) >= sourceLimit || len(events) >= sourceLimit || len(audits) >= sourceLimit,
	}, nil
}

func taskEntry(task taskstore.Task) Entry {
	var object *string
	for _, field := range []string{"host", "account_name", "account_id", "object_label"} {
		if value := stringValue(task.Result[field]); value != "" {
			object = &value
			break
		}
	}
	return Entry{ID: "task:" + task.ID, Kind: "task", OccurredAt: task.UpdatedAt, Title: task.Operation, Summary: task.Message,
		Status: task.Status, ObjectLabel: object, Source: "task", SourceID: task.ID, Details: map[string]any{
			"skill": task.Skill, "operation": task.Operation, "progress": task.Progress, "created_at": task.CreatedAt, "result": task.Result,
		}}
}

func runEntry(run business.RunRecord) Entry {
	occurred := firstText(run.EndedAt, &run.UpdatedAt, run.StartedAt)
	title := "运行任务"
	if strings.TrimSpace(run.TaskName) != "" {
		title = run.TaskName
	}
	summary := title
	if run.Summary != nil && strings.TrimSpace(*run.Summary) != "" {
		summary = *run.Summary
	}
	return Entry{ID: "run:" + run.RunKey, Kind: "task", OccurredAt: occurred, Title: title, Summary: summary,
		Status: pointerText(run.Status, "unknown"), Source: "run_record", SourceID: run.RunKey, Details: map[string]any{
			"task_name": run.TaskName, "stage": run.Stage, "started_at": run.StartedAt, "ended_at": run.EndedAt,
			"duration_seconds": run.DurationSeconds, "payload": run.Payload,
		}}
}

func eventEntry(event business.RunEvent) Entry {
	actor := optionalText(event.Payload["actor"])
	var object *string
	for _, field := range []string{"host", "account_name", "account_id"} {
		if value := optionalText(event.Payload[field]); value != nil {
			object = value
			break
		}
	}
	title := event.EventType
	if strings.TrimSpace(title) == "" {
		title = "事件日志"
	}
	summary := event.Summary
	if strings.TrimSpace(summary) == "" {
		summary = title
	}
	return Entry{ID: fmt.Sprintf("event:%d", event.ID), Kind: "event", OccurredAt: event.CreatedAt, Title: title, Summary: summary,
		Status: event.Status, Actor: actor, ObjectLabel: object, Source: "runtime_event", SourceID: fmt.Sprint(event.ID),
		Details: map[string]any{"event_type": event.EventType, "payload": event.Payload}}
}

func changeEntries(rows []business.AuditEvent) []Entry {
	groups := map[string][]business.AuditEvent{}
	for _, row := range rows {
		key := auditGroupKey(row)
		if key == "" {
			key = fmt.Sprintf("audit:%d", row.ID)
		}
		groups[key] = append(groups[key], row)
	}
	result := make([]Entry, 0, len(groups))
	for operationID, changes := range groups {
		sort.SliceStable(changes, func(i, j int) bool { return timestamp(changes[i].CreatedAt).After(timestamp(changes[j].CreatedAt)) })
		first := changes[0]
		failed := false
		fields, names := map[string]struct{}{}, map[string]struct{}{}
		var errorText *string
		for _, change := range changes {
			if normalizedState(change.State) == "failed" {
				failed = true
			}
			if change.FieldName != nil && *change.FieldName != "" {
				fields[*change.FieldName] = struct{}{}
			}
			name := firstText(change.ObjectName, change.ObjectID)
			if name != "" {
				names[name] = struct{}{}
			}
			if errorText == nil && change.Error != nil && *change.Error != "" {
				errorText = change.Error
			}
		}
		var object *string
		if len(names) == 1 {
			for name := range names {
				object = &name
			}
		} else if len(names) > 1 {
			text := fmt.Sprintf("%d 个对象", len(names))
			object = &text
		}
		fieldCount := len(fields)
		if fieldCount == 0 {
			fieldCount = len(changes)
		}
		title, summary := auditActionSummary(changes, len(names), fieldCount, errorText)
		status := first.State
		if failed {
			status = "failed"
		}
		result = append(result, Entry{ID: "change:" + operationID, Kind: "change", OccurredAt: first.CreatedAt,
			Title: title, Summary: summary, Status: status, Actor: first.Actor, ObjectLabel: object,
			Source: "operation_audit", SourceID: operationID, RelatedCount: len(changes), Details: map[string]any{
				"operation_id": operationID, "operation_type": first.OperationType, "phase": first.Phase,
				"request_id": first.RequestID, "source": first.Source, "changes": changes,
			}})
	}
	return result
}

func auditGroupKey(row business.AuditEvent) string {
	if !row.Writeback && row.RemoteConfirmed != nil && !*row.RemoteConfirmed &&
		row.ReadbackConfirmed != nil && *row.ReadbackConfirmed &&
		(row.OperationType == "routing.writeback" || row.OperationType == "account.scheduling") {
		// One inspection can read hundreds of accounts. Keep the audit useful without
		// turning every confirmed no-op readback into a separate table row.
		bucket := timestamp(row.CreatedAt).UTC().Truncate(time.Second).Format(time.RFC3339)
		return "readback:" + row.OperationType + ":" + pointerText(row.Actor, "system") + ":" + bucket
	}
	if row.OperationID != "" {
		return row.OperationID
	}
	return fmt.Sprintf("audit:%d", row.ID)
}

func auditActionSummary(changes []business.AuditEvent, objectCount, fieldCount int, failure *string) (string, string) {
	first := changes[0]
	writeback := false
	remoteConfirmed := false
	readbackConfirmed := false
	for _, change := range changes {
		writeback = writeback || change.Writeback
		remoteConfirmed = remoteConfirmed || (change.RemoteConfirmed != nil && *change.RemoteConfirmed)
		readbackConfirmed = readbackConfirmed || (change.ReadbackConfirmed != nil && *change.ReadbackConfirmed)
	}
	if !writeback && readbackConfirmed {
		return "远程读取复核", fmt.Sprintf("读取并复核 %d 个账号，远程状态已符合调度目标", max(objectCount, len(changes)))
	}
	if failure != nil {
		if first.OperationType == "account.delete" {
			return accountDeleteFailureSummary(first, *failure)
		}
		if remoteConfirmed && !readbackConfirmed || first.Phase == "remote-readback" {
			return "写后复核", "远程写入已提交，写后读取不一致：" + *failure
		}
		return "远程写入", "远程写入失败：" + *failure
	}
	if remoteConfirmed && readbackConfirmed {
		return "远程写入与复核", fmt.Sprintf("%d 个对象写入成功，写后复核一致，涉及 %d 个字段", max(objectCount, 1), fieldCount)
	}
	return first.OperationType, fmt.Sprintf("%d 条操作记录，涉及 %d 个字段", len(changes), fieldCount)
}

func accountDeleteFailureSummary(change business.AuditEvent, failure string) (string, string) {
	switch change.Phase {
	case "upstream-key-readback":
		return "上游 Key 删除结果未确认", "上游 Key DELETE 已发出，但删除后读回未确认：" + failure
	case "upstream-key-reconcile", "upstream-key-secret-reconcile":
		return "上游 Key 本地对账失败", "上游 Key 已确认不存在，但本地对账未完成：" + failure
	case "management-readback-still-readable":
		return "管理账号仍存在", "管理 DELETE 已发出，读回确认账号仍存在：" + failure
	case "management-readback":
		return "管理账号删除结果未知", "管理 DELETE 已发出，但删除后读回失败：" + failure
	case "local-commit":
		return "本地账号清理失败", "远端删除均已确认，但本地账号记录清理失败：" + failure
	}
	if auditAfterBool(change.After, "upstream_key_deleted") {
		if auditAfterBool(change.After, "upstream_key_delete_requested") {
			return "部分删除已确认", "上游 Key 删除已确认，后续删除未完成：" + failure
		}
		return "远程删除未发出", "上游 Key 已确认不存在，未发出后续远程删除：" + failure
	}
	if !change.Writeback {
		return "远程删除未发出", "未发出远程删除：" + failure
	}
	return "远程删除失败", "远程删除未完成：" + failure
}

func auditAfterBool(value any, key string) bool {
	fields, ok := value.(map[string]any)
	if !ok {
		return false
	}
	result, _ := fields[key].(bool)
	return result
}

func attach(parent *Entry, key string, related Entry) {
	values, _ := parent.Details[key].([]Entry)
	parent.Details[key] = append(values, related)
	parent.RelatedCount++
}

func normalizedState(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "queued", "running", "waiting_input", "pending":
		return "active"
	case "failed", "error", "cancelled", "fused":
		return "failed"
	case "warning", "warn", "partial", "degraded":
		return "warning"
	default:
		return "succeeded"
	}
}

func eventLevel(value string) string {
	switch normalizedState(value) {
	case "failed":
		return "error"
	case "warning":
		return "warning"
	default:
		return "info"
	}
}

func eventMatchesGroup(entry Entry, groups ...string) bool {
	targets := []string{}
	for _, group := range groups {
		if target := strings.ToLower(strings.TrimSpace(group)); target != "" {
			targets = append(targets, target)
		}
	}
	if len(targets) == 0 {
		return true
	}
	payload, _ := entry.Details["payload"].(map[string]any)
	for _, key := range []string{"group", "group_name", "group_id", "groups", "group_names", "group_ids"} {
		for _, target := range targets {
			if groupValueMatches(payload[key], target) {
				return true
			}
		}
	}
	return false
}

func groupValueMatches(value any, target string) bool {
	switch typed := value.(type) {
	case []string:
		for _, item := range typed {
			if strings.EqualFold(strings.TrimSpace(item), target) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if groupValueMatches(item, target) {
				return true
			}
		}
	default:
		return strings.EqualFold(stringValue(value), target)
	}
	return false
}

func timestamp(value string) time.Time {
	parsed, _ := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
	return parsed
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func optionalText(value any) *string {
	text := stringValue(value)
	if text == "" {
		return nil
	}
	return &text
}

func pointerText(value *string, fallback string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return fallback
	}
	return *value
}

func firstText(values ...*string) string {
	for _, value := range values {
		if value != nil && strings.TrimSpace(*value) != "" {
			return *value
		}
	}
	return ""
}
