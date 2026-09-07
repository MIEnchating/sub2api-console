package authrecovery

import (
	"context"
	"errors"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type unavailableRecoveryProjection struct {
	recoveryRepository
	failure error
}

func (repository *unavailableRecoveryProjection) PersistAuthRecoveryOutcomes(context.Context, []business.AuthRecoveryOutcome, string) (business.AuthRecoverySummary, error) {
	return business.AuthRecoverySummary{}, repository.failure
}

func TestBatchRecoveryPreservesRotatedCredentialsWhenProjectionPersistenceFails(t *testing.T) {
	oldToken, refresh, rotated, rotatedRefresh := "old", "single-use-refresh", "rotated", "next-refresh"
	original := configstore.AuthRecord{Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", AccessToken: &oldToken, RefreshToken: &refresh}
	private := &recoveryPrivate{record: &original}
	failure := errors.New("projection storage unavailable")
	repository := &unavailableRecoveryProjection{failure: failure}
	auth := &recoveryAuthenticator{refresh: func(_ context.Context, record configstore.AuthRecord) (configstore.AuthRecord, error) {
		record.AccessToken, record.RefreshToken = &rotated, &rotatedRefresh
		return record, nil
	}}
	service := New(repository, private, auth, &recoveryConfigurator{}, &recoveryBalance{}, nil)
	result, err := service.recoverRecords(context.Background(), []configstore.AuthRecord{original}, "operator")
	if !errors.Is(err, failure) || len(result.Outcomes) != 1 || !result.Outcomes[0].Success {
		t.Fatalf("projection failure lost completed recovery result: result=%#v err=%v", result, err)
	}
	stored, err := private.AuthRecord(context.Background(), original.Host)
	if err != nil {
		t.Fatal(err)
	}
	if stored.AccessToken == nil || *stored.AccessToken != rotated || stored.RefreshToken == nil || *stored.RefreshToken != rotatedRefresh {
		t.Fatal("projection failure restored invalid old credentials over successfully rotated credentials")
	}
}
