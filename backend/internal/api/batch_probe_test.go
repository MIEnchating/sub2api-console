package api

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestBatchProbeRouteAcceptsExplicitManualIDs(t *testing.T) {
	calls := []probeCall{}
	router, _ := testRouterWithDependencies(t, config.Config{AdminToken: "test-token"}, fakeBusiness{mode: "完全模式"}, Dependencies{
		ProbeTasks: fakeProbeTasks{task: taskstore.Task{ID: "batch-1"}, calls: &calls},
	})
	response := authenticatedRequest(t, router, http.MethodPost, "/api/inspection/probe", map[string]any{"account_ids": []string{"41", "42"}})
	if response.Code != http.StatusOK || len(calls) != 1 {
		t.Fatalf("status=%d response=%s calls=%v", response.Code, response.Body, calls)
	}
	if !reflect.DeepEqual(calls[0].request.SelectedAccountIDs, []string{"41", "42"}) || calls[0].request.Automatic || calls[0].request.AccountIDs != nil {
		t.Fatalf("incorrect batch selection: %+v", calls[0].request)
	}
}

func TestBatchProbeRouteRejectsEmptyInvalidAndMixedSelections(t *testing.T) {
	for _, body := range []map[string]any{
		{"account_ids": nil}, {"account_ids": []string{}}, {"account_ids": "41"},
		{"account_ids": []string{"041"}}, {"account_ids": []string{"41", "41"}},
		{"account_ids": []int{41}}, {"account_ids": make([]string, 101)},
		{"account_ids": []string{"41"}, "account_id": "42"},
		{"account_ids": []string{"41"}, "group_name": "codex"},
		{"account_ids": []string{"41"}, "platform": "openai", "model": "gpt"},
	} {
		calls := []probeCall{}
		router, _ := testRouterWithDependencies(t, config.Config{AdminToken: "test-token"}, fakeBusiness{}, Dependencies{
			ProbeTasks: fakeProbeTasks{calls: &calls},
		})
		response := authenticatedRequest(t, router, http.MethodPost, "/api/inspection/probe", body)
		if response.Code != http.StatusUnprocessableEntity || len(calls) != 0 {
			t.Fatalf("body=%v status=%d calls=%v", body, response.Code, calls)
		}
	}
}
