package upstreamexclusion

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"slices"
	"time"
)

type Request struct {
	UpstreamID         string   `json:"upstream_id"`
	Excluded           bool     `json:"excluded"`
	StopScheduling     bool     `json:"stop_scheduling"`
	ExpectedAccountIDs []string `json:"expected_account_ids"`
}
type stopper interface {
	StopExcludedAccount(context.Context, string, string) (map[string]any, error)
}
type privateStore interface {
	TargetSettings(context.Context) (configstore.TargetSettings, error)
}
type taskStore interface {
	Save(context.Context, taskstore.Task) error
}
type Service struct {
	store   *business.Store
	private privateStore
	tasks   taskStore
	stopper stopper
	runner  taskrunner.Runner
}

func New(store *business.Store, private privateStore, tasks taskStore, stop stopper) *Service {
	return &Service{store: store, private: private, tasks: tasks, stopper: stop}
}
func (s *Service) UseTaskRunner(runner taskrunner.Runner) { s.runner = runner }
func (s *Service) Preview(ctx context.Context, host string) (business.UpstreamExclusionPreview, error) {
	return s.store.UpstreamExclusionPreview(ctx, host)
}
func (s *Service) validate(ctx context.Context, host string, request Request) (business.UpstreamExclusionPreview, error) {
	if request.StopScheduling && !request.Excluded {
		return business.UpstreamExclusionPreview{}, errors.New("取消排除不能同时关闭调度")
	}
	preview, err := s.Preview(ctx, host)
	if err != nil {
		return preview, err
	}
	expected := append([]string{}, request.ExpectedAccountIDs...)
	actual := append([]string{}, preview.AccountIDs...)
	slices.Sort(expected)
	slices.Sort(actual)
	if preview.UpstreamID != request.UpstreamID || !slices.Equal(expected, actual) {
		return preview, errors.New("上游或账号范围已变化，请重新读取并确认")
	}
	if request.StopScheduling {
		mode, err := s.store.Mode(ctx)
		if err != nil {
			return preview, err
		}
		if mode != runtimepolicy.Full {
			return preview, errors.New("关闭调度需要完全模式，也可以仅排除并保留调度状态")
		}
	}
	return preview, nil
}
func (s *Service) Enqueue(ctx context.Context, host string, request Request, actor string) (taskstore.Task, error) {
	if _, err := s.validate(ctx, host, request); err != nil {
		return taskstore.Task{}, err
	}
	target, err := s.private.TargetSettings(ctx)
	if err != nil {
		return taskstore.Task{}, err
	}
	var id [16]byte
	if _, err = rand.Read(id[:]); err != nil {
		return taskstore.Task{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{ID: hex.EncodeToString(id[:]), Skill: "sub2api-upstream-info", Operation: "upstream-exclusion", Status: "queued", Message: "上游排除设置已排队", Result: map[string]any{}, CreatedAt: now, UpdatedAt: now}
	if err = s.tasks.Save(ctx, task); err != nil {
		return taskstore.Task{}, err
	}
	request.ExpectedAccountIDs = append([]string{}, request.ExpectedAccountIDs...)
	if err = taskrunner.GoTask(s.runner, task.ID, func(parent context.Context) {
		runCtx, cancel := context.WithTimeout(targetguard.Expect(parent, target), 30*time.Minute)
		defer cancel()
		task.Status, task.Message = "running", "正在保存上游排除设置"
		if !taskstore.SaveRunning(runCtx, s.tasks, task) {
			return
		}
		result, runErr := s.Apply(runCtx, host, request, actor, func(done, total int, partial map[string]any) {
			task.Result = partial
			task.Progress = done * 100 / max(total, 1)
			task.Message = "正在关闭关联账号调度"
			task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			if !taskstore.SaveRunning(runCtx, s.tasks, task) {
				cancel()
			}
		})
		task.Result = result
		task.Progress = 100
		task.Status, task.Message = "succeeded", "上游排除设置已完成"
		if runErr != nil {
			task.Status, task.Message = "failed", "上游排除设置未全部完成"
			task.Result["error"] = runErr.Error()
		}
		task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		taskstore.MarkCancelled(runCtx, &task, "上游排除任务已取消；已保存的排除状态和已关闭的调度保留")
		taskstore.PersistFinal(s.tasks, task)
	}); err != nil {
		taskstore.PersistLaunchFailure(s.tasks, task, err)
		return taskstore.Task{}, err
	}
	return task, nil
}
func (s *Service) Apply(ctx context.Context, host string, request Request, actor string, progress func(int, int, map[string]any)) (map[string]any, error) {
	result := map[string]any{"host": host, "excluded": false, "accounts": []map[string]any{}}
	resources := []string{mutationguard.UpstreamCatalog(), mutationguard.AccountCatalog(), mutationguard.Upstream(host)}
	for _, id := range request.ExpectedAccountIDs {
		resources = append(resources, mutationguard.Account(id))
	}
	guarded, release, err := targetguard.Acquire(ctx, s.store, resources...)
	if err != nil {
		return result, err
	}
	defer release()
	ctx = guarded
	preview, err := s.validate(ctx, host, request)
	if err != nil {
		return result, err
	}
	if request.StopScheduling {
		ctx, err = targetguard.Bind(ctx, s.private)
		if err != nil {
			return result, err
		}
	}
	if err = s.store.SetUpstreamExcluded(ctx, preview.UpstreamID, request.Excluded, actor); err != nil {
		return result, err
	}
	result["excluded"] = request.Excluded
	if !request.StopScheduling {
		return result, nil
	}
	rows := []map[string]any{}
	var failures []error
	for index, id := range preview.AccountIDs {
		if err := ctx.Err(); err != nil {
			failures = append(failures, err)
			break
		}
		row, err := s.stopper.StopExcludedAccount(ctx, id, actor)
		if err != nil {
			row = map[string]any{"account_id": id, "error": err.Error()}
			failures = append(failures, err)
		}
		rows = append(rows, row)
		result["accounts"] = rows
		if progress != nil {
			progress(index+1, len(preview.AccountIDs), result)
		}
	}
	result["accounts"] = rows
	if len(failures) > 0 {
		return result, errors.New("排除状态已保存，但部分账号调度关闭未确认；请查看逐项结果后重试")
	}
	return result, nil
}
