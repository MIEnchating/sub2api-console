package authrecovery

import (
	"context"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestRecoveryDoesNotOverwriteCredentialsChangedDuringRemoteVerification(t *testing.T) {
	oldToken, refresh, rotated, replacement := "old", "refresh", "rotated", "replacement"
	original := configstore.AuthRecord{Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", AccessToken: &oldToken, RefreshToken: &refresh}
	private := &recoveryPrivate{record: &original}
	auth := &recoveryAuthenticator{refresh: func(_ context.Context, candidate configstore.AuthRecord) (configstore.AuthRecord, error) {
		changed := cloneAuthRecord(original)
		changed.BaseURL = "https://replacement.example"
		changed.AccessToken = &replacement
		if err := private.SaveAuthRecord(context.Background(), changed, allAuthFields()); err != nil {
			t.Fatal(err)
		}
		candidate.AccessToken = &rotated
		return candidate, nil
	}}
	service := New(&recoveryRepository{}, private, auth, &recoveryConfigurator{}, &recoveryBalance{}, nil)
	outcome := service.recover(context.Background(), original, "", false)
	stored, err := private.AuthRecord(context.Background(), original.Host)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Success || stored.BaseURL != "https://replacement.example" || stored.AccessToken == nil || *stored.AccessToken != replacement {
		t.Fatalf("stale recovery overwrote newer configuration: outcome=%#v stored=%#v", outcome, stored)
	}
}

func TestRecoveryDoesNotRestoreAuthRecordRemovedDuringVerificationWhileUpstreamRemains(t *testing.T) {
	token, refresh := "old", "refresh"
	original := configstore.AuthRecord{Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", AccessToken: &token, RefreshToken: &refresh}
	private := &recoveryPrivate{record: &original}
	repository := &recoveryRepository{seeds: map[string]business.UpstreamAuthSeed{"api.example": {Host: "api.example", BaseURL: original.BaseURL, UpstreamType: original.UpstreamType}}}
	auth := &recoveryAuthenticator{refresh: func(_ context.Context, record configstore.AuthRecord) (configstore.AuthRecord, error) {
		private.mu.Lock()
		private.record = nil
		private.mu.Unlock()
		return record, nil
	}}
	service := New(repository, private, auth, &recoveryConfigurator{}, &recoveryBalance{}, nil)
	outcome := service.recover(context.Background(), original, "", false)
	stored, err := private.AuthRecord(context.Background(), original.Host)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Success || stored != nil {
		t.Fatal("recovery restored deliberately removed authentication while public upstream remained")
	}
}
