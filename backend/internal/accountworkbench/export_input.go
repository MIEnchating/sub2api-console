package accountworkbench

import (
	"context"
	"errors"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

// ExportInput consumes a normal parsed or authorized input preview. It applies
// the reviewed configuration and writes a private file without creating or
// modifying a managed account. Retry previews cannot be converted this way.
func (s *Service) ExportInput(ctx context.Context, owner, id string, confirmed bool) (taskstore.Task, error) {
	if !confirmed {
		return taskstore.Task{}, errors.New("请先确认私有转换范围")
	}
	state, err := s.exportStorage()
	if err != nil {
		return taskstore.Task{}, err
	}
	s.mu.Lock()
	prepared := s.previews[id]
	if prepared == nil || prepared.owner != owner || time.Now().After(prepared.expires) {
		s.mu.Unlock()
		return taskstore.Task{}, ErrPreview
	}
	if prepared.retry != nil {
		s.mu.Unlock()
		return taskstore.Task{}, errors.New("重新处理预览不能用于私有转换，请重新输入账号凭据")
	}
	delete(s.previews, id)
	s.mu.Unlock()
	if prepared.view.Scope == ScopeLocalExport {
		return s.exportLocalInput(ctx, owner, prepared, state)
	}
	if _, err := targetguard.Pin(targetguard.Expect(ctx, prepared.target), s.private); err != nil {
		return taskstore.Task{}, err
	}
	return s.enqueue(ctx, "account-workbench-convert", "等待生成私有账号文件", func(run context.Context, update func([]ResultItem) error) (rows []ResultItem, runErr error) {
		defer func() { prepared.items = nil }()
		rows = []ResultItem{{Index: 0, Name: "私有账号文件", Status: "running", Message: "正在核对模板并转换账号"}}
		defer func() {
			if runErr != nil {
				rows[0].Status, rows[0].Message = "failed", publicError(runErr).Error()
				if errors.Is(runErr, context.Canceled) {
					rows[0].Status, rows[0].Message = "cancelled", "私有转换已取消"
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
		payloads := make([]map[string]any, 0, len(prepared.items))
		for index, item := range prepared.items {
			if err := guarded.Err(); err != nil {
				return rows, err
			}
			template := prepared.templates[index]
			if template != nil {
				current, err := s.private.WorkbenchTemplate(guarded, prepared.target.BaseURL, template.ID)
				if err != nil || current.Revision != template.Revision {
					return rows, errors.New("模板已变化，请重新预览私有转换范围")
				}
			}
			payload, err := ApplyTemplate(item, template)
			if err != nil {
				return rows, err
			}
			payloads = append(payloads, payload)
		}
		if _, err := targetguard.Pin(guarded, s.private); err != nil {
			return rows, err
		}
		metadata, err := state.write(guarded, exportHash(owner), exportTargetHash(prepared.target), payloads)
		if err != nil {
			return rows, errors.New("私有文件生成失败，请核对服务器目录后重新预览")
		}
		rows[0].Status, rows[0].Message = "succeeded", "私有账号文件已生成"
		rows[0].Report = exportMetadataReport(metadata)
		return rows, nil
	})
}
