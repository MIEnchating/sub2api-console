package accountops

import (
	"context"
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

// StopExcludedAccount is the explicit one-time stop selected when excluding an
// upstream. It changes only scheduling, including manually controlled accounts.
func (s *Service) StopExcludedAccount(ctx context.Context, id, actor string) (map[string]any, error) {
	if !stableID(id) {
		return nil, errors.New("账号必须使用稳定 ID")
	}
	guarded, release, err := targetguard.Acquire(ctx, s.repository, mutationguard.Account(id))
	if err != nil {
		return nil, err
	}
	defer release()
	ctx = guarded
	if err := business.CheckAccountUpstreamManaged(ctx, s.repository, id); !errors.Is(err, business.ErrUpstreamExcluded) {
		if err != nil {
			return nil, err
		}
		return nil, errors.New("账号所属上游尚未排除，已停止操作")
	}
	mode, local, err := s.controlAccount(ctx, id)
	if err != nil {
		return nil, err
	}
	if mode != runtimepolicy.Full {
		return nil, errors.New("关闭调度需要完全模式")
	}
	ctx, err = targetguard.Bind(ctx, s.targets)
	if err != nil {
		return nil, err
	}
	client, err := s.adminClient(ctx)
	if err != nil {
		return nil, err
	}
	before, err := client.Account(ctx, id)
	if err != nil {
		return nil, err
	}
	enabled, err := accountSchedulable(before)
	if err != nil {
		return nil, err
	}
	opID, err := operationID("upstream-exclude-stop")
	if err != nil {
		return nil, err
	}
	operation := controlOperation(opID, actor, id, local.Name, "upstream-exclude-stop", map[string]any{"schedulable": enabled}, map[string]any{"schedulable": false}, false, false)
	if enabled {
		if err = writeAccountSchedulable(ctx, client, id, false); err != nil {
			s.recordFailure(ctx, operation, "remote-write", err, false)
			return nil, err
		}
		operation.RemoteConfirmed, operation.Writeback = true, true
	}
	after, err := client.Account(ctx, id)
	if err == nil {
		var actual bool
		actual, err = accountSchedulable(after)
		if err == nil && actual {
			err = errors.New("关闭调度后回读仍为开启，请重试")
		}
	}
	if err != nil {
		s.recordFailure(ctx, operation, "readback", err, enabled)
		return nil, err
	}
	operation.ReadbackConfirmed = true
	disabled := false
	if err = s.repository.CommitAccountFieldsReadback(ctx, id, nil, nil, nil, nil, &disabled, nil, nil, nil, false, nil, operation); err != nil {
		return nil, err
	}
	return map[string]any{"account_id": id, "schedulable": false, "readback_confirmed": true}, nil
}
