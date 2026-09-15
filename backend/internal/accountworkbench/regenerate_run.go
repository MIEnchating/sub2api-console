package accountworkbench

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func (s *Service) Regenerate(ctx context.Context, owner, previewID string, confirmed bool) (taskstore.Task, error) {
	if !confirmed {
		return taskstore.Task{}, errors.New("请先确认使用所选 RT 重新生成私有文件")
	}
	if owner == "" {
		return taskstore.Task{}, ErrExportPreview
	}
	state, err := s.exportStorage()
	if err != nil {
		return taskstore.Task{}, err
	}
	prepared, err := state.takePreview(exportHash(owner), previewID, exportRegeneration)
	if err != nil {
		return taskstore.Task{}, err
	}
	if prepared.regeneration.input.Scope == ScopeLocalExport {
		return s.regenerateLocal(ctx, prepared, state)
	}
	if _, err := targetguard.Pin(targetguard.Expect(ctx, prepared.target), s.private); err != nil {
		return taskstore.Task{}, err
	}
	return s.enqueue(ctx, "account-workbench-regenerate", "等待核对 RT 再生来源", func(run context.Context, update func([]ResultItem) error) (rows []ResultItem, runErr error) {
		rows = make([]ResultItem, len(prepared.regeneration.view.Items))
		for i, item := range prepared.regeneration.view.Items {
			rows[i] = ResultItem{Index: item.Index, AccountID: item.AccountID, Name: item.Name, Email: item.Email, Status: "queued", Message: "等待重新核对来源"}
		}
		defer func() {
			if runErr == nil {
				return
			}
			for i := range rows {
				if rows[i].Status != "queued" && rows[i].Status != "running" {
					continue
				}
				rows[i].Status, rows[i].Message = "failed", "来源或执行状态发生变化，请核对已生成文件后重新预览"
				if errors.Is(runErr, context.Canceled) {
					rows[i].Status, rows[i].Message = "cancelled", "再生已取消，请核对已完成项目"
				}
			}
		}()
		if err := update(rows); err != nil {
			return rows, err
		}
		guarded, release, err := targetguard.Acquire(targetguard.Expect(run, prepared.target), s.repository)
		if err != nil {
			return rows, err
		}
		defer func() { _ = release() }()
		guarded, err = targetguard.Bind(guarded, s.private)
		if err != nil {
			return rows, err
		}
		if err := state.regenerationAvailable(); err != nil {
			return rows, err
		}
		source, err := s.loadRegenerationSource(guarded, state, prepared.owner, prepared.target, prepared.regeneration.input)
		if err != nil {
			return rows, err
		}
		current := make([]RegenerationItem, 0, len(source.accounts))
		for _, account := range source.accounts {
			current = append(current, account.view)
		}
		if source.revision != prepared.regeneration.sourceRevision || !slices.Equal(current, prepared.regeneration.view.Items) {
			return rows, errRegenerationSource
		}
		for i, account := range source.accounts {
			if err := guarded.Err(); err != nil {
				return rows, err
			}
			if !source.expires.IsZero() && !time.Now().Before(source.expires) {
				return rows, errRegenerationSource
			}
			rows[i].Status, rows[i].Message = "running", "正在刷新 RT 并核对官方身份"
			if err := update(rows); err != nil {
				return rows, err
			}
			metadata, err := s.regenerateAccount(guarded, state, prepared, account)
			if err != nil {
				if guarded.Err() != nil {
					return rows, guarded.Err()
				}
				rows[i].Status, rows[i].Message = "failed", err.Error()
			} else {
				rows[i].Status, rows[i].Message, rows[i].Report = "succeeded", "授权已再生至私有文件", exportMetadataReport(metadata)
			}
			if err := update(rows); err != nil {
				return rows, err
			}
		}
		return rows, nil
	})
}

func (s *Service) regenerateAccount(ctx context.Context, state *exportState, prepared *preparedExport, account regenerationAccount) (ExportMetadata, error) {
	client, err := s.clientFor(prepared.target)
	if err != nil {
		return ExportMetadata{}, errors.New("管理目标不可用，请重新配置后预览")
	}
	if account.view.AccountID != "" {
		live, err := client.Account(ctx, account.view.AccountID)
		if err != nil || stringValue(live["id"]) != account.view.AccountID {
			return ExportMetadata{}, errRegenerationSource
		}
		current, err := regenerationSnapshot(account.view.Index, account.view.AccountID, live, prepared.target)
		if err != nil || current.view != account.view {
			return ExportMetadata{}, errRegenerationSource
		}
	}
	if _, err := targetguard.Pin(ctx, s.private); err != nil {
		return ExportMetadata{}, err
	}
	if err := state.regenerationAvailable(); err != nil {
		return ExportMetadata{}, err
	}
	credentials := inputObject(account.payload["credentials"])
	refreshed, err := materialize(ctx, client, InputItem{Index: account.view.Index, Name: account.view.Name, Credentials: map[string]any{"refresh_token": credentials["refresh_token"]}})
	if err != nil {
		return ExportMetadata{}, errors.New("RT 刷新未取得可验证结果，未自动重放；请核对原授权后重新处理")
	}
	identity := inputIdentity(refreshed.Credentials)
	if identity.user != account.view.UserID || identity.workspace != account.view.WorkspaceID {
		return ExportMetadata{}, errors.New("刷新返回的官方用户或工作区与预览不一致，未生成文件")
	}
	merged := cloneInputMap(credentials)
	for _, key := range []string{"access_token", "refresh_token", "id_token", "expires_at", "expires_in"} {
		delete(merged, key)
	}
	for key, value := range refreshed.Credentials {
		merged[key] = value
	}
	payload := cloneInputMap(account.payload)
	payload["credentials"] = merged
	persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if _, err := targetguard.Pin(persist, s.private); err != nil {
		return ExportMetadata{}, err
	}
	// Once a validated refresh response arrives, cancellation stops subsequent
	// rows but must not discard the newly rotated credentials in this row.
	metadata, err := state.write(persist, prepared.owner, exportTargetHash(prepared.target), []map[string]any{payload})
	if err != nil {
		return ExportMetadata{}, errors.New("RT 已刷新但私有文件保存失败，请核对服务器目录与原授权；未自动重放")
	}
	return metadata, nil
}

func (s *exportState) regenerationAvailable() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrExportUnavailable
	}
	return nil
}
