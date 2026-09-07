package business

import (
	"context"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

func TestManualFuseReadbackExposesReasonWithoutRoutingDecision(t *testing.T) {
	for _, mode := range []string{runtimepolicy.Full, runtimepolicy.Monitoring} {
		t.Run(mode, func(t *testing.T) {
			store := openPolicyStore(t)
			ctx := context.Background()
			if _, err := store.SetMode(ctx, mode); err != nil {
				t.Fatal(err)
			}
			if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','channel-41','{"status":"active"}','now')`); err != nil {
				t.Fatal(err)
			}
			if err := store.CommitAccountControlReadback(ctx, "41", "fuse", "operator", false, testControlOperation("fuse-1")); err != nil {
				t.Fatal(err)
			}
			account, err := store.Account(ctx, "41")
			if err != nil {
				t.Fatal(err)
			}
			if account.Health != "fused" || account.DecisionState == nil || *account.DecisionState != "fused" || account.DecisionReason == nil || *account.DecisionReason != "人工熔断，等待手动解除" {
				t.Fatalf("manual fuse not explained: %+v", account.AccountStatus)
			}
			if account.ApplyPending {
				t.Fatal("confirmed manual fuse must not be pending")
			}
		})
	}
}

func TestManualRecoveryClearsManualFuseReason(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','channel-41','{"status":"active"}','now')`); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"fuse", "recover"} {
		if err := store.CommitAccountControlReadback(ctx, "41", action, "operator", action == "recover", testControlOperation(action)); err != nil {
			t.Fatal(err)
		}
	}
	account, err := store.Account(ctx, "41")
	if err != nil {
		t.Fatal(err)
	}
	if account.Health == "fused" || account.DecisionReason != nil || account.Schedulable == nil || !*account.Schedulable {
		t.Fatalf("manual recovery retained fuse: %+v", account.AccountStatus)
	}
}

func TestManualFuseProjectionOverridesStaleSchedulerTarget(t *testing.T) {
	stopped, previouslySchedulable := false, true
	item := accountProjection{
		AccountStatus: AccountStatus{
			Schedulable: &stopped, RoutingState: stringPointer("healthy"),
			TargetSchedulable: &previouslySchedulable,
		},
		manualFused: true,
		metadataRaw: `{"status":"active"}`,
	}
	applyAccountCalculations(&item,
		[]decisionProjection{{state: "healthy", reason: stringPointer("旧调度决策")}}, nil,
		struct {
			message string
			at      *string
		}{}, routingApplyView{fields: map[string]bool{"schedulable": true}, automatic: true})
	if item.Health != "fused" || item.DecisionReason == nil || *item.DecisionReason != "人工熔断，等待手动解除" || item.ApplyPending {
		t.Fatalf("stale scheduler target overrides confirmed manual fuse: %+v", item.AccountStatus)
	}
}
