package accountworkbench

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func (s *Service) previewLocalItems(ctx context.Context, owner string, input PreviewInput, items []InputItem) (Preview, error) {
	view := Preview{Scope: ScopeLocalExport, ExportOnly: true, Items: []PreviewItem{}, Errors: []InputError{}}
	if owner == "" || !input.ExportOnly || input.TemplateID != "" || input.CheckAfterImport {
		return view, errors.New("独立导出仅接受当前会话的私有文件转换")
	}
	if err := ctx.Err(); err != nil {
		return view, err
	}
	if len(items) == 0 || len(items) > 500 {
		return view, errors.New("单批需要 1～500 个有效账号")
	}
	identities := make(map[string]bool, len(items))
	for _, item := range items {
		refreshRequired := inputText(item.Credentials["access_token"]) == ""
		key := IdentityKey(item)
		if key != "" && identities[key] {
			return view, fmt.Errorf("第 %d 项与本批其他账号重复，请删除重复项", item.Index+1)
		}
		identities[key] = true
		for _, field := range []string{"access_token", "refresh_token", "id_token"} {
			secret := inputText(item.Credentials[field])
			if secret != "" && (strings.Contains(item.Name, secret) || strings.Contains(item.Email, secret) || strings.Contains(item.PlanType, secret)) {
				return view, errors.New("账号名称、邮箱和套餐信息不能包含授权凭据")
			}
		}
		view.Items = append(view.Items, PreviewItem{ID: strconv.Itoa(item.Index), Index: item.Index, Name: item.Name, Email: item.Email, PlanType: item.PlanType, GroupIDs: []string{}, RefreshRequired: refreshRequired})
	}
	if len(view.Errors) > 0 {
		view.Items = []PreviewItem{}
		return view, nil
	}
	id, err := randomID()
	if err != nil {
		return view, err
	}
	expires := time.Now().Add(10 * time.Minute)
	view.ID, view.ExpiresAt = id, expires.UTC().Format(time.RFC3339)
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, prepared := range s.previews {
		if time.Now().After(prepared.expires) || prepared.owner == owner {
			delete(s.previews, id)
		}
	}
	if len(s.previews) >= 20 {
		return Preview{}, errors.New("预览任务已满，请稍后重试")
	}
	s.previews[id] = &preparedImport{owner: owner, expires: expires, view: view, items: items}
	time.AfterFunc(time.Until(expires), func() { s.DeletePreview(owner, id) })
	return view, nil
}

func (s *Service) exportLocalInput(ctx context.Context, owner string, prepared *preparedImport, state *exportState) (taskstore.Task, error) {
	for _, item := range prepared.items {
		if inputText(item.Credentials["access_token"]) == "" {
			return s.exportLocalRefreshInput(ctx, owner, prepared, state)
		}
	}
	return s.enqueue(ctx, "account-workbench-convert", "等待生成独立私有账号文件", func(run context.Context, update func([]ResultItem) error) (rows []ResultItem, runErr error) {
		defer func() { prepared.items = nil }()
		rows = []ResultItem{{Index: 0, Name: "独立私有账号文件", Status: "running", Message: "正在转换账号"}}
		defer func() {
			if runErr != nil {
				rows[0].Status, rows[0].Message = "failed", "独立私有文件生成失败，请重新预览"
				if errors.Is(runErr, context.Canceled) {
					rows[0].Status, rows[0].Message = "cancelled", "独立私有转换已取消"
				}
			}
		}()
		if err := update(rows); err != nil {
			return rows, err
		}
		payloads := make([]map[string]any, 0, len(prepared.items))
		for _, item := range prepared.items {
			if err := run.Err(); err != nil {
				return rows, err
			}
			payload, err := ApplyTemplate(item, nil)
			if err != nil {
				return rows, err
			}
			payloads = append(payloads, payload)
		}
		metadata, err := state.write(run, exportHash(owner), workbenchScopeFingerprint(ScopeLocalExport, configstore.TargetSettings{}), payloads)
		if err != nil {
			return rows, errors.New("独立私有文件生成失败，请检查私有目录后重新预览")
		}
		rows[0].Status, rows[0].Message, rows[0].Report = "succeeded", "独立私有账号文件已生成", localExportMetadataReport(metadata)
		return rows, nil
	})
}

func (s *Service) LocalExports(ctx context.Context, owner string) ([]ExportMetadata, error) {
	if owner == "" {
		return nil, ErrExportPreview
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	state, err := s.exportStorage()
	if err != nil {
		return nil, err
	}
	return state.list(exportHash(owner), workbenchScopeFingerprint(ScopeLocalExport, configstore.TargetSettings{}))
}

func (s *Service) DeleteLocalExport(ctx context.Context, owner, id string) error {
	if owner == "" || !validExportID(id) {
		return ErrExportPreview
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	state, err := s.exportStorage()
	if err != nil {
		return err
	}
	return state.remove(exportHash(owner), workbenchScopeFingerprint(ScopeLocalExport, configstore.TargetSettings{}), id)
}
