package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func (s *Service) documents(ctx context.Context, prefix string, visit func(configstore.WorkbenchDocumentRecord) error) error {
	after := ""
	for {
		rows, err := s.private.WorkbenchDocumentPage(ctx, prefix, after, 20)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if err = visit(row); err != nil {
				return err
			}
			after = row.ID
		}
		if len(rows) < 20 {
			return nil
		}
	}
}

// Recover runs before serving requests. Interrupted writes are never replayed.
func (s *Service) Recover(ctx context.Context) error {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if err := s.cleanup(ctx); err != nil {
		return err
	}
	if err := s.cleanupMaintenance(ctx, true); err != nil {
		return err
	}
	return s.documents(ctx, "run:", func(record configstore.WorkbenchDocumentRecord) error {
		value, err := decodeRun(record.Payload)
		if err != nil {
			return err
		}
		value.Public.Revision = record.Revision
		if value.Public.Status != "running" && value.Public.Status != "queued" {
			return nil
		}
		value.Public.Status = "interrupted"
		for i := range value.Public.Items {
			row := &value.Public.Items[i]
			row.BrowserReady = false
			if row.Status != "completed" && row.Status != "exported" && row.Status != "review" {
				row.Status = "interrupted"
				row.Message = "服务重启，任务已停止；未提交项可继续处理"
			}
		}
		value.ManualIDs = nil
		return s.saveRun(ctx, &value)
	})
}

func (s *Service) Cleanup(ctx context.Context) error {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return s.cleanup(ctx)
}

func (s *Service) cleanup(ctx context.Context) error {
	now := time.Now().UTC()
	if err := s.documents(ctx, "preview:", func(record configstore.WorkbenchDocumentRecord) error {
		var value privatePreview
		if json.Unmarshal(record.Payload, &value) != nil {
			return errors.New("预览清理记录无效")
		}
		active, err := s.private.ActiveSessionOwner(ctx, value.Owner, now)
		if err != nil {
			return err
		}
		if active && value.Public.ExpiresAt.After(now) {
			return nil
		}
		err = s.private.DeleteWorkbenchDocument(ctx, record.ID, record.Revision)
		if errors.Is(err, configstore.ErrWorkbenchVersion) {
			return nil
		}
		return err
	}); err != nil {
		return err
	}
	owners := map[string]bool{}
	err := s.documents(ctx, "run:", func(record configstore.WorkbenchDocumentRecord) error {
		value, err := decodeRun(record.Payload)
		if err != nil {
			return err
		}
		value.Public.Revision = record.Revision
		owners[value.Owner] = true
		live, err := s.private.ActiveSessionOwner(ctx, value.Owner, now)
		if err != nil {
			return err
		}
		if live && value.Public.ExpiresAt.After(now) {
			return nil
		}
		s.activeMu.Lock()
		active := s.active[value.Public.ID]
		s.activeMu.Unlock()
		if active != nil {
			s.runner.CancelTask(active.taskID)
			// The worker must finish saving any received rotated token first.
			return nil
		}
		if !live {
			return s.deleteRecord(ctx, &value)
		}
		if value.Public.Action == "maintenance" {
			return s.deleteRecord(ctx, &value)
		}
		if err = s.deleteArtifacts(ctx, value.Owner, value.Public.ID); err != nil {
			return err
		}
		if value.SecretsCleared {
			return nil
		}
		value.Items = nil
		value.Exports = nil
		value.Target = configstore.TargetSettings{}
		value.Settings = PreviewInput{}
		value.AccountVersions = nil
		value.ManualIDs = nil
		value.SecretsCleared = true
		for i := range value.Public.Items {
			value.Public.Items[i].BrowserReady = false
		}
		return s.saveRun(ctx, &value)
	})
	if err != nil {
		return err
	}
	for owner := range owners {
		if err = s.pruneRecords(ctx, owner); err != nil {
			return err
		}
	}
	return s.cleanupMaintenance(ctx, false)
}
