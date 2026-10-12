package accountautomation_test

import (
	"context"
	"github.com/MIEnchating/sub2api-console/backend/internal/accountautomation"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"testing"
)

type exclusionGroups struct {
	ids      []string
	excluded map[string]bool
}

func (g *exclusionGroups) AccountIDsByGroups(context.Context, []string) ([]string, error) {
	return g.ids, nil
}
func (g *exclusionGroups) AccountUpstreamExcluded(_ context.Context, id string) (bool, error) {
	return g.excluded[id], nil
}

func TestExcludedAccountsDoNotReachPreviewOrExecution(t *testing.T) {
	_, accounts, manager, tasks, request := automationFixture()
	groups := &exclusionGroups{ids: []string{"10", "20"}, excluded: map[string]bool{"20": true}}
	service := accountautomation.New(accounts, manager, nil, tasks, immediateRunner{}, groups)
	preview, err := service.Enqueue(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if accounts.previewCalls != 1 {
		t.Fatalf("excluded account read: %d", accounts.previewCalls)
	}
	request.DryRun, request.PreviewTaskID = false, preview.ID
	execution, err := service.Enqueue(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if tasks.items[execution.ID].Status != "succeeded" || accounts.syncCalls != 1 || manager.publishCalls != 1 {
		t.Fatalf("execution: %+v calls=%d publish=%d", tasks.items[execution.ID], accounts.syncCalls, manager.publishCalls)
	}
}

func TestExclusionAfterPreviewShrinksScopeWithoutFailingOtherAccounts(t *testing.T) {
	_, accounts, manager, tasks, request := automationFixture()
	groups := &exclusionGroups{ids: []string{"10", "20"}, excluded: map[string]bool{}}
	service := accountautomation.New(accounts, manager, nil, tasks, immediateRunner{}, groups)
	preview, err := service.Enqueue(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	groups.excluded["20"] = true
	groups.ids = []string{"10"}
	request.DryRun, request.PreviewTaskID = false, preview.ID
	execution, err := service.Enqueue(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if tasks.items[execution.ID].Status != "succeeded" || accounts.syncCalls != 1 {
		t.Fatalf("excluded scope change: %+v calls=%d", tasks.items[execution.ID], accounts.syncCalls)
	}
}

func TestAllAccountsExcludedDuringExecutionIsNoOp(t *testing.T) {
	service, accounts, manager, tasks, request := automationFixture()
	preview, err := service.Enqueue(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	accounts.err = business.ErrUpstreamExcluded
	request.DryRun, request.PreviewTaskID = false, preview.ID
	execution, err := service.Enqueue(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if tasks.items[execution.ID].Status != "succeeded" || manager.publishCalls != 0 || manager.priceCalls != 0 {
		t.Fatalf("excluded execution failed: %+v", tasks.items[execution.ID])
	}
	if tasks.items[execution.ID].Result["account_count"] != float64(0) {
		t.Fatalf("excluded accounts counted: %+v", tasks.items[execution.ID])
	}
}
