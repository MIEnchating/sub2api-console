package upstreamdelete

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

const deleteConcurrency = 4

type Repository interface {
	Mode(context.Context) (string, error)
	UpstreamDeletePreview(context.Context, string) (business.UpstreamDeletePreview, error)
	DeleteUpstreamProjection(context.Context, string, []string, business.UpstreamDeleteAudit) (business.UpstreamDeleteProjection, error)
	ManualPriorityControls(context.Context, []string) (map[string]business.ManualPriorityControl, error)
}

type PrivateStore interface {
	TargetSettings(context.Context) (configstore.TargetSettings, error)
	DeleteAuthRecord(context.Context, string) (bool, error)
}

type mutationProtectionRepository interface {
	AccountMutationProtections(context.Context, []string) (map[string]business.AccountMutationProtection, error)
}

type TaskStore interface {
	Save(context.Context, taskstore.Task) error
}

type Service struct {
	repository Repository
	private    PrivateStore
	tasks      TaskStore
	taskRunner taskrunner.Runner
	timeout    time.Duration
}

type Result struct {
	business.UpstreamDeleteProjection
	RemoteDeletedAccounts int   `json:"remote_deleted_accounts"`
	PrivateAuthDeleted    bool  `json:"private_auth_deleted"`
	EventID               int64 `json:"event_id"`
	RemoteWrite           bool  `json:"remote_write"`
	ReadbackConfirmed     bool  `json:"readback_confirmed"`
}

func New(repository Repository, private PrivateStore, tasks TaskStore) *Service {
	return &Service{repository: repository, private: private, tasks: tasks, timeout: 30 * time.Minute}
}

func (s *Service) UseTaskRunner(runner taskrunner.Runner) { s.taskRunner = runner }

func (s *Service) Preview(ctx context.Context, host string) (business.UpstreamDeletePreview, error) {
	return s.repository.UpstreamDeletePreview(ctx, host)
}

func (s *Service) Enqueue(ctx context.Context, host string, expected []string, actor string) (taskstore.Task, error) {
	mode, err := s.repository.Mode(ctx)
	if err != nil {
		return taskstore.Task{}, err
	}
	if mode != runtimepolicy.Full {
		return taskstore.Task{}, errors.New("上游删除只能在完全模式执行")
	}
	preview, err := s.repository.UpstreamDeletePreview(ctx, host)
	if err != nil {
		return taskstore.Task{}, err
	}
	if !sameIDs(preview.AccountIDs, expected) {
		return taskstore.Task{}, errors.New("删除预览后的账号范围已变化，请重新确认")
	}
	if err := s.rejectManualPriorityAccounts(ctx, preview.AccountIDs); err != nil {
		return taskstore.Task{}, err
	}
	expectedTarget, err := s.private.TargetSettings(ctx)
	if err != nil {
		return taskstore.Task{}, err
	}
	id, err := taskID()
	if err != nil {
		return taskstore.Task{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{
		ID: id, Skill: "sub2api-upstream-info", Operation: "upstream-delete", Status: "queued", Progress: 0,
		Message: "上游删除已排队", Result: map[string]any{}, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.tasks.Save(ctx, task); err != nil {
		return taskstore.Task{}, err
	}
	if err := taskrunner.Go(s.taskRunner, func(parent context.Context) {
		s.execute(targetguard.Expect(parent, expectedTarget), task, preview.Host, append([]string{}, expected...), actor)
	}); err != nil {
		taskstore.PersistLaunchFailure(s.tasks, task, err)
		return taskstore.Task{}, err
	}
	return task, nil
}

func (s *Service) execute(parent context.Context, task taskstore.Task, host string, expected []string, actor string) {
	ctx, cancel := context.WithTimeout(parent, s.timeout)
	defer cancel()
	task.Status, task.Progress, task.Message, task.UpdatedAt = "running", 10, "正在删除 Sub2API 账号", time.Now().UTC().Format(time.RFC3339Nano)
	if !taskstore.SaveRunning(ctx, s.tasks, task) {
		return
	}
	result, err := s.Delete(ctx, host, expected, actor)
	task.Progress, task.UpdatedAt = 100, time.Now().UTC().Format(time.RFC3339Nano)
	if err != nil {
		task.Status, task.Message = "failed", "上游删除失败"
		task.Result = map[string]any{
			"host": host, "error": err.Error(), "remote_deleted_accounts": result.RemoteDeletedAccounts,
			"private_auth_deleted": result.PrivateAuthDeleted, "remote_write": result.RemoteWrite,
		}
	} else {
		task.Status, task.Message = "succeeded", "上游及关联账号已删除"
		task.Result = map[string]any{
			"host": result.Host, "deleted_accounts": result.DeletedAccounts, "deleted_groups": result.DeletedGroups,
			"remote_deleted_accounts": result.RemoteDeletedAccounts, "private_auth_deleted": result.PrivateAuthDeleted,
			"event_id": result.EventID, "remote_write": result.RemoteWrite,
			"readback_confirmed": result.ReadbackConfirmed,
		}
	}
	taskstore.MarkCancelled(ctx, &task, "上游删除已取消")
	taskstore.PersistFinal(s.tasks, task)
}

func (s *Service) Delete(ctx context.Context, host string, expected []string, actor string) (Result, error) {
	preview, err := s.repository.UpstreamDeletePreview(ctx, host)
	if err != nil {
		return Result{}, err
	}
	if !sameIDs(preview.AccountIDs, expected) {
		return Result{}, errors.New("删除预览后的账号范围已变化，请重新确认")
	}
	identityHosts := deleteIdentityHosts(preview)
	resources := []string{mutationguard.AccountCatalog(), mutationguard.UpstreamCatalog()}
	for _, identityHost := range identityHosts {
		resources = append(resources, mutationguard.Upstream(identityHost))
	}
	for _, accountID := range preview.AccountIDs {
		resources = append(resources, mutationguard.Account(accountID))
	}
	guardedCtx, release, err := targetguard.Acquire(ctx, s.repository, resources...)
	if err != nil {
		return Result{}, err
	}
	defer func() {
		if err := release(); err != nil {
			slog.Error("上游删除租约释放失败", "host", preview.Host, "error", err)
		}
	}()
	ctx = guardedCtx
	preview, err = s.repository.UpstreamDeletePreview(ctx, preview.Host)
	if err != nil {
		return Result{}, err
	}
	if !sameIDs(preview.AccountIDs, expected) {
		return Result{}, errors.New("获取删除锁后账号范围已变化，请重新确认")
	}
	if !sameHosts(identityHosts, deleteIdentityHosts(preview)) {
		return Result{}, errors.New("获取删除锁后稳定上游别名集合已变化，请重新确认")
	}
	if err := s.rejectMutationProtectedAccounts(ctx, preview.AccountIDs); err != nil {
		return Result{}, err
	}
	ctx, err = targetguard.Bind(ctx, s.private)
	if err != nil {
		return Result{}, err
	}
	target, err := targetguard.Settings(ctx, s.private)
	if err != nil {
		return Result{}, err
	}
	client, err := adminclient.New(adminclient.Config{
		BaseURL: target.BaseURL, AdminKey: target.AdminKey,
		Timeout: time.Duration(target.TimeoutSeconds) * time.Second, Attempts: 3,
	}, nil)
	if err != nil {
		return Result{}, err
	}
	deleteErrors := deleteAccounts(ctx, client, preview.AccountIDs, true, deleteConcurrency)
	remoteDeleted := 0
	var firstError error
	for index, deleteErr := range deleteErrors {
		if deleteErr == nil {
			remoteDeleted++
			continue
		}
		if firstError == nil {
			firstError = fmt.Errorf("账号 %s 远程删除失败：%w", preview.AccountIDs[index], deleteErr)
		}
	}
	if firstError != nil {
		return Result{RemoteDeletedAccounts: remoteDeleted, RemoteWrite: remoteDeleted > 0}, firstError
	}
	privateDeleted := false
	for _, identityHost := range deleteIdentityHosts(preview) {
		deleted, err := s.private.DeleteAuthRecord(ctx, identityHost)
		if err != nil {
			return Result{RemoteDeletedAccounts: remoteDeleted, PrivateAuthDeleted: privateDeleted, RemoteWrite: remoteDeleted > 0},
				fmt.Errorf("远端账号已删除，但 Host %s 的私有鉴权记录删除失败：%w", identityHost, err)
		}
		privateDeleted = privateDeleted || deleted
	}
	projection, err := s.repository.DeleteUpstreamProjection(ctx, preview.Host, preview.AccountIDs, business.UpstreamDeleteAudit{
		Actor: actor, RemoteDeletedAccounts: remoteDeleted, PrivateAuthDeleted: privateDeleted, ReadbackConfirmed: true,
	})
	if err != nil {
		return Result{RemoteDeletedAccounts: remoteDeleted, PrivateAuthDeleted: privateDeleted, RemoteWrite: remoteDeleted > 0},
			fmt.Errorf("远端账号已删除，但本地投影与事件提交失败：%w", err)
	}
	return Result{
		UpstreamDeleteProjection: projection, RemoteDeletedAccounts: remoteDeleted,
		PrivateAuthDeleted: privateDeleted, EventID: projection.EventID, RemoteWrite: true, ReadbackConfirmed: true,
	}, nil
}

func deleteIdentityHosts(preview business.UpstreamDeletePreview) []string {
	hosts := append([]string{}, preview.IdentityHosts...)
	if len(hosts) == 0 {
		hosts = append(hosts, preview.Host)
	}
	seen := make(map[string]struct{}, len(hosts))
	result := make([]string, 0, len(hosts))
	for _, host := range hosts {
		host = configstore.CanonicalHost(host)
		if host == "" {
			continue
		}
		if _, duplicate := seen[host]; duplicate {
			continue
		}
		seen[host] = struct{}{}
		result = append(result, host)
	}
	sort.Strings(result)
	return result
}

func sameHosts(left, right []string) bool {
	left = deleteIdentityHosts(business.UpstreamDeletePreview{IdentityHosts: left})
	right = deleteIdentityHosts(business.UpstreamDeletePreview{IdentityHosts: right})
	return strings.Join(left, "\x00") == strings.Join(right, "\x00")
}

func (s *Service) rejectMutationProtectedAccounts(ctx context.Context, accountIDs []string) error {
	repository, ok := s.repository.(mutationProtectionRepository)
	if !ok {
		return s.rejectManualPriorityAccounts(ctx, accountIDs)
	}
	protections, err := repository.AccountMutationProtections(ctx, accountIDs)
	if err != nil {
		return fmt.Errorf("人工保护状态读取失败：%w", err)
	}
	protected := make([]string, 0, len(protections))
	for accountID, protection := range protections {
		if protection.Protected() {
			protected = append(protected, accountID+"（"+strings.Join(protection.Reasons(), "、")+"）")
		}
	}
	if len(protected) == 0 {
		return nil
	}
	sort.Strings(protected)
	return fmt.Errorf("上游包含人工保护账号 %s，请先解除保护再删除", strings.Join(protected, "、"))
}

func (s *Service) rejectManualPriorityAccounts(ctx context.Context, accountIDs []string) error {
	controls, err := s.repository.ManualPriorityControls(ctx, accountIDs)
	if err != nil {
		return fmt.Errorf("人工优先位保护状态读取失败：%w", err)
	}
	if len(controls) == 0 {
		return nil
	}
	protected := make([]string, 0, len(controls))
	for accountID := range controls {
		protected = append(protected, accountID)
	}
	sort.Strings(protected)
	return fmt.Errorf("上游包含人工优先位账号 %s，请先取消人工优先位再删除", strings.Join(protected, "、"))
}

type accountDeleter interface {
	DeleteAccountWithVerification(context.Context, string, bool) (map[string]any, error)
}

func deleteAccounts(ctx context.Context, client accountDeleter, accountIDs []string, verification bool, concurrency int) []error {
	result := make([]error, len(accountIDs))
	if len(accountIDs) == 0 {
		return result
	}
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > len(accountIDs) {
		concurrency = len(accountIDs)
	}
	jobs := make(chan int)
	var workers sync.WaitGroup
	for worker := 0; worker < concurrency; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				outcome, err := client.DeleteAccountWithVerification(ctx, accountIDs[index], verification)
				if err == nil && verification {
					if confirmed, _ := outcome["confirmed_absent"].(bool); !confirmed {
						err = errors.New("远程删除结果未通过复核")
					}
				}
				result[index] = err
			}
		}()
	}
	for index := range accountIDs {
		select {
		case jobs <- index:
		case <-ctx.Done():
			close(jobs)
			workers.Wait()
			for pending := index; pending < len(accountIDs); pending++ {
				result[pending] = ctx.Err()
			}
			return result
		}
	}
	close(jobs)
	workers.Wait()
	return result
}

func sameIDs(left, right []string) bool {
	leftCopy, rightCopy := append([]string{}, left...), append([]string{}, right...)
	for index := range leftCopy {
		leftCopy[index] = strings.TrimSpace(leftCopy[index])
	}
	for index := range rightCopy {
		rightCopy[index] = strings.TrimSpace(rightCopy[index])
	}
	sort.Strings(leftCopy)
	sort.Strings(rightCopy)
	return strings.Join(leftCopy, "\x00") == strings.Join(rightCopy, "\x00")
}

func taskID() (string, error) {
	value := make([]byte, 6)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
