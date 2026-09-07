package upstreamconfig

import (
	"context"
	"testing"
)

func TestUpdateQueuesAccountSyncWhenOnlyBaseURLPathCaseChanges(t *testing.T) {
	private, businessStore := openStores(t)
	token, refresh, name := "access", "refresh", "Example"
	service := New(businessStore, private, &passVerifier{})
	_, err := service.Create(context.Background(), Input{
		Host: "api.example", Name: &name, BaseURL: "https://api.example", AccountBaseURL: "https://accounts.example/Team",
		UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1",
		AccessToken: &token, RefreshToken: &refresh, Present: map[string]bool{"access_token": true, "refresh_token": true},
	}, "tester")
	if err != nil {
		t.Fatal(err)
	}
	scheduler := &captureRateSyncScheduler{baseURLTaskID: "case-change-sync"}
	service = New(businessStore, private, &passVerifier{}, scheduler)
	result, err := service.Update(context.Background(), "api.example", Input{
		Name: &name, BaseURL: "https://api.example", AccountBaseURL: "https://accounts.example/team",
		UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1", Present: map[string]bool{},
	}, "tester")
	if err != nil {
		t.Fatal(err)
	}
	if scheduler.baseURLCalls != 1 || result.BaseURLSyncTaskID == nil || *result.BaseURLSyncTaskID != "case-change-sync" {
		t.Fatalf("case-sensitive account URL change did not enqueue synchronization: result=%#v calls=%d", result, scheduler.baseURLCalls)
	}
}
