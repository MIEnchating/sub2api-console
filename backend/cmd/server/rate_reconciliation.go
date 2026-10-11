package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
	"github.com/MIEnchating/sub2api-console/backend/internal/routingwrite"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

type rateModeReader interface {
	Mode(context.Context) (string, error)
}

type ratePriceReconciler interface {
	ReconcileAccountRates(context.Context, []string, string) error
}

type rateRouter interface {
	Calculate(context.Context, routing.Scope, bool) (routing.Result, error)
}

type rateRoutingWriter interface {
	EnforceManualCostWall(context.Context, []string, string) (routingwrite.Result, error)
	Apply(context.Context, map[string]business.AccountRoutingTarget, string) (routingwrite.Result, error)
}

func reconcileConfirmedRates(ctx context.Context, ids []string, actor string, modeReader rateModeReader, prices ratePriceReconciler, router rateRouter, writer rateRoutingWriter) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	mode, err := modeReader.Mode(ctx)
	if err != nil || mode != runtimepolicy.Full {
		return err
	}
	priceErr := prices.ReconcileAccountRates(ctx, ids, actor)
	// A failed migration must still enforce the current cost boundary.
	manualResult, manualErr := writer.EnforceManualCostWall(ctx, ids, actor)
	if manualResult.Failed > 0 {
		manualErr = errors.Join(manualErr, fmt.Errorf("手动账号成本墙写回失败 %d 项", manualResult.Failed))
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(priceErr, manualErr, err)
	}
	// Group weights and capacity depend on peers, so calculate the full routing round once.
	calculated, err := router.Calculate(ctx, routing.Scope{}, true)
	if err != nil {
		return errors.Join(priceErr, manualErr, err)
	}
	confirmed := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		confirmed[id] = struct{}{}
	}
	targets := make(map[string]business.AccountRoutingTarget, len(ids))
	for id, target := range calculated.AccountTargets {
		if _, ok := confirmed[id]; ok {
			targets[id] = target
		}
	}
	if len(targets) == 0 {
		return errors.Join(priceErr, manualErr)
	}
	applied, routingErr := writer.Apply(ctx, targets, actor)
	if applied.Failed > 0 {
		routingErr = errors.Join(routingErr, fmt.Errorf("账号成本复核写回失败 %d 项", applied.Failed))
	}
	return errors.Join(priceErr, manualErr, routingErr)
}
