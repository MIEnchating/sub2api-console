package upstreamdelete

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

const deleteConcurrency = 4

type Repository interface {
	Mode(context.Context) (string, error)
	UpstreamDeletePreview(context.Context, string) (business.UpstreamDeletePreview, error)
	DeleteUpstreamProjection(context.Context, string, []string, business.UpstreamDeleteAudit) (business.UpstreamDeleteProjection, error)
}

type PrivateStore interface {
	TargetSettings(context.Context) (configstore.TargetSettings, error)
	DeleteAuthRecord(context.Context, string) (bool, error)
}

type TaskStore interface {
	Save(context.Context, taskstore.Task) error
}

type Service struct {
	repository Repository
	private    PrivateStore
	tasks      TaskStore
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
	go s.execute(task, preview.Host, append([]string{}, expected...), actor)
	return task, nil
}

func (s *Service) execute(task taskstore.Task, host string, expected []string, actor string) {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	task.Status, task.Progress, task.Message, task.UpdatedAt = "running", 10, "正在删除 Sub2API 账号", time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.tasks.Save(ctx, task); err != nil {
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
	target, err := s.private.TargetSettings(ctx)
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
	privateDeleted, err := s.private.DeleteAuthRecord(ctx, preview.Host)
	if err != nil {
		return Result{RemoteDeletedAccounts: remoteDeleted, RemoteWrite: remoteDeleted > 0},
			fmt.Errorf("远端账号已删除，但私有鉴权记录删除失败：%w", err)
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
