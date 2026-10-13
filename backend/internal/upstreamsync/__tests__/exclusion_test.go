package upstreamsync_test

import (
	"context"
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
	"testing"
)

type unavailableExclusionRepository struct{ *business.Store }

func (r unavailableExclusionRepository) UpstreamExcluded(context.Context, string) (bool, error) {
	return false, errors.New("exclusion storage unavailable")
}

func TestExclusionReadFailureIsNotReportedAsSkipped(t *testing.T) {
	f := newBalanceFixture(t, balanceReader{read: func(context.Context, string) (business.UpstreamBalanceObservation, error) {
		t.Error("unexpected upstream request")
		return business.UpstreamBalanceObservation{}, nil
	}}, "fixture.example")
	service := upstreamsync.New(unavailableExclusionRepository{f.store}, privateRecords{}, nil, nil, nil)
	result, err := service.SyncHost(t.Context(), "fixture.example", upstreamsync.Scope{Balance: true}, "test")
	if err != nil || result.Status != "failed" {
		t.Fatalf("lookup failure: %+v %v", result, err)
	}
}

func TestExcludedUpstreamSkipsRemoteSyncAndClosesAlerts(t *testing.T) {
	fixture := newBalanceFixture(t, balanceReader{read: func(context.Context, string) (business.UpstreamBalanceObservation, error) {
		t.Error("excluded upstream contacted")
		return business.UpstreamBalanceObservation{}, nil
	}}, "excluded.example")
	preview, err := fixture.store.UpstreamExclusionPreview(t.Context(), "excluded.example")
	if err != nil {
		t.Fatal(err)
	}
	if err = fixture.store.SetUpstreamExcluded(t.Context(), preview.UpstreamID, true, "test"); err != nil {
		t.Fatal(err)
	}
	result, err := fixture.syncer.SyncAllNow(t.Context(), upstreamsync.Scope{Balance: true}, "test")
	if err != nil || result.Total != 0 || result.Skipped != 0 || result.Failed != 0 {
		t.Fatalf("sync=%+v err=%v", result, err)
	}
	if _, err = fixture.alerts.Evaluate(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(fixture.sender.messages) != 0 {
		t.Fatalf("excluded upstream sent alerts: %v", fixture.sender.messages)
	}
	var status string
	if err = fixture.db.QueryRow(`SELECT status FROM alert_incidents WHERE object_id='excluded.example'`).Scan(&status); err != nil || status != "closed" {
		t.Fatalf("alert status=%s %v", status, err)
	}
}
