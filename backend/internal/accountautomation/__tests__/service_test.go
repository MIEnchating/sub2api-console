package accountautomation_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountautomation"
	"github.com/MIEnchating/sub2api-console/backend/internal/newapimanagement"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type accountSource struct {
	models       []string
	err          error
	syncCalls    int
	previewCalls int
	previewStart chan struct{}
}

func (s *accountSource) Models(context.Context, string) ([]string, error) {
	s.syncCalls++
	return s.models, s.err
}
func (s *accountSource) PreviewModels(context.Context, string) ([]string, error) {
	s.previewCalls++
	if s.previewStart != nil {
		s.previewStart <- struct{}{}
	}
	return s.models, s.err
}
func (s *accountSource) AccountIDsByGroups(context.Context, []string) ([]string, error) {
	return []string{"10"}, nil
}

type manager struct {
	channel      newapimanagement.Channel
	prices       []newapimanagement.Sub2APIModelPrice
	failPublish  bool
	cancel       context.CancelFunc
	publishCalls int
	priceCalls   int
	channelReads int
	catalogReads int
	writeStarted chan struct{}
	writeRelease chan struct{}
	publishStart chan struct{}
}

func (m *manager) ChannelsByID(context.Context, string, []string, int, int) (newapimanagement.ChannelPage, error) {
	m.channelReads++
	return newapimanagement.ChannelPage{Items: []newapimanagement.Channel{m.channel}, Total: 1}, nil
}
func (m *manager) ChangeChannelModels(_ context.Context, _, _ string, change newapimanagement.ChannelModelChange) (newapimanagement.Channel, error) {
	m.publishCalls++
	if m.publishStart != nil {
		m.publishStart <- struct{}{}
	}
	if m.writeStarted != nil {
		m.writeStarted <- struct{}{}
		<-m.writeRelease
	}
	if m.failPublish {
		return newapimanagement.Channel{}, errors.New("远端未确认写入")
	}
	m.channel.Models = append(m.channel.Models, change.Models...)
	if m.cancel != nil {
		m.cancel()
	}
	return m.channel, nil
}
func (m *manager) ModelPriceCatalog(context.Context, string, bool) (newapimanagement.ModelPriceCatalog, error) {
	m.catalogReads++
	return newapimanagement.ModelPriceCatalog{Models: m.prices}, nil
}
func (m *manager) SaveModelPrices(context.Context, string, []newapimanagement.ModelPriceInput) (newapimanagement.RemoteSnapshot, error) {
	m.priceCalls++
	return newapimanagement.RemoteSnapshot{}, nil
}

type memoryTasks struct {
	mu        sync.Mutex
	items     map[string]taskstore.Task
	completed chan struct{}
}

type memorySchedule struct {
	mu  sync.Mutex
	raw string
}

func (s *memorySchedule) JSONSetting(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if key != "account_model_automation.schedules" {
		return "", nil
	}
	return s.raw, nil
}

func (s *memorySchedule) UpdateJSONSettings(_ context.Context, _ []string, update func(map[string]string) (map[string]string, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	changes, err := update(map[string]string{"account_model_automation.schedules": s.raw})
	if err != nil {
		return err
	}
	if raw, ok := changes["account_model_automation.schedules"]; ok {
		s.raw = raw
	}
	return nil
}

func (s *memoryTasks) Save(_ context.Context, task taskstore.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.items == nil {
		s.items = map[string]taskstore.Task{}
	}
	raw, err := json.Marshal(task.Result)
	if err != nil {
		return err
	}
	copy := task
	if err := json.Unmarshal(raw, &copy.Result); err != nil {
		return err
	}
	s.items[task.ID] = copy
	if s.completed != nil && task.Result["dry_run"] == false &&
		(task.Status == "succeeded" || task.Status == "failed" || task.Status == "partial" || task.Status == "cancelled") {
		select {
		case s.completed <- struct{}{}:
		default:
		}
	}
	return nil
}
func (s *memoryTasks) Get(_ context.Context, id string) (taskstore.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[id]
	if !ok {
		return taskstore.Task{}, taskstore.ErrNotFound
	}
	return item, nil
}

func (s *memoryTasks) CancelTask(_ context.Context, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[id]
	if !ok || (item.Status != "queued" && item.Status != "running" && item.Status != "waiting_input") {
		return false, nil
	}
	item.Status = "cancelled"
	item.Recovery = nil
	s.items[id] = item
	return true, nil
}

type immediateRunner struct{ ctx context.Context }

func (r immediateRunner) Go(run func(context.Context)) error {
	ctx := r.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	run(ctx)
	return nil
}
func (r immediateRunner) GoTask(_ string, run func(context.Context)) error {
	ctx := r.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	run(ctx)
	return nil
}
func (immediateRunner) CancelTask(string) bool { return false }

func automationFixture() (*accountautomation.Service, *accountSource, *manager, *memoryTasks, accountautomation.Request) {
	accounts := &accountSource{models: []string{"model-a"}}
	management := &manager{
		channel: newapimanagement.Channel{ID: "11", Version: strings.Repeat("a", 64)},
		prices:  []newapimanagement.Sub2APIModelPrice{{Model: "model-a", ModelRatio: "0.5", CompletionRatio: "2"}},
	}
	store := &memoryTasks{}
	request := accountautomation.Request{SelectedModels: []string{"model-a"}, GroupIDs: []string{"7"}, PlatformID: "platform", Channels: []accountautomation.ChannelTarget{{ID: "11", Version: management.channel.Version}}, PublishMissing: true, PriceEnabled: true, PricingSource: "official", DryRun: true}
	return accountautomation.New(accounts, management, nil, store, immediateRunner{}, accounts), accounts, management, store, request
}

func TestPriceInputsSkipMissingAndInvalidOfficialPrices(t *testing.T) {
	prices := []newapimanagement.Sub2APIModelPrice{
		{Model: "model-a", ModelRatio: "0.5", CompletionRatio: "2", CacheRatio: "0.1"},
		{Model: "model-b", ModelRatio: "", CompletionRatio: "2"},
		{Model: "model-c", ModelRatio: "1", CompletionRatio: "2", SyncError: "官方页面解析不完整"},
	}
	got := accountautomation.BuildPriceInputs([]string{"MODEL-A", "model-b", "model-c"}, prices)
	if len(got) != 1 || got[0].Model != "MODEL-A" || got[0].InputRatio != "0.5" || got[0].BillingExpr != "" {
		t.Fatalf("price inputs = %#v", got)
	}
}

func TestWriteWithoutCompletedPreviewIsRejected(t *testing.T) {
	service, _, management, _, request := automationFixture()
	request.DryRun = false
	if _, err := service.Enqueue(context.Background(), request); err == nil || !strings.Contains(err.Error(), "预览") {
		t.Fatalf("expected preview rejection, got %v", err)
	}
	if management.publishCalls != 0 || management.priceCalls != 0 {
		t.Fatal("remote writes happened without preview")
	}
}

func TestPreviewReadsModelsWithoutStartingAccountSync(t *testing.T) {
	service, accounts, management, store, request := automationFixture()
	preview, err := service.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if store.items[preview.ID].Status != "succeeded" || accounts.previewCalls != 1 || accounts.syncCalls != 0 || management.publishCalls != 0 || management.priceCalls != 0 {
		t.Fatalf("preview performed a write: account=%#v manager=%#v", accounts, management)
	}
}

func TestUnavailableSelectionAfterPreviewSkipsWrites(t *testing.T) {
	service, accounts, management, store, request := automationFixture()
	preview, err := service.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if store.items[preview.ID].Status != "succeeded" {
		t.Fatalf("preview status = %s", store.items[preview.ID].Status)
	}
	accounts.models = []string{"model-b"}
	request.DryRun, request.PreviewTaskID = false, preview.ID
	execution, err := service.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if store.items[execution.ID].Status != "succeeded" || management.publishCalls != 0 || management.priceCalls != 0 {
		t.Fatalf("unavailable selection must be skipped: task=%#v publish=%d price=%d", store.items[execution.ID], management.publishCalls, management.priceCalls)
	}
}

func TestFailedChannelPublishDoesNotPriceUnverifiedModel(t *testing.T) {
	service, _, management, store, request := automationFixture()
	preview, err := service.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	management.failPublish = true
	request.DryRun, request.PreviewTaskID = false, preview.ID
	execution, err := service.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if store.items[execution.ID].Status != "partial" || management.publishCalls != 1 || management.priceCalls != 0 {
		t.Fatalf("failed publish must not price: task=%#v publish=%d price=%d", store.items[execution.ID], management.publishCalls, management.priceCalls)
	}
}

func TestAccountReadFailureAfterPreviewStopsBeforeRemoteWrites(t *testing.T) {
	service, accounts, management, store, request := automationFixture()
	preview, err := service.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	accounts.err = errors.New("上游账号不可用")
	request.DryRun, request.PreviewTaskID = false, preview.ID
	execution, err := service.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if store.items[execution.ID].Status != "failed" || management.publishCalls != 0 || management.priceCalls != 0 {
		t.Fatalf("partial discovery must not write: task=%#v", store.items[execution.ID])
	}
}

func TestPriceChangeAfterPreviewStopsBeforeChannelWrite(t *testing.T) {
	service, _, management, store, request := automationFixture()
	preview, err := service.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	management.prices[0].ModelRatio = "0.8"
	request.DryRun, request.PreviewTaskID = false, preview.ID
	execution, err := service.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if store.items[execution.ID].Status != "failed" || management.publishCalls != 0 || management.priceCalls != 0 {
		t.Fatalf("changed price must not write: task=%#v", store.items[execution.ID])
	}
}

func TestExistingChannelModelIsReportedAndNotPublishedAgain(t *testing.T) {
	service, _, management, store, request := automationFixture()
	management.channel.Models = []string{" model-a "}
	preview, err := service.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.DryRun, request.PreviewTaskID = false, preview.ID
	execution, err := service.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if management.publishCalls != 0 || management.priceCalls != 0 || store.items[execution.ID].Status != "succeeded" {
		t.Fatalf("existing model must be skipped: task=%#v publish=%d price=%d", store.items[execution.ID], management.publishCalls, management.priceCalls)
	}
	items, ok := store.items[execution.ID].Result["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("missing channel result: %#v", store.items[execution.ID].Result)
	}
	for _, rawItem := range items {
		channelResult, ok := rawItem.(map[string]any)
		if ok && channelResult["channel_id"] == "11" {
			if channelResult["existing"] == nil {
				t.Fatalf("existing channel model was not reported as skipped: %#v", channelResult)
			}
			return
		}
	}
	t.Fatalf("channel result is missing: %#v", items)
}

func TestSchedulesPersistIndependentGroupChannelDefinitions(t *testing.T) {
	service, _, management, _, request := automationFixture()
	schedules := &memorySchedule{}
	service.UseScheduleRepository(schedules)
	saved, err := service.SaveSchedule(context.Background(), accountautomation.Schedule{
		Name:            "生产组",
		Request:         request,
		Enabled:         true,
		IntervalMinutes: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	secondRequest := request
	secondRequest.GroupIDs = []string{"8"}
	secondRequest.Channels = []accountautomation.ChannelTarget{{ID: "12", Version: strings.Repeat("c", 64)}}
	secondRequest.PriceEnabled = false
	second, err := service.SaveSchedule(context.Background(), accountautomation.Schedule{
		Name:            "研发组",
		Request:         secondRequest,
		Enabled:         false,
		IntervalMinutes: 30,
	})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := service.Schedules(context.Background())
	if err != nil || len(loaded) != 2 || loaded[0].ID != saved.ID || loaded[1].ID != second.ID ||
		loaded[0].Request.GroupIDs[0] != "7" || loaded[0].Request.Channels[0].ID != "11" ||
		loaded[1].Request.GroupIDs[0] != "8" || loaded[1].Request.Channels[0].ID != "12" || loaded[1].Request.PriceEnabled {
		t.Fatalf("schedule was not persisted: %#v err=%v", loaded, err)
	}
	if management.publishCalls != 0 {
		t.Fatal("saving a schedule must not execute remote writes")
	}
}

func TestEnabledDueScheduleStartsAutomatically(t *testing.T) {
	service, accounts, management, store, request := automationFixture()
	accounts.previewStart = make(chan struct{}, 1)
	management.publishStart = make(chan struct{}, 1)
	store.completed = make(chan struct{}, 1)
	schedules := &memorySchedule{}
	service.UseScheduleRepository(schedules)
	due := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	if _, err := service.SaveSchedule(context.Background(), accountautomation.Schedule{
		Name:            "定时任务",
		Request:         request,
		Enabled:         true,
		IntervalMinutes: 60,
		NextRunAt:       &due,
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service.StartSchedules(ctx)
	defer service.StopSchedules()
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	select {
	case <-management.publishStart:
	case <-deadline.C:
		t.Fatal("enabled due schedule did not reach the channel write")
	}
	select {
	case <-store.completed:
	case <-deadline.C:
		t.Fatal("scheduled write did not reach a terminal task state")
	}
}

func TestScheduledRunRefreshesChannelVersionBeforePreview(t *testing.T) {
	service, _, management, _, request := automationFixture()
	schedules := &memorySchedule{}
	service.UseScheduleRepository(schedules)
	_, err := service.SaveSchedule(context.Background(), accountautomation.Schedule{
		Name:            "生产组",
		Request:         request,
		Enabled:         true,
		IntervalMinutes: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	management.channel.Version = strings.Repeat("b", 64)
	loaded, err := service.Schedules(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	task, err := service.RunSchedule(context.Background(), loaded[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Result["dry_run"] != true {
		t.Fatalf("scheduled run did not start with a preview: %#v", task.Result)
	}
	loaded, err = service.Schedules(context.Background())
	if err != nil || len(loaded) != 1 || loaded[0].Request.Channels[0].Version != management.channel.Version {
		t.Fatalf("schedule did not save the refreshed channel version: %#v err=%v", loaded, err)
	}
}

func TestScheduledRunRejectsOverlapUntilRemoteWriteFinishes(t *testing.T) {
	service, _, management, _, request := automationFixture()
	schedules := &memorySchedule{}
	service.UseScheduleRepository(schedules)
	management.writeStarted = make(chan struct{}, 1)
	management.writeRelease = make(chan struct{})
	defer close(management.writeRelease)
	schedule, err := service.SaveSchedule(context.Background(), accountautomation.Schedule{
		Name:            "生产组",
		Request:         request,
		Enabled:         true,
		IntervalMinutes: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RunSchedule(context.Background(), schedule.ID); err != nil {
		t.Fatal(err)
	}
	<-management.writeStarted
	if _, err := service.RunSchedule(context.Background(), schedule.ID); err == nil || !strings.Contains(err.Error(), "正在执行") {
		t.Fatalf("overlapping run was not rejected: %v", err)
	}
}

func TestCancellationAfterChannelPublishSkipsPricing(t *testing.T) {
	service, accounts, management, store, request := automationFixture()
	preview, err := service.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	management.cancel = cancel
	service = accountautomation.New(accounts, management, nil, store, immediateRunner{ctx: ctx}, accounts)
	request.DryRun, request.PreviewTaskID = false, preview.ID
	execution, err := service.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if store.items[execution.ID].Status != "cancelled" || management.publishCalls != 1 || management.priceCalls != 0 {
		t.Fatalf("cancelled task must skip pricing: task=%#v publish=%d price=%d", store.items[execution.ID], management.publishCalls, management.priceCalls)
	}
}
