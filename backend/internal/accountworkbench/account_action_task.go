package accountworkbench

import (
	"context"
	"errors"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func (s *Service) StartAccountAction(ctx context.Context, owner, id string, input AccountActionInput, actor string) (taskstore.Task, error) {
	if s.tasks == nil || s.runner == nil {
		return taskstore.Task{}, errors.New("工作台任务执行器尚未就绪")
	}
	if err := s.checkOwner(ctx, owner); err != nil {
		return taskstore.Task{}, err
	}
	preview, err := s.AccountActionPreview(ctx, owner, id)
	if err != nil {
		return taskstore.Task{}, err
	}
	if err = validateAccountAction(preview, input); err != nil {
		return taskstore.Task{}, err
	}
	if err = s.accountActionAllowed(ctx, id, input); err != nil {
		return taskstore.Task{}, err
	}
	target, err := targetguard.Settings(ctx, s.private)
	if err != nil {
		return taskstore.Task{}, err
	}
	if targetguard.Fingerprint(target) != input.TargetVersion {
		return taskstore.Task{}, targetguard.ErrChanged
	}
	if input.Schedulable != nil {
		enabled := *input.Schedulable
		input.Schedulable = &enabled
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{ID: newID(), Skill: Skill, Operation: "workbench-account-" + input.Action, Status: "queued", Message: "账号操作已排队", CreatedAt: now, UpdatedAt: now, Result: map[string]any{"account_id": id}}
	if err = s.tasks.Save(ctx, task); err != nil {
		return taskstore.Task{}, err
	}
	err = s.runner.GoTask(task.ID, func(parent context.Context) {
		run, cancel := context.WithTimeout(targetguard.Expect(parent, target), 2*time.Minute)
		defer cancel()
		running := task
		running.Status, running.Message = "running", "正在核对并执行账号操作"
		running.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if !taskstore.SaveRunning(run, s.tasks, running) {
			return
		}
		result, actionErr := s.applyAccountAction(run, owner, id, input, actor, task.ID)
		running.Result = result
		running.Status, running.Message = "succeeded", "账号操作完成"
		if actionErr != nil {
			running.Status, running.Message = "failed", actionErr.Error()
			running.Result["error"] = actionErr.Error()
		}
		running.Progress = 100
		running.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		taskstore.MarkCancelled(run, &running, "账号操作已中断，请刷新列表核对结果；不会自动重放")
		taskstore.PersistFinal(s.tasks, running)
	})
	if err != nil {
		taskstore.PersistLaunchFailure(s.tasks, task, err)
		return taskstore.Task{}, err
	}
	return task, nil
}

func (s *Service) applyAccountAction(ctx context.Context, owner, id string, input AccountActionInput, actor, operationID string) (result map[string]any, err error) {
	result = map[string]any{"account_id": id, "remote_confirmed": false, "remembered_deleted": false}
	ctx, release, err := s.acquireGroupMutation(ctx, mutationguard.Account(id))
	if err != nil {
		return result, err
	}
	defer release()
	ctx, err = targetguard.Bind(ctx, s.private)
	if err != nil {
		return result, err
	}
	if err = s.checkOwner(ctx, owner); err != nil {
		return result, err
	}
	s.rememberMu.Lock()
	defer s.rememberMu.Unlock()
	preview, row, err := s.accountActionSnapshot(ctx, id)
	if err != nil {
		return result, err
	}
	if err = validateAccountAction(preview, input); err != nil {
		return result, err
	}
	if err = s.accountActionAllowed(ctx, id, input); err != nil {
		return result, err
	}
	target, err := targetguard.Settings(ctx, s.private)
	if err != nil {
		return result, err
	}
	if input.DeleteRemembered && preview.PlatformPresent {
		saved, readErr := s.readRememberedAccount(ctx, target, preview.Remembered.ID)
		if readErr != nil {
			return result, readErr
		}
		if !sameAccountIdentity(row, saved.Input.Credentials) {
			return result, errors.New("平台账号身份与已记住账号不一致，未执行删除，请核对后重试")
		}
	}
	client, err := s.client(target)
	if err != nil {
		return result, err
	}
	ctx = adminclient.WithMutationAuthorization(ctx, func(check context.Context) error {
		if _, err := targetguard.Pin(check, s.private); err != nil {
			return err
		}
		return s.accountActionAllowed(check, id, input)
	})
	defer func() {
		err = errors.Join(err, s.recordAccountAction(ctx, id, actor, operationID, input, preview, result, err))
	}()
	if input.Action == "scheduling" {
		result["write_attempted"] = true
		if _, err = client.SetAccountSchedulable(ctx, id, *input.Schedulable); err != nil {
			return result, errors.New("调度写入未确认，请刷新列表核对后重试")
		}
		after, readErr := client.Account(ctx, id)
		if readErr != nil || text(after["id"]) != id || after["schedulable"] != *input.Schedulable {
			return result, errors.New("调度状态读回不一致，请在管理平台核对")
		}
		result["remote_confirmed"], result["schedulable"] = true, *input.Schedulable
		return result, nil
	}
	if preview.PlatformPresent {
		result["write_attempted"] = true
		if _, err = client.DeleteAccount(ctx, id); err != nil {
			return result, errors.New("平台账号删除未确认，已保留本地资料，请刷新列表核对后重试")
		}
	}
	result["remote_confirmed"] = true
	if input.DeleteRemembered {
		if err = s.private.DeleteWorkbenchDocument(ctx, rememberedAccountKey(target, preview.Remembered.ID), input.RememberedRevision); err != nil {
			return result, errors.New("平台账号已删除，本地资料清理失败，请在已记住账号中重试删除")
		}
		result["remembered_deleted"] = true
	}
	return result, nil
}

func (s *Service) recordAccountAction(ctx context.Context, id, actor, operationID string, input AccountActionInput, preview AccountActionPreview, result map[string]any, actionErr error) error {
	recorder, ok := s.groupProtection.(interface {
		RecordAccountOperation(context.Context, business.AccountOperation) error
	})
	if !ok {
		return nil
	}
	confirmed := result["remote_confirmed"] == true
	op := business.AccountOperation{OperationID: operationID, OperationType: "account." + input.Action, State: "succeeded", Phase: "readback", Actor: actor, ObjectID: id, Writeback: result["write_attempted"] == true, RemoteConfirmed: confirmed, ReadbackConfirmed: confirmed}
	if input.Action == "scheduling" {
		field := "schedulable"
		op.FieldName, op.Before, op.After = &field, preview.Schedulable, *input.Schedulable
	} else {
		op.Before = map[string]any{"platform_present": preview.PlatformPresent}
		op.After = result
	}
	if actionErr != nil {
		message := actionErr.Error()
		op.State, op.Error = "failed", &message
	}
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := recorder.RecordAccountOperation(auditCtx, op); err != nil {
		return errors.New("账号操作审计保存失败，请刷新列表核对结果")
	}
	return nil
}
