package accountworkbench

import (
	"context"
	"errors"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func (s *Service) maintainReauthorization(ctx context.Context, target configstore.TargetSettings, config configstore.WorkbenchMaintenance, rows []ResultItem, pendingAccounts map[string]bool, update func([]ResultItem) error) (result []ResultItem, resultErr error) {
	defer func() {
		if resultErr == nil {
			return
		}
		for i := range result {
			if result[i].Status == "waiting_input" || result[i].Status == "running" || result[i].Status == "queued" {
				result[i].Status, result[i].Message = "review", publicError(resultErr).Error()
			}
		}
	}()
	ids := []string{}
	for _, row := range rows {
		if row.Status == "review" && !pendingAccounts[row.AccountID] {
			ids = append(ids, row.AccountID)
		}
	}
	if len(ids) == 0 {
		return rows, nil
	}
	owner, err := s.currentMaintenanceOwner(ctx, target, config.Revision)
	if err != nil {
		for i := range rows {
			if rows[i].Status == "review" {
				rows[i].Message = "账号需要重新授权，请在当前登录会话重新启用自动维护"
			}
		}
		return rows, nil
	}
	candidates, err := s.ReauthorizationCandidates(ctx, ids)
	if err != nil {
		return rows, err
	}
	ids = ids[:0]
	selected := map[string]bool{}
	for _, candidate := range candidates {
		ids = append(ids, candidate.AccountID)
		selected[candidate.AccountID] = true
	}
	if len(ids) == 0 {
		return rows, nil
	}
	bound, cancel := context.WithCancel(ctx)
	stopOwner := context.AfterFunc(owner.ctx, cancel)
	defer stopOwner()
	defer cancel()
	if owner.ctx.Err() != nil {
		cancel()
	}
	view, err := s.StartMaintenanceReauthorization(bound, owner.owner, ids, config.Revision)
	if err != nil {
		return rows, err
	}
	defer func() {
		s.maintenanceMu.Lock()
		if s.maintenanceOwner == owner {
			owner.batchID, owner.importID = "", ""
		}
		s.maintenanceMu.Unlock()
	}()
	job, err := s.oauthBatch(owner.owner, view.ID)
	if err != nil {
		return rows, err
	}
	stopBatch := context.AfterFunc(bound, func() { _ = s.CancelOAuthBatch(owner.owner, view.ID) })
	defer stopBatch()
	for i := range rows {
		if selected[rows[i].AccountID] {
			rows[i].Status, rows[i].Message = "waiting_input", "正在重新授权，官方验证可在当前登录会话接管"
		}
	}
	if err := update(rows); err != nil {
		_ = s.CancelOAuthBatch(owner.owner, view.ID)
		<-job.done
		return rows, err
	}
	<-job.done
	if bound.Err() != nil {
		return rows, bound.Err()
	}
	view, err = s.ReadOAuthBatch(owner.owner, view.ID)
	if err != nil {
		return rows, err
	}
	for _, authorized := range view.Items {
		for i := range rows {
			if rows[i].AccountID != authorized.AccountID {
				continue
			}
			rows[i].Status, rows[i].Message = "review", authorized.Message
			if authorized.Status == "succeeded" {
				rows[i].Status, rows[i].Message = "running", "重新授权成功，等待更新原账号凭据"
			}
		}
	}
	if err := update(rows); err != nil {
		return rows, err
	}
	if view.Available == 0 {
		return rows, nil
	}
	records, err := s.maintenanceBatchUploads(bound, job)
	if err != nil {
		return rows, err
	}
	for _, record := range records {
		imported := s.retryMaintenanceUpload(bound, target, config, owner, &record, 0)
		for i := range rows {
			if rows[i].AccountID == imported.AccountID {
				rows[i].Status, rows[i].Message, rows[i].Report = imported.Status, imported.Message, imported.Report
			}
		}
		if err := update(rows); err != nil {
			return rows, err
		}
		if imported.Status == "failed" {
			resultErr = errors.New(imported.Message)
		}
	}
	if bound.Err() != nil {
		return rows, bound.Err()
	}
	return rows, resultErr
}
