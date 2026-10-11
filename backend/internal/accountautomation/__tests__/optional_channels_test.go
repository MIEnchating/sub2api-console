package accountautomation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountautomation"
)

func TestNoChannelsDoesNotHideAccountReadFailure(t *testing.T) {
	service, accounts, management, store, request := automationFixture()
	request.Channels = nil
	accounts.err = errors.New("账号模型目录读取失败")
	preview, err := service.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if store.items[preview.ID].Status != "failed" || management.channelReads != 0 || management.catalogReads != 0 {
		t.Fatal("account failure must fail the preview without accessing channel management")
	}
}

func TestNoChannelsPreviewAndExecuteOnlyAccountModels(t *testing.T) {
	service, accounts, management, store, request := automationFixture()
	request.Channels = nil
	// The backend must also suppress pricing for requests from older clients.
	request.PriceEnabled = true
	preview, err := service.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.items[preview.ID]; got.Status != "succeeded" || got.Result["pricing_plan"] != nil {
		t.Fatalf("account-only preview failed: %#v", got)
	}
	request.DryRun, request.PreviewTaskID = false, preview.ID
	execution, err := service.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.items[execution.ID]; got.Status != "succeeded" || accounts.syncCalls != 1 {
		t.Fatalf("account-only execution failed: %#v", got)
	}
	if management.channelReads != 0 || management.catalogReads != 0 || management.publishCalls != 0 || management.priceCalls != 0 {
		t.Fatalf("account-only task accessed channel management: %#v", management)
	}
}

func TestNoChannelsSchedulePersistsAndRunsWithPricingDisabled(t *testing.T) {
	service, accounts, management, store, request := automationFixture()
	request.Channels = nil
	store.completed = make(chan struct{}, 1)
	service.UseScheduleRepository(&memorySchedule{})
	saved, err := service.SaveSchedule(context.Background(), accountautomation.Schedule{
		Name: "只同步账号模型", Request: request, Enabled: true, IntervalMinutes: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := service.Schedules(context.Background())
	if err != nil || len(loaded) != 1 {
		t.Fatalf("load schedule: %#v, %v", loaded, err)
	}
	if loaded[0].Request.Channels == nil || len(loaded[0].Request.Channels) != 0 || loaded[0].Request.PriceEnabled {
		t.Fatalf("empty channel selection was not preserved: %#v", loaded[0])
	}
	if _, err := service.RunSchedule(context.Background(), saved.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-store.completed:
	case <-time.After(5 * time.Second):
		t.Fatal("account-only schedule did not complete")
	}
	if accounts.syncCalls != 1 || management.channelReads != 0 || management.catalogReads != 0 || management.priceCalls != 0 {
		t.Fatalf("schedule did not restrict execution to accounts: account=%#v manager=%#v", accounts, management)
	}
}

func TestSelectedChannelStillSynchronizesPrices(t *testing.T) {
	service, _, management, store, request := automationFixture()
	preview, err := service.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.DryRun, request.PreviewTaskID = false, preview.ID
	execution, err := service.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if store.items[execution.ID].Status != "succeeded" || management.publishCalls != 1 || management.priceCalls != 1 {
		t.Fatal("selected channel must retain model publication and pricing")
	}
}
