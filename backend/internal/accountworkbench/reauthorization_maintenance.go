package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

func (s *Service) validateMaintenanceReauthorization(ctx context.Context, target configstore.TargetSettings, revision int64) error {
	store, ok := s.private.(maintenanceStore)
	if !ok {
		return errors.New("自动维护配置存储尚未就绪")
	}
	config, err := store.WorkbenchMaintenance(ctx, target.BaseURL)
	if err != nil {
		return err
	}
	if !config.Enabled || !config.ReauthorizeWithProfiles || revision < 1 || config.Revision != revision {
		return errors.New("自动重新授权尚未开启或维护配置已变化，请重新确认")
	}
	return nil
}

// The caller supplies the live console owner that enabled maintenance, allowing
// manual takeover of verification challenges. Automatic relogin never buys SMS
// or reuses a previous OAuth code, session, or phone allocation.
func (s *Service) StartMaintenanceReauthorization(ctx context.Context, owner string, accountIDs []string, revision int64) (OAuthBatchView, error) {
	ctx, err := targetguard.Pin(ctx, s.private)
	if err != nil {
		return OAuthBatchView{}, err
	}
	target, err := targetguard.Settings(ctx, s.private)
	if err != nil {
		return OAuthBatchView{}, err
	}
	if err := s.validateMaintenanceReauthorization(ctx, target, revision); err != nil {
		return OAuthBatchView{}, err
	}
	delegated, err := s.currentMaintenanceOwner(ctx, target, revision)
	if err != nil {
		return OAuthBatchView{}, err
	}
	if delegated.owner != owner {
		return OAuthBatchView{}, errors.New("自动重新授权仅允许绑定的登录会话接管")
	}
	config, err := s.private.(maintenanceStore).WorkbenchMaintenance(ctx, target.BaseURL)
	if err != nil || config.Revision != revision || !config.Enabled || !config.ReauthorizeWithProfiles {
		return OAuthBatchView{}, errors.New("自动维护配置已变化，请重新确认")
	}
	preview, err := s.previewReauthorization(ctx, owner, ReauthorizationPreviewInput{AccountIDs: accountIDs, FreshLogin: true}, true)
	if err != nil {
		return OAuthBatchView{}, err
	}
	client, err := s.clientFor(target)
	if err != nil {
		return OAuthBatchView{}, err
	}
	snapshots := map[string]json.RawMessage{}
	for _, item := range preview.Items {
		account, err := client.Account(ctx, item.AccountID)
		if err != nil {
			s.DeleteOAuthBatchPreview(owner, preview.ID)
			return OAuthBatchView{}, publicError(err)
		}
		snapshots[item.AccountID] = executionSnapshot(account)
	}
	s.batches.mu.Lock()
	prepared := s.batches.previews[preview.ID]
	if prepared == nil || prepared.owner != owner {
		s.batches.mu.Unlock()
		return OAuthBatchView{}, ErrPreview
	}
	for i := range prepared.inputs {
		prepared.inputs[i].SMS = nil
		prepared.view.Items[i].SMSProvider = ""
		prepared.profiles[i].maintenanceRevision = revision
		prepared.profiles[i].maintenanceOwner = owner
		prepared.profiles[i].uploadSnapshot = snapshots[prepared.profiles[i].identity.accountID]
		prepared.profiles[i].uploadMaintenance = &configstore.WorkbenchExecutionMaintenance{Revision: config.Revision, Model: config.Model, CheckAfterImport: config.CheckAfterRepair, CooldownMinutes: config.CooldownMinutes}
	}
	s.batches.mu.Unlock()
	view, err := s.StartOAuthBatch(ctx, owner, preview.ID, true)
	if err != nil {
		s.DeleteOAuthBatchPreview(owner, preview.ID)
		return view, err
	}
	s.maintenanceMu.Lock()
	if s.maintenanceOwner == delegated {
		delegated.batchID = view.ID
	}
	s.maintenanceMu.Unlock()
	s.batches.mu.Lock()
	job := s.batches.active[view.ID]
	s.batches.mu.Unlock()
	if job != nil {
		stop := context.AfterFunc(delegated.ctx, func() { _ = s.CancelOAuthBatch(owner, view.ID) })
		go func() { <-job.done; stop() }()
	}
	return view, err
}
