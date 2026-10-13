package accountworkbench

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func (s *Service) MoveAccountGroups(ctx context.Context, id string, input MoveGroupsInput, actor string) (taskstore.Task, error) {
	if s.tasks == nil || s.runner == nil {
		return taskstore.Task{}, errors.New("工作台任务执行器尚未就绪")
	}
	preview, err := s.AccountGroups(ctx, id)
	if err != nil {
		return taskstore.Task{}, err
	}
	if err = validateMoveGroups(preview, input); err != nil {
		return taskstore.Task{}, err
	}
	target, err := targetguard.Settings(ctx, s.private)
	if err != nil {
		return taskstore.Task{}, err
	}
	if targetguard.Fingerprint(target) != input.TargetVersion {
		return taskstore.Task{}, targetguard.ErrChanged
	}
	input.GroupIDs, input.ExpectedGroupIDs = slices.Clone(input.GroupIDs), slices.Clone(input.ExpectedGroupIDs)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{ID: newID(), Skill: Skill, Operation: "workbench-move-groups", Status: "queued", Message: "移动分组已排队", CreatedAt: now, UpdatedAt: now, Result: map[string]any{"account_id": id, "before": input.ExpectedGroupIDs, "after": input.GroupIDs}}
	if err = s.tasks.Save(ctx, task); err != nil {
		return taskstore.Task{}, err
	}
	err = s.runner.GoTask(task.ID, func(parent context.Context) {
		running := task
		run, cancel := context.WithTimeout(targetguard.Expect(parent, target), 2*time.Minute)
		defer cancel()
		running.Status, running.Message = "running", "正在核对并移动分组"
		running.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if !taskstore.SaveRunning(run, s.tasks, running) {
			return
		}
		result, moveErr := s.applyGroupMove(run, id, input, actor, task.ID)
		running.Result = result
		running.Status, running.Message = "succeeded", "移动分组完成"
		if moveErr != nil {
			running.Status, running.Message = "failed", moveErr.Error()
			running.Result["error"] = moveErr.Error()
		}
		running.Progress = 100
		running.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		taskstore.MarkCancelled(run, &running, "移动分组已取消，请刷新列表核对当前分组")
		taskstore.PersistFinal(s.tasks, running)
	})
	if err != nil {
		taskstore.PersistLaunchFailure(s.tasks, task, err)
		return taskstore.Task{}, err
	}
	return task, nil
}

func (s *Service) applyGroupMove(ctx context.Context, id string, input MoveGroupsInput, actor, operationID string) (result map[string]any, err error) {
	result = map[string]any{"account_id": id, "before": input.ExpectedGroupIDs, "after": input.GroupIDs, "remote_write": false, "readback_confirmed": false}
	ctx, release, err := s.acquireGroupMutation(ctx, mutationguard.Account(id))
	if err != nil {
		return result, err
	}
	defer release()
	ctx, err = targetguard.Bind(ctx, s.private)
	if err != nil {
		return result, err
	}
	preview, err := s.AccountGroups(ctx, id)
	if err != nil {
		return result, err
	}
	if err = validateMoveGroups(preview, input); err != nil {
		return result, err
	}
	target, err := targetguard.Settings(ctx, s.private)
	if err != nil {
		return result, err
	}
	client, err := s.client(target)
	if err != nil {
		return result, err
	}
	ctx = adminclient.WithMutationAuthorization(ctx, func(check context.Context) error {
		if _, err := targetguard.Pin(check, s.private); err != nil {
			return err
		}
		return s.groupsMoveAllowed(check, id)
	})
	ids := make([]int64, 0, len(input.GroupIDs))
	for _, id := range input.GroupIDs {
		n, _ := strconv.ParseInt(id, 10, 64)
		ids = append(ids, n)
	}
	// Record the attempted write separately from its readback. Uncertain writes
	// are never replayed or rolled back over a possible external edit.
	result["write_attempted"] = true
	after, err := client.UpdateAccountGroups(ctx, id, ids)
	if err != nil {
		err = errors.New("移动分组未确认，请刷新列表并在管理平台核对后重试")
	} else if text(after["id"]) != id {
		err = errors.New("写后账号 ID 不一致，请在管理平台核对分组")
	} else {
		result["remote_write"], result["readback_confirmed"] = true, true
	}
	if recorder, ok := s.groupProtection.(interface {
		RecordAccountOperation(context.Context, business.AccountOperation) error
	}); ok {
		field := "group_ids"
		operation := business.AccountOperation{OperationID: operationID, OperationType: "account.groups", State: "succeeded", Phase: "readback", Actor: actor, ObjectID: id, FieldName: &field, Before: input.ExpectedGroupIDs, After: input.GroupIDs, Writeback: true, RemoteConfirmed: err == nil, ReadbackConfirmed: err == nil}
		if err != nil {
			message := err.Error()
			operation.State, operation.Error = "failed", &message
		}
		auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if auditErr := recorder.RecordAccountOperation(auditCtx, operation); auditErr != nil {
			err = errors.Join(err, errors.New("分组操作审计保存失败，请刷新列表核对结果"))
		}
	}
	return result, err
}
