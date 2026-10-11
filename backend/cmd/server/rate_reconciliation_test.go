package main

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
	"github.com/MIEnchating/sub2api-console/backend/internal/routingwrite"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

type rateModeStub struct{ mode string }

func (stub rateModeStub) Mode(context.Context) (string, error) { return stub.mode, nil }

type ratePriceStub struct {
	ids   []string
	calls int
	err   error
}

func (stub *ratePriceStub) ReconcileAccountRates(_ context.Context, ids []string, _ string) error {
	stub.calls++
	stub.ids = append([]string(nil), ids...)
	return stub.err
}

type rateRouterStub struct {
	accounts map[string]business.AccountRoutingTarget
	calls    int
	err      error
	scope    routing.Scope
}

func (stub *rateRouterStub) Calculate(_ context.Context, scope routing.Scope, persist bool) (routing.Result, error) {
	stub.calls++
	stub.scope = scope
	if !persist {
		return routing.Result{}, errors.New("routing decisions were not persisted")
	}
	if stub.err != nil {
		return routing.Result{}, stub.err
	}
	return routing.Result{AccountTargets: stub.accounts}, nil
}

type rateWriterStub struct {
	manualCalls int
	applyCalls  int
	manualIDs   []string
	applied     map[string]business.AccountRoutingTarget
	applyResult routingwrite.Result
}

func (stub *rateWriterStub) EnforceManualCostWall(_ context.Context, ids []string, _ string) (routingwrite.Result, error) {
	stub.manualCalls++
	stub.manualIDs = append([]string(nil), ids...)
	return routingwrite.Result{}, nil
}

func (stub *rateWriterStub) Apply(_ context.Context, targets map[string]business.AccountRoutingTarget, _ string) (routingwrite.Result, error) {
	stub.applyCalls++
	stub.applied = targets
	return stub.applyResult, nil
}

func TestRateReconciliationForManyUnchangedAccountsCalculatesOnceAndAppliesOnlyConfirmedTargets(t *testing.T) {
	ids := make([]string, 450)
	targets := make(map[string]business.AccountRoutingTarget, len(ids)+1)
	for index := range ids {
		id := strconv.Itoa(index + 1)
		ids[index] = id
		targets[id] = business.AccountRoutingTarget{AccountID: id}
	}
	targets["unconfirmed"] = business.AccountRoutingTarget{AccountID: "unconfirmed"}
	prices := &ratePriceStub{}
	router := &rateRouterStub{accounts: targets}
	writer := &rateWriterStub{}

	err := reconcileConfirmedRates(t.Context(), ids, "test", rateModeStub{runtimepolicy.Full}, prices, router, writer)

	if err != nil {
		t.Fatal(err)
	}
	if router.calls != 1 || writer.applyCalls != 1 {
		t.Fatalf("routing calculations=%d apply batches=%d, want one each", router.calls, writer.applyCalls)
	}
	if router.scope.AccountID != nil || router.scope.GroupName != nil {
		t.Fatalf("routing scope=%+v, want full inventory for shared capacity", router.scope)
	}
	if len(writer.applied) != len(ids) {
		t.Fatalf("applied %d targets, want %d confirmed accounts", len(writer.applied), len(ids))
	}
	if _, found := writer.applied["unconfirmed"]; found {
		t.Fatal("unconfirmed account was sent for remote write")
	}
	if prices.calls != 1 || len(prices.ids) != len(ids) || writer.manualCalls != 1 || len(writer.manualIDs) != len(ids) {
		t.Fatal("price and manual cost-wall reconciliation did not receive the confirmed range once")
	}
}

func TestRateReconciliationWhenSingleConfirmedWriteFailsReportsFailureCount(t *testing.T) {
	router := &rateRouterStub{accounts: map[string]business.AccountRoutingTarget{
		"1": {AccountID: "1"},
	}}
	writer := &rateWriterStub{applyResult: routingwrite.Result{Failed: 1}}

	err := reconcileConfirmedRates(t.Context(), []string{"1"}, "test", rateModeStub{runtimepolicy.Full}, &ratePriceStub{}, router, writer)

	if err == nil || err.Error() != "账号成本复核写回失败 1 项" || router.calls != 1 || writer.applyCalls != 1 {
		t.Fatalf("error=%v calculations=%d apply batches=%d", err, router.calls, writer.applyCalls)
	}
}

func TestRateReconciliationWhenConfirmedAccountHasNoTargetSkipsRemoteWrite(t *testing.T) {
	router := &rateRouterStub{accounts: map[string]business.AccountRoutingTarget{
		"other": {AccountID: "other"},
	}}
	writer := &rateWriterStub{}

	err := reconcileConfirmedRates(t.Context(), []string{"1"}, "test", rateModeStub{runtimepolicy.Full}, &ratePriceStub{}, router, writer)

	if err != nil || router.calls != 1 || writer.applyCalls != 0 {
		t.Fatalf("error=%v calculations=%d apply batches=%d", err, router.calls, writer.applyCalls)
	}
}

func TestRateReconciliationWhenRoutingCalculationFailsReturnsOneErrorWithoutApplying(t *testing.T) {
	calculationErr := errors.New("routing database unavailable")
	prices := &ratePriceStub{err: errors.New("price migration unavailable")}
	router := &rateRouterStub{err: calculationErr}
	writer := &rateWriterStub{}

	err := reconcileConfirmedRates(t.Context(), []string{"1", "2"}, "test", rateModeStub{runtimepolicy.Full}, prices, router, writer)

	if !errors.Is(err, calculationErr) || router.calls != 1 || writer.applyCalls != 0 {
		t.Fatalf("error=%v calculations=%d apply batches=%d", err, router.calls, writer.applyCalls)
	}
	if prices.calls != 1 || writer.manualCalls != 1 {
		t.Fatal("price failure prevented manual cost-wall enforcement")
	}
}

func TestRateReconciliationWhenCanceledDoesNotStartAnotherCalculation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	router := &rateRouterStub{}
	writer := &rateWriterStub{}

	err := reconcileConfirmedRates(ctx, []string{"1", "2"}, "test", rateModeStub{runtimepolicy.Full}, &ratePriceStub{}, router, writer)

	if !errors.Is(err, context.Canceled) || router.calls != 0 || writer.applyCalls != 0 {
		t.Fatalf("error=%v calculations=%d apply batches=%d", err, router.calls, writer.applyCalls)
	}
}
