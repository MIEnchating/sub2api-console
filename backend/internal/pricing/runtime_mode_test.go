package pricing

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

type pricingModeRepository struct {
	*fakeRepository
	mode string
}

func (r *pricingModeRepository) Mode(context.Context) (string, error) { return r.mode, nil }

func TestApplyPlanRejectsRemoteWritesInMonitoringMode(t *testing.T) {
	var writes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		group := int64(6)
		if r.Method != http.MethodGet {
			writes.Add(1)
		}
		if writes.Load() > 0 {
			group = 7
		}
		_, _ = io.WriteString(w, `{"success":true,"data":`+accountJSON("41", "0.4", []int64{group})+`}`)
	}))
	defer server.Close()
	repository := &pricingModeRepository{fakeRepository: &fakeRepository{}, mode: runtimepolicy.Monitoring}
	service := New(repository, &fakeTargets{settings: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "test", TimeoutSeconds: 1}}, nil)
	decision := Decision{AccountID: "41", CurrentGroupIDs: []string{"6"}, DesiredGroupIDs: []string{"7"}, Changed: true}
	result, err := service.applyPlan(context.Background(), plan{snapshot: Snapshot{Accounts: 1, Changes: 1, Decisions: []Decision{decision}}}, Config{WriteConcurrency: 1}, "operator")
	if err == nil || writes.Load() != 0 {
		t.Fatalf("monitoring mode changed groups: writes=%d result=%#v error=%v", writes.Load(), result, err)
	}
}
