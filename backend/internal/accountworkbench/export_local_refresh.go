package accountworkbench

import (
	"context"
	"errors"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func (s *Service) exportLocalRefreshInput(ctx context.Context, owner string, prepared *preparedImport, state *exportState) (taskstore.Task, error) {
	return s.enqueue(ctx, "account-workbench-convert", "等待刷新并生成独立私有账号文件", func(run context.Context, update func([]ResultItem) error) (rows []ResultItem, runErr error) {
		defer func() { prepared.items = nil }()
		for _, item := range prepared.items {
			rows = append(rows, ResultItem{Index: item.Index, Name: item.Name, Email: item.Email, Status: "queued", Message: "等待私有转换"})
		}
		defer func() {
			if runErr == nil {
				return
			}
			for i := range rows {
				if rows[i].Status == "queued" || rows[i].Status == "running" {
					rows[i].Status, rows[i].Message = "failed", "私有转换未完成，请核对已生成文件后重新预览"
					if errors.Is(runErr, context.Canceled) {
						rows[i].Status, rows[i].Message = "cancelled", "私有转换已取消，已生成文件保留"
					}
				}
			}
		}()
		if err := update(rows); err != nil {
			return rows, err
		}
		guarded, release, err := mutationguard.Acquire(run, s.repository, "workbench-local-regeneration/"+exportHash(owner))
		if err != nil {
			return rows, err
		}
		defer func() { _ = release() }()
		for i, item := range prepared.items {
			if err := guarded.Err(); err != nil {
				return rows, err
			}
			rows[i].Status, rows[i].Message = "running", "正在转换并核对授权凭据"
			if err := update(rows); err != nil {
				return rows, err
			}
			metadata, err := s.exportLocalRefreshItem(guarded, state, exportHash(owner), item)
			if err != nil {
				if guarded.Err() != nil {
					return rows, guarded.Err()
				}
				rows[i].Status, rows[i].Message = "failed", err.Error()
			} else {
				rows[i].Status, rows[i].Message, rows[i].Report = "succeeded", "独立私有账号文件已生成", localExportMetadataReport(metadata)
			}
			if err := update(rows); err != nil {
				return rows, err
			}
		}
		return rows, nil
	})
}

func (s *Service) exportLocalRefreshItem(ctx context.Context, state *exportState, owner string, item InputItem) (ExportMetadata, error) {
	if err := state.regenerationAvailable(); err != nil {
		return ExportMetadata{}, err
	}
	if inputText(item.Credentials["access_token"]) == "" {
		refreshed, err := s.refreshOfficialOAuth(ctx, inputText(item.Credentials["refresh_token"]))
		if err != nil {
			return ExportMetadata{}, err
		}
		before, after := inputIdentity(item.Credentials), inputIdentity(refreshed.Credentials)
		if after.user == "" || after.workspace == "" || (before.user != "" && before.user != after.user) || (before.workspace != "" && before.workspace != after.workspace) {
			return ExportMetadata{}, errors.New("官方刷新未返回匹配的用户及工作区，未生成文件，请重新授权")
		}
		credentials := cloneInputMap(item.Credentials)
		for _, key := range []string{"access_token", "refresh_token", "id_token", "expires_at", "expires_in"} {
			delete(credentials, key)
		}
		for key, value := range refreshed.Credentials {
			credentials[key] = value
		}
		item.Credentials = credentials
	}
	payload, err := ApplyTemplate(item, nil)
	if err != nil {
		return ExportMetadata{}, errors.New("账号转换未通过校验，请重新授权")
	}
	// Persist completed rotations before cancellation can stop the next item.
	persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	metadata, err := state.write(persist, owner, workbenchScopeFingerprint(ScopeLocalExport, configstore.TargetSettings{}), []map[string]any{payload})
	if err != nil {
		return ExportMetadata{}, errors.New("私有文件保存失败，请核对服务器目录；官方刷新不会自动重放")
	}
	return metadata, nil
}
