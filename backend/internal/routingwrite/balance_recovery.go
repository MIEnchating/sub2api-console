package routingwrite

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

type balanceRecoveryRepository interface {
	BalanceRecoveryAccounts(context.Context, string) ([]business.BalanceRecoveryAccount, error)
	SaveBalanceRecoveryState(context.Context, business.BalanceRecoveryState) error
	DeleteBalanceRecoveryState(context.Context, string) error
	CommitBalanceRecovery(context.Context, business.BalanceRecoveryState, business.AccountOperation) error
	AccountMutationProtection(context.Context, string) (business.AccountMutationProtection, error)
}

// ReconcileBalanceRecovery runs after a fresh balance observation. The durable
// intent survives process restarts; every execution rechecks binding and target.
func (s *Service) ReconcileBalanceRecovery(ctx context.Context, host, actor string) (bool, error) {
	repository, ok := s.repository.(balanceRecoveryRepository)
	if !ok {
		return false, errors.New("余额恢复记录存储不可用")
	}
	mode, err := s.repository.Mode(ctx)
	if err != nil {
		return false, err
	}
	capabilities, _ := runtimepolicy.For(mode)
	if !capabilities.AutomaticRemoteApply {
		return false, nil
	}
	items, err := repository.BalanceRecoveryAccounts(ctx, host)
	if err != nil {
		return false, err
	}
	var failures []error
	remoteWrite := false
	for _, item := range items {
		if !item.BalanceKnown {
			continue
		}
		if err := s.reconcileBalanceAccount(ctx, repository, host, actor, item, &remoteWrite); err != nil {
			failures = append(failures, fmt.Errorf("账号 %s 余额恢复失败：%w", item.AccountID, err))
		}
	}
	return remoteWrite, errors.Join(failures...)
}

func (s *Service) reconcileBalanceAccount(ctx context.Context, repository balanceRecoveryRepository, host, actor string, item business.BalanceRecoveryAccount, remoteWrite *bool) error {
	document, err := s.repository.ControlPolicy(ctx)
	if err != nil {
		return err
	}
	policy, err := parseWritePolicy(document)
	if err != nil {
		return err
	}
	enabled := true
	targets := map[string]business.AccountRoutingTarget{item.AccountID: {AccountID: item.AccountID, Schedulable: &enabled}}
	plan, capacityResources, err := prepareUpstreamCapacity(ctx, s.repository, targets, policy, document)
	if err != nil {
		return err
	}
	resources := append(capacityResources, mutationguard.Account(item.AccountID), mutationguard.Upstream(host))
	ctx, err = targetguard.Capture(ctx, s.targets)
	if err != nil {
		return err
	}
	ctx, release, err := targetguard.Acquire(ctx, s.repository, resources...)
	if err != nil {
		return err
	}
	defer release()
	ctx, err = targetguard.Bind(ctx, s.targets)
	if err != nil {
		return err
	}
	fingerprint, err := s.managementTargetFingerprint(ctx)
	if err != nil {
		return err
	}
	// Re-read after waiting for the lease; never use a stale balance or binding.
	latest, err := repository.BalanceRecoveryAccounts(ctx, host)
	if err != nil {
		return err
	}
	found := false
	for _, candidate := range latest {
		if candidate.AccountID == item.AccountID && candidate.BindingFingerprint == item.BindingFingerprint {
			item, found = candidate, true
			break
		}
	}
	if !found || !item.BalanceKnown {
		return nil
	}
	mode, err := s.recheckWriteAuthorization(ctx, document)
	if err != nil {
		return err
	}
	capabilities, _ := runtimepolicy.For(mode)
	if !capabilities.AutomaticRemoteApply || !policy.autoApply["schedulable"] {
		return nil
	}
	protection, err := repository.AccountMutationProtection(ctx, item.AccountID)
	if err != nil {
		return err
	}
	if protection.Protected() {
		return repository.DeleteBalanceRecoveryState(ctx, item.AccountID)
	}
	if item.State != nil && (item.State.TargetFingerprint != fingerprint || item.State.BindingFingerprint != item.BindingFingerprint) {
		// A newly bound account starts with no inherited scheduling intent.
		if err := repository.DeleteBalanceRecoveryState(ctx, item.AccountID); err != nil {
			return err
		}
		item.State = nil
	}
	admin, err := s.adminClient(ctx)
	if err != nil {
		return err
	}
	payload, err := admin.Account(ctx, item.AccountID)
	if err != nil {
		return err
	}
	if fmt.Sprint(payload["id"]) != item.AccountID {
		return errors.New("账号读回 ID 不匹配")
	}
	current, err := remoteValues(payload)
	if err != nil {
		return err
	}
	if current.schedulable == nil || current.status == nil {
		return errors.New("账号状态不完整，请同步账号后重试")
	}
	remoteFingerprint, err := balanceRemoteFingerprint(payload)
	if err != nil {
		return err
	}
	if item.State != nil && item.State.RemoteFingerprint != remoteFingerprint {
		return repository.DeleteBalanceRecoveryState(ctx, item.AccountID)
	}
	state := item.State
	if state == nil {
		if *current.status != "active" && !balanceForbidden(payload) {
			return nil
		}
		state = &business.BalanceRecoveryState{AccountID: item.AccountID, BindingFingerprint: item.BindingFingerprint, TargetFingerprint: fingerprint, RemoteFingerprint: remoteFingerprint, Schedulable: *current.schedulable, Depleted: item.Depleted}
		return repository.SaveBalanceRecoveryState(ctx, *state)
	}
	if item.Depleted {
		state.Depleted = true
		// A live, active account is authoritative about its current switch.
		if *current.status == "active" {
			state.Schedulable = *current.schedulable
		}
		return repository.SaveBalanceRecoveryState(ctx, *state)
	}
	if !state.Depleted {
		if *current.status == "active" {
			state.Schedulable = *current.schedulable
			return repository.SaveBalanceRecoveryState(ctx, *state)
		}
		return nil
	}
	if !balanceForbidden(payload) && !(state.Started && *current.status == "active") {
		// A different error or an external recovery ends this depletion episode.
		return repository.DeleteBalanceRecoveryState(ctx, item.AccountID)
	}
	// Never reactivate an account stopped by another controller.
	if state.Schedulable && (item.RoutingState == business.AccountStateFused || item.RoutingState == business.AccountStateCostBlocked || item.RoutingState == business.AccountStateConcurrencyLimited || item.RoutingState == business.AccountStatePaused) {
		return nil
	}
	capacity, err := plan.recheck(ctx, admin, document)
	if err != nil {
		return err
	}
	if err := capacity.reserve(item.AccountID, current, map[string]any{"schedulable": state.Schedulable, "status": "active"}); err != nil {
		var wait *upstreamCapacityWait
		if errors.As(err, &wait) {
			return nil
		}
		return err
	}
	ctx = adminclient.WithMutationAuthorization(ctx, func(checkCtx context.Context) error {
		mode, err := s.recheckWriteAuthorization(checkCtx, document)
		if err != nil {
			return err
		}
		caps, _ := runtimepolicy.For(mode)
		if !caps.AutomaticRemoteApply {
			return errors.New("运行模式已变化，已停止余额恢复")
		}
		protection, err := repository.AccountMutationProtection(checkCtx, item.AccountID)
		if err != nil {
			return err
		}
		if protection.Protected() {
			return errors.New("账号已受人工或排除保护，已停止余额恢复")
		}
		return nil
	})
	return s.restoreBalanceAccount(ctx, repository, admin, *state, payload, current, actor, remoteWrite)
}

func balanceForbidden(payload map[string]any) bool {
	status, _ := payload["status"].(string)
	if status != "error" {
		return false
	}
	message, _ := payload["error_message"].(string)
	message = strings.ToLower(message)
	if message == "access forbidden (403)" || message == "access forbidden (403):" {
		return true
	}
	if !strings.Contains(message, "403") {
		return false
	}
	for _, clue := range []string{"insufficient balance", "insufficient quota", "quota exceeded", "balance exhausted", "balance is not enough", "credit balance", "余额不足", "余额耗尽", "额度不足", "额度耗尽"} {
		if strings.Contains(message, clue) {
			return true
		}
	}
	return false
}

// Keep a digest only: management DTOs can contain private generation keys.
func balanceRemoteFingerprint(payload map[string]any) (string, error) {
	raw, err := json.Marshal([]any{payload["type"], payload["platform"], payload["base_url"], payload["credentials"]})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}
