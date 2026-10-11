package accountautomation_test

import (
	"context"
	"maps"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountautomation"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/newapimanagement"
	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"
)

type configurationManager struct {
	*manager
	once             sync.Once
	entered, release chan struct{}
}

func (m *configurationManager) ChannelsByID(ctx context.Context, platform string, ids []string, offset, limit int) (newapimanagement.ChannelPage, error) {
	m.once.Do(func() { close(m.entered); <-m.release })
	return m.manager.ChannelsByID(ctx, platform, ids, offset, limit)
}

func TestScheduleStartupPreservesConfigurationSavedDuringChannelRead(t *testing.T) {
	for _, operation := range []string{"edit", "delete", "add"} {
		t.Run(operation, func(t *testing.T) {
			_, accounts, management, tasks, request := automationFixture()
			remote := &configurationManager{manager: management, entered: make(chan struct{}), release: make(chan struct{})}
			var release sync.Once
			defer release.Do(func() { close(remote.release) })
			service := accountautomation.New(accounts, remote, nil, tasks, immediateRunner{}, accounts)
			repo, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer repo.Close()
			defer service.StopSchedules()
			service.UseScheduleRepository(repo)
			schedule := saveTaskGroupSchedule(t, service, request, "原计划")
			done := make(chan error, 1)
			go func() { _, err := service.RunSchedule(context.Background(), schedule.ID); done <- err }()
			<-remote.entered
			switch operation {
			case "edit":
				schedule.Name, schedule.IntervalMinutes = "已修改", 120
				_, err = service.SaveSchedule(context.Background(), schedule)
			case "delete":
				err = service.DeleteSchedule(context.Background(), schedule.ID)
			case "add":
				saveTaskGroupSchedule(t, service, request, "新增计划")
			}
			if err != nil {
				t.Fatal(err)
			}
			before, err := service.Schedules(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			release.Do(func() { close(remote.release) })
			runErr := <-done
			if err := service.StopSchedulesContext(context.Background()); err != nil {
				t.Fatal(err)
			}
			after, err := service.Schedules(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if operation == "add" {
				if runErr != nil || len(after) != 2 || after[1].ID != before[1].ID || after[0].LastRunAt == nil {
					t.Fatalf("startup lost another schedule or its run timestamp: %v err=%v", after, runErr)
				}
				return
			}
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("startup overwrote concurrent %s: before=%v after=%v", operation, before, after)
			}
			if runErr == nil || management.publishCalls != 0 || management.priceCalls != 0 || accounts.syncCalls != 0 {
				t.Fatalf("changed schedule started writes: err=%v publish=%d price=%d sync=%d", runErr, management.publishCalls, management.priceCalls, accounts.syncCalls)
			}
		})
	}
}

func TestScheduleDeletionRollsBackWhenTaskGroupSaveFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.db")
	repo, err := configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	service, _, _, _, request := automationFixture()
	service.UseScheduleRepository(repo)
	first := saveTaskGroupSchedule(t, service, request, "第一项")
	second := saveTaskGroupSchedule(t, service, request, "第二项")
	group, err := service.SaveTaskGroup(context.Background(), accountautomation.TaskGroup{Name: "关联任务组", ScheduleIDs: []string{first.ID, second.ID}})
	if err != nil {
		t.Fatal(err)
	}
	db, err := sqliteutil.OpenDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TRIGGER fail_group_save BEFORE INSERT ON settings WHEN NEW.key='account_model_automation.task_groups' BEGIN SELECT RAISE(ABORT, 'group save blocked'); END`)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteSchedule(context.Background(), first.ID); err == nil || !strings.Contains(err.Error(), "group save blocked") {
		t.Fatalf("expected write failure, got %v", err)
	}
	schedules, err := service.Schedules(context.Background())
	if err != nil || len(schedules) != 2 || schedules[0].ID != first.ID {
		t.Fatalf("failed delete removed schedule: %v err=%v", schedules, err)
	}
	groups, err := service.TaskGroups(context.Background())
	if err != nil || len(groups) != 1 || !reflect.DeepEqual(groups[0], group) {
		t.Fatalf("failed delete changed task group: %v err=%v", groups, err)
	}
}

// Pause a storage snapshot to expose lost read/modify/write updates without
// sleeps. Transactional updates hold the storage lock until their commit.
type pausedSettings struct {
	mu               chan struct{}
	values           map[string]string
	pause            bool
	pauseKey         string
	entered, release chan struct{}
}

func (s *pausedSettings) JSONSetting(_ context.Context, key string) (string, error) {
	s.mu <- struct{}{}
	value, pause := s.values[key], s.pause && key == s.pauseKey
	if pause {
		s.pause = false
	}
	<-s.mu
	if pause {
		close(s.entered)
		<-s.release
	}
	return value, nil
}

func (s *pausedSettings) SaveJSONSetting(_ context.Context, key, value string) error {
	s.mu <- struct{}{}
	defer func() { <-s.mu }()
	s.values[key] = value
	return nil
}

func (s *pausedSettings) UpdateJSONSettings(_ context.Context, keys []string, update func(map[string]string) (map[string]string, error)) error {
	s.mu <- struct{}{}
	defer func() { <-s.mu }()
	if s.pause {
		s.pause = false
		close(s.entered)
		<-s.release
	}
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		values[key] = s.values[key]
	}
	changes, err := update(values)
	if err == nil {
		maps.Copy(s.values, changes)
	}
	return err
}

func TestConcurrentConfigurationSavesRetainBothEntries(t *testing.T) {
	for _, kind := range []string{"schedule", "task-group"} {
		t.Run(kind, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				service, _, _, _, request := automationFixture()
				repo := &pausedSettings{mu: make(chan struct{}, 1), values: map[string]string{}, entered: make(chan struct{}), release: make(chan struct{})}
				service.UseScheduleRepository(repo)
				first := saveTaskGroupSchedule(t, service, request, "原计划一")
				second := saveTaskGroupSchedule(t, service, request, "原计划二")
				save := func(name string) error {
					if kind == "task-group" {
						_, err := service.SaveTaskGroup(context.Background(), accountautomation.TaskGroup{Name: name, ScheduleIDs: []string{first.ID, second.ID}})
						return err
					}
					_, err := service.SaveSchedule(context.Background(), accountautomation.Schedule{Name: name, Request: request, IntervalMinutes: 60})
					return err
				}
				repo.pause = true
				repo.pauseKey = "account_model_automation.schedules"
				if kind == "task-group" {
					repo.pauseKey = "account_model_automation.task_groups"
				}
				results := make(chan error, 2)
				go func() { results <- save("新增一") }()
				<-repo.entered
				go func() { results <- save("新增二") }()
				synctest.Wait()
				close(repo.release)
				for range 2 {
					if err := <-results; err != nil {
						t.Fatal(err)
					}
				}
				if kind == "task-group" {
					groups, err := service.TaskGroups(context.Background())
					if err != nil || len(groups) != 2 {
						t.Fatalf("concurrent group save lost an entry: %v err=%v", groups, err)
					}
				} else {
					schedules, err := service.Schedules(context.Background())
					if err != nil || len(schedules) != 4 {
						t.Fatalf("concurrent schedule save lost an entry: %v err=%v", schedules, err)
					}
				}
			})
		})
	}
}
