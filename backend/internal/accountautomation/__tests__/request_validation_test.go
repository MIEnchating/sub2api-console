package accountautomation_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountautomation"
	"github.com/MIEnchating/sub2api-console/backend/internal/newapimanagement"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestAutomationEntryPointsRejectInvalidScopeBeforePersistingOrDispatching(t *testing.T) {
	cases := []struct {
		name, reason string
		change       func(*accountautomation.Request)
	}{
		{"missing groups", "分组", func(r *accountautomation.Request) { r.GroupIDs = nil }},
		{"duplicate groups", "分组", func(r *accountautomation.Request) { r.GroupIDs = []string{"7", "7"} }},
		{"invalid group ID", "分组", func(r *accountautomation.Request) { r.GroupIDs = []string{"group-name"} }},
		{"too many groups", "50", func(r *accountautomation.Request) {
			r.GroupIDs = nil
			for i := 1; i <= 51; i++ {
				r.GroupIDs = append(r.GroupIDs, fmt.Sprint(i))
			}
		}},
		{"missing platform", "平台", func(r *accountautomation.Request) { r.PlatformID = " " }},
		{"duplicate channels", "渠道", func(r *accountautomation.Request) { r.Channels = append(r.Channels, r.Channels[0]) }},
		{"invalid channel ID", "渠道", func(r *accountautomation.Request) { r.Channels[0].ID = "channel-name" }},
		{"invalid channel version", "版本", func(r *accountautomation.Request) { r.Channels[0].Version = "old" }},
		{"nonhex channel version", "版本", func(r *accountautomation.Request) { r.Channels[0].Version = strings.Repeat("z", 64) }},
		{"too many channels", "50", func(r *accountautomation.Request) {
			r.Channels = nil
			for i := 1; i <= 51; i++ {
				r.Channels = append(r.Channels, accountautomation.ChannelTarget{ID: fmt.Sprint(i), Version: strings.Repeat("a", 64)})
			}
		}},
		{"invalid pricing source", "价格来源", func(r *accountautomation.Request) { r.PricingSource = "unsupported" }},
		{"invalid selection", "模型", func(r *accountautomation.Request) { r.SelectedModels = []string{"model-a", "MODEL-A"} }},
	}
	for _, tc := range cases {
		for _, entry := range []string{"create", "schedule", "recover"} {
			t.Run(tc.name+"/"+entry, func(t *testing.T) {
				_, accounts, manager, _, request := automationFixture()
				store, openErr := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
				if openErr != nil {
					t.Fatal(openErr)
				}
				defer store.Close()
				pool := &recoveryRunner{}
				service := accountautomation.New(accounts, manager, nil, store, pool, accounts)
				repo := &memorySchedule{}
				service.UseScheduleRepository(repo)
				tc.change(&request)
				var err error
				switch entry {
				case "create":
					_, err = service.Enqueue(t.Context(), request)
				case "schedule":
					_, err = service.SaveSchedule(t.Context(), accountautomation.Schedule{Name: "校验范围", Request: request, IntervalMinutes: 60})
				case "recover":
					task := taskstore.Task{ID: strings.Repeat("a", 32), Skill: accountautomation.Skill, Operation: accountautomation.Operation, Status: "queued"}
					if err := taskstore.WithRecovery(&task, struct {
						Request accountautomation.Request `json:"request"`
					}{request}); err != nil {
						t.Fatal(err)
					}
					err = service.ResumeTask(t.Context(), task)
				}
				if err == nil || !strings.Contains(err.Error(), tc.reason) {
					t.Errorf("%s accepted invalid scope or lost reason: %v", entry, err)
				}
				stored, listErr := store.List(t.Context(), nil)
				if listErr != nil {
					t.Fatal(listErr)
				}
				if pool.run != nil || len(stored) != 0 || repo.raw != "" {
					t.Error("invalid scope was persisted or dispatched")
				}
			})
		}
	}
}

type recoveryPriceManager struct {
	*manager
	official bool
}

func (m *recoveryPriceManager) ModelPriceCatalog(ctx context.Context, platform string, official bool) (newapimanagement.ModelPriceCatalog, error) {
	m.official = official
	return m.manager.ModelPriceCatalog(ctx, platform, official)
}

func TestRecoveryMissingPricingSourceUsesOfficialLookup(t *testing.T) {
	_, accounts, manager, _, request := automationFixture()
	store, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	remote := &recoveryPriceManager{manager: manager}
	pool := &recoveryRunner{}
	service := accountautomation.New(accounts, remote, nil, store, pool, accounts)
	task, err := service.Enqueue(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.PricingSource = ""
	if err := taskstore.WithRecovery(&task, struct {
		Request accountautomation.Request `json:"request"`
	}{request}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	if err := service.ResumeTask(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	pool.run(t.Context())
	got, err := store.Get(t.Context(), task.ID)
	if err != nil || got.Status != "succeeded" || !remote.official {
		t.Fatalf("legacy recovery did not use the official default: status=%s official=%v err=%v", got.Status, remote.official, err)
	}
}

func TestAutomationMaximumValidScopeIsAcceptedByEveryEntryPoint(t *testing.T) {
	_, accounts, manager, _, request := automationFixture()
	store, openErr := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
	if openErr != nil {
		t.Fatal(openErr)
	}
	defer store.Close()
	pool := &recoveryRunner{}
	service := accountautomation.New(accounts, manager, nil, store, pool, accounts)
	service.UseScheduleRepository(&memorySchedule{})
	request.GroupIDs, request.Channels = nil, nil
	for i := 1; i <= 50; i++ {
		request.GroupIDs = append(request.GroupIDs, fmt.Sprint(i))
		request.Channels = append(request.Channels, accountautomation.ChannelTarget{ID: fmt.Sprint(i), Version: strings.Repeat("a", 64)})
	}
	task, err := service.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveSchedule(t.Context(), accountautomation.Schedule{Name: "完整范围", Request: request, IntervalMinutes: 60}); err != nil {
		t.Fatal(err)
	}
	if err := service.ResumeTask(t.Context(), task); err != nil {
		t.Fatal(err)
	}
}
