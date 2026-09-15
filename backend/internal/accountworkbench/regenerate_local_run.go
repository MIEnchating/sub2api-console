package accountworkbench

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func (s *Service) regenerateLocal(ctx context.Context, prepared *preparedExport, state *exportState) (taskstore.Task, error) {
	return s.enqueue(ctx, "account-workbench-regenerate", "等待核对独立 RT 再生来源", func(run context.Context, update func([]ResultItem) error) (rows []ResultItem, runErr error) {
		for _, item := range prepared.regeneration.view.Items {
			rows = append(rows, ResultItem{Index: item.Index, Name: item.Name, Email: item.Email, Status: "queued", Message: "等待核对本地来源"})
		}
		defer func() {
			if runErr == nil {
				return
			}
			for i := range rows {
				if rows[i].Status == "queued" || rows[i].Status == "running" {
					rows[i].Status, rows[i].Message = "failed", "再生未完成，请核对已生成文件后重新预览"
					if errors.Is(runErr, context.Canceled) {
						rows[i].Status, rows[i].Message = "cancelled", "再生已取消，已完成文件保留"
					}
				}
			}
		}()
		if err := update(rows); err != nil {
			return rows, err
		}
		guarded, release, err := mutationguard.Acquire(run, s.repository, "workbench-local-regeneration/"+prepared.owner)
		if err != nil {
			return rows, err
		}
		defer func() { _ = release() }()
		for i := range rows {
			if err := guarded.Err(); err != nil {
				return rows, err
			}
			source, err := s.loadLocalRegenerationSource(guarded, state, prepared.owner, prepared.regeneration.input)
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
			rows[i].Status, rows[i].Message = "running", "正在请求官方 RT 刷新并核对身份"
			if err := update(rows); err != nil {
				return rows, err
			}
			metadata, err := s.regenerateLocalAccount(guarded, state, prepared.owner, source.accounts[i])
			if err != nil {
				if guarded.Err() != nil {
					return rows, guarded.Err()
				}
				rows[i].Status, rows[i].Message = "failed", err.Error()
			} else {
				rows[i].Status, rows[i].Message, rows[i].Report = "succeeded", "授权已再生至独立私有文件", localExportMetadataReport(metadata)
			}
			if err := update(rows); err != nil {
				return rows, err
			}
		}
		return rows, nil
	})
}

func (s *Service) regenerateLocalAccount(ctx context.Context, state *exportState, owner string, account regenerationAccount) (ExportMetadata, error) {
	if err := state.regenerationAvailable(); err != nil {
		return ExportMetadata{}, err
	}
	credentials := inputObject(account.payload["credentials"])
	refreshed, err := s.refreshOfficialOAuth(ctx, inputText(credentials["refresh_token"]))
	if err != nil {
		return ExportMetadata{}, err
	}
	identity := inputIdentity(refreshed.Credentials)
	if identity.user != account.view.UserID || identity.workspace != account.view.WorkspaceID {
		return ExportMetadata{}, errors.New("官方刷新结果的用户或工作区与本地来源不一致，未生成文件")
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
	// A validated rotation is saved even when cancellation stops later items.
	persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	metadata, err := state.write(persist, owner, workbenchScopeFingerprint(ScopeLocalExport, configstore.TargetSettings{}), []map[string]any{payload})
	if err != nil {
		return ExportMetadata{}, errors.New("官方 RT 已刷新但私有文件保存失败，请核对服务器目录；未自动重放")
	}
	return metadata, nil
}
