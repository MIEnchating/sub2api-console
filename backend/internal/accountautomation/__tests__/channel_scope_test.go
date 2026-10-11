package accountautomation_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountautomation"
	"github.com/MIEnchating/sub2api-console/backend/internal/newapimanagement"
)

type snapshotManager struct {
	*manager
	channels []newapimanagement.Channel
}

type changingGroups struct{ ids []string }

func (g *changingGroups) AccountIDsByGroups(context.Context, []string) ([]string, error) {
	return append([]string(nil), g.ids...), nil
}

func TestExecutionRejectsChangedGroupMembershipBeforeAnyRemoteWrite(t *testing.T) {
	_, accounts, manager, tasks, request := automationFixture()
	groups := &changingGroups{ids: []string{"10"}}
	service := accountautomation.New(accounts, manager, nil, tasks, immediateRunner{}, groups)
	preview, err := service.Enqueue(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if tasks.items[preview.ID].Status != "succeeded" {
		t.Fatal("valid preview failed")
	}
	groups.ids = []string{"20"}
	request.DryRun, request.PreviewTaskID = false, preview.ID
	execution, err := service.Enqueue(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	got := tasks.items[execution.ID]
	if got.Status != "failed" || !strings.Contains(got.Message, "账号范围") || accounts.syncCalls != 0 || manager.publishCalls != 0 || manager.priceCalls != 0 {
		t.Fatalf("changed group membership reached remote writes: status=%s message=%s sync=%d publish=%d price=%d", got.Status, got.Message, accounts.syncCalls, manager.publishCalls, manager.priceCalls)
	}
}

func (m *snapshotManager) ChannelsByID(context.Context, string, []string, int, int) (newapimanagement.ChannelPage, error) {
	return newapimanagement.ChannelPage{Items: m.channels, Total: len(m.channels)}, nil
}

func TestScheduleStartupRejectsSubstitutedChannelOrMalformedVersion(t *testing.T) {
	for _, channel := range []newapimanagement.Channel{
		{ID: "12", Version: strings.Repeat("a", 64)},
		{ID: "11", Version: strings.Repeat("z", 64)},
	} {
		t.Run(channel.ID+"-"+channel.Version[:1], func(t *testing.T) {
			_, accounts, manager, tasks, request := automationFixture()
			remote := &snapshotManager{manager: manager, channels: []newapimanagement.Channel{channel}}
			pool := &recoveryRunner{}
			service := accountautomation.New(accounts, remote, nil, tasks, pool, accounts)
			service.UseScheduleRepository(&memorySchedule{})
			defer service.StopSchedules()
			saved := saveTaskGroupSchedule(t, service, request, "固定渠道")
			_, err := service.RunSchedule(t.Context(), saved.ID)
			if err == nil {
				t.Error("invalid remote channel was accepted")
			}
			if err := service.StopSchedulesContext(t.Context()); err != nil {
				t.Fatal(err)
			}
			loaded, err := service.Schedules(t.Context())
			if err != nil || len(loaded) != 1 || !reflect.DeepEqual(loaded[0], saved) {
				t.Errorf("invalid remote response changed schedule: %v err=%v", loaded, err)
			}
			if pool.run != nil || len(tasks.items) != 0 {
				t.Error("invalid channel snapshot started a preview")
			}
		})
	}
}

func TestExecutionRejectsInconsistentChannelSetBeforeAnyPublication(t *testing.T) {
	for _, scenario := range []string{"unexpected", "duplicate"} {
		t.Run(scenario, func(t *testing.T) {
			_, accounts, manager, tasks, request := automationFixture()
			second := newapimanagement.Channel{ID: "12", Version: strings.Repeat("b", 64)}
			request.Channels = append(request.Channels, accountautomation.ChannelTarget{ID: second.ID, Version: second.Version})
			remote := &snapshotManager{manager: manager, channels: []newapimanagement.Channel{manager.channel, second}}
			service := accountautomation.New(accounts, remote, nil, tasks, immediateRunner{}, accounts)
			preview, err := service.Enqueue(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			if tasks.items[preview.ID].Status != "succeeded" {
				t.Fatal("valid preview failed")
			}
			if scenario == "unexpected" {
				remote.channels[1].ID = "13"
			} else {
				remote.channels[1] = remote.channels[0]
			}
			request.DryRun, request.PreviewTaskID = false, preview.ID
			execution, err := service.Enqueue(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			got := tasks.items[execution.ID]
			if got.Status != "failed" || !strings.Contains(got.Message, "渠道") || manager.publishCalls != 0 || manager.priceCalls != 0 {
				t.Fatalf("invalid channel set reached publication: status=%s message=%s publish=%d price=%d", got.Status, got.Message, manager.publishCalls, manager.priceCalls)
			}
		})
	}
}

func TestScheduleStartupPreservesSelectedChannelOrderWhenRemoteOrderDiffers(t *testing.T) {
	_, accounts, manager, tasks, request := automationFixture()
	second := newapimanagement.Channel{ID: "12", Version: strings.Repeat("b", 64)}
	request.Channels = append(request.Channels, accountautomation.ChannelTarget{ID: second.ID, Version: strings.Repeat("a", 64)})
	remote := &snapshotManager{manager: manager, channels: []newapimanagement.Channel{second, manager.channel}}
	service := accountautomation.New(accounts, remote, nil, tasks, &recoveryRunner{}, accounts)
	service.UseScheduleRepository(&memorySchedule{})
	defer service.StopSchedules()
	saved := saveTaskGroupSchedule(t, service, request, "渠道顺序")
	if _, err := service.RunSchedule(t.Context(), saved.ID); err != nil {
		t.Fatal(err)
	}
	if err := service.StopSchedulesContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	loaded, err := service.Schedules(t.Context())
	want := []accountautomation.ChannelTarget{{ID: "11", Version: manager.channel.Version}, {ID: "12", Version: second.Version}}
	if err != nil || len(loaded) != 1 || !reflect.DeepEqual(loaded[0].Request.Channels, want) {
		t.Fatalf("remote response reordered selected targets: %v err=%v", loaded, err)
	}
}
