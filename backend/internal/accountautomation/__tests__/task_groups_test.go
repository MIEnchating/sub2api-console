package accountautomation_test

import (
	"context"
	"maps"
	"sync"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountautomation"
)

type taskGroupSettings struct {
	mu     sync.Mutex
	values map[string]string
}

func (s *taskGroupSettings) JSONSetting(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.values[key], nil
}

func (s *taskGroupSettings) UpdateJSONSettings(_ context.Context, keys []string, update func(map[string]string) (map[string]string, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		values[key] = s.values[key]
	}
	changes, err := update(values)
	if err != nil {
		return err
	}
	if s.values == nil {
		s.values = map[string]string{}
	}
	maps.Copy(s.values, changes)
	return nil
}

func saveTaskGroupSchedule(t *testing.T, service *accountautomation.Service, request accountautomation.Request, name string) accountautomation.Schedule {
	t.Helper()
	schedule, err := service.SaveSchedule(context.Background(), accountautomation.Schedule{
		Name:            name,
		Request:         request,
		IntervalMinutes: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	return schedule
}

func TestTaskGroupPersistsMembersAndRemovesItWhenScheduleDeletionLeavesOneMember(t *testing.T) {
	service, _, _, _, request := automationFixture()
	service.UseScheduleRepository(&taskGroupSettings{})
	first := saveTaskGroupSchedule(t, service, request, "第一项")
	second := saveTaskGroupSchedule(t, service, request, "第二项")
	group, err := service.SaveTaskGroup(context.Background(), accountautomation.TaskGroup{
		Name:        "发布顺序",
		ScheduleIDs: []string{second.ID, first.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if group.ID == "" {
		t.Fatal("saved task group has no ID")
	}
	groups, err := service.TaskGroups(context.Background())
	if err != nil || len(groups) != 1 || groups[0].ScheduleIDs[0] != second.ID {
		t.Fatalf("task group was not saved in member order: %#v err=%v", groups, err)
	}
	if err := service.DeleteSchedule(context.Background(), first.ID); err != nil {
		t.Fatal(err)
	}
	groups, err = service.TaskGroups(context.Background())
	if err != nil || len(groups) != 0 {
		t.Fatalf("task group with fewer than two members was retained: %#v err=%v", groups, err)
	}
}

func TestTaskGroupRunsSavedSchedulesInOrder(t *testing.T) {
	service, _, management, store, request := automationFixture()
	service.UseScheduleRepository(&taskGroupSettings{})
	service.UseGroupRunner(immediateRunner{})
	first := saveTaskGroupSchedule(t, service, request, "先执行")
	second := saveTaskGroupSchedule(t, service, request, "后执行")
	group, err := service.SaveTaskGroup(context.Background(), accountautomation.TaskGroup{
		Name:        "连续发布",
		ScheduleIDs: []string{first.ID, second.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.RunTaskGroup(context.Background(), group.ID)
	if err != nil {
		t.Fatal(err)
	}
	completed, err := store.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != "succeeded" || completed.Progress != 100 {
		t.Fatalf("task group did not complete: %#v", completed)
	}
	items, ok := completed.Result["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("task group items = %#v", completed.Result["items"])
	}
	firstItem, firstOK := items[0].(map[string]any)
	secondItem, secondOK := items[1].(map[string]any)
	if !firstOK || !secondOK || firstItem["schedule_id"] != first.ID || secondItem["schedule_id"] != second.ID ||
		firstItem["status"] != "succeeded" || secondItem["status"] != "succeeded" {
		t.Fatalf("task group did not preserve successful member order: %#v", items)
	}
	if management.publishCalls != 1 {
		t.Fatalf("expected the first member to publish the missing model, got %d writes", management.publishCalls)
	}
}
