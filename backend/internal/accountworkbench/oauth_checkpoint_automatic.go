package accountworkbench

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func randomWorkerCheckpointID() (string, error) {
	value := make([]byte, 24)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

// Persist the exact transaction before opening a browser that can save it.
func (s *Service) prepareAutomaticCheckpoint(ctx context.Context, value *oauthSession) error {
	store, err := s.checkpointStore()
	if err != nil {
		return err
	}
	options := value.options
	if options.Recovery == nil || !options.Recovery.AutoCheckpoint {
		return errors.New("自动检查点缺少授权事务绑定")
	}
	payload, err := json.Marshal(oauthCheckpointPayload{SMSOriginTaskID: value.smsOriginTaskID, ParentID: value.batchID, Scope: value.scope, Options: options, Verifier: value.verifier, ExpectedEmail: value.expectedEmail, ExpectedWorkspace: value.expectedWorkspace})
	if err != nil {
		return errors.New("自动检查点授权事务无法保存")
	}
	id, err := randomID()
	if err != nil {
		return err
	}
	record := configstore.WorkbenchOAuthCheckpoint{ParentID: value.batchID, Scope: string(value.scope), ID: id, OwnerHash: exportHash(value.owner), TargetFingerprint: workbenchScopeFingerprint(value.scope, value.target), TargetURL: value.target.BaseURL, SourceTaskID: value.view.TaskID, Status: "watching", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), ExpiresAt: value.expires.UTC().Format(time.RFC3339Nano), WorkerID: options.Recovery.CheckpointID, WorkerLease: options.Recovery.Lease, Automatic: true, Payload: payload}
	saved, err := store.SaveWorkbenchOAuthCheckpoint(ctx, record)
	if err != nil {
		return errors.New("自动检查点授权事务保存失败，未启动登录")
	}
	value.mu.Lock()
	value.automaticRecord = &saved
	value.view.RecoveryEnabled, value.view.CheckpointID = true, saved.ID
	value.mu.Unlock()
	return nil
}

func (s *Service) readCheckpointView(ctx context.Context, record configstore.WorkbenchOAuthCheckpoint) (OAuthCheckpointView, error) {
	view := checkpointView(record)
	if !record.Automatic || record.Status != "watching" && record.Status != "ready" {
		return view, nil
	}
	view.Active = s.activeCheckpointSession(record) != nil
	view.CanRestore, view.CheckpointRevision = view.Active, 0
	reader, ok := s.oauthFactory.(browserlogin.OAuthCheckpointReader)
	if !ok {
		return view, nil
	}
	checkpoint, err := reader.ReadOAuthCheckpoint(ctx, browserlogin.OAuthCheckpointRef{ID: record.WorkerID, Owner: record.OwnerHash, Lease: record.WorkerLease})
	if errors.Is(err, browserlogin.ErrOAuthCheckpoint) {
		return view, nil
	}
	if err != nil {
		if view.Active {
			return view, nil
		}
		return view, errors.New("自动登录检查点状态读取失败，请检查 browser 服务后重试")
	}
	expires, err := time.Parse(time.RFC3339Nano, record.ExpiresAt)
	options := browserlogin.OAuthOptions{Recovery: &browserlogin.OAuthRecoveryBinding{Owner: record.OwnerHash, Lease: record.WorkerLease, ExpiresAt: expires, AutoCheckpoint: true, CheckpointID: record.WorkerID}}
	if err != nil || !validOAuthCheckpointReceipt(checkpoint, options) {
		return view, errors.New("自动登录检查点绑定无效，请重新授权")
	}
	view.Stage, view.CheckpointRevision, view.CanRestore = checkpoint.Stage, checkpoint.Revision, time.Now().Before(expires)
	return view, nil
}

func (s *Service) activeCheckpointSession(record configstore.WorkbenchOAuthCheckpoint) *oauthSession {
	if !record.Automatic || record.Status != "watching" || record.ParentID != "" {
		return nil
	}
	s.oauthMu.Lock()
	value := s.oauthSessions[record.SourceTaskID]
	s.oauthMu.Unlock()
	if value == nil || exportHash(value.owner) != record.OwnerHash || string(value.scope) != record.Scope || workbenchScopeFingerprint(value.scope, value.target) != record.TargetFingerprint || time.Now().After(value.expires) {
		return nil
	}
	value.mu.Lock()
	defer value.mu.Unlock()
	if value.batchID != "" || !value.view.RecoveryEnabled || value.view.CheckpointID != record.ID || value.explicitCancel {
		return nil
	}
	switch value.view.Status {
	case "starting", "waiting", "verifying":
		return value
	default:
		return nil
	}
}

func (s *Service) discardAutomaticCheckpoint(value *oauthSession) error {
	if value.automaticRecord == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	store, err := s.checkpointStore()
	if err != nil {
		return err
	}
	reference := value.automaticRecord
	record, err := store.WorkbenchOAuthCheckpoint(ctx, reference.OwnerHash, reference.TargetFingerprint, reference.ID)
	if errors.Is(err, configstore.ErrWorkbenchOAuthCheckpoint) {
		return nil
	}
	if err != nil {
		return err
	}
	// A claimed source belongs to its recovery task, which has a new lease.
	if record.Status == "restoring" || record.Status == "restored" {
		return nil
	}
	if record.Status != "deleting" {
		record.Status, record.Payload = "deleting", nil
		record, err = store.SaveWorkbenchOAuthCheckpoint(ctx, record)
		if err != nil {
			return err
		}
	}
	factory, ok := s.oauthFactory.(browserlogin.OAuthRecoveryFactory)
	if !ok {
		return errors.New("浏览器自动检查点清理服务尚未就绪")
	}
	err = factory.DeleteOAuthCheckpoint(ctx, browserlogin.OAuthCheckpointRef{ID: record.WorkerID, Owner: record.OwnerHash, Lease: record.WorkerLease})
	if err != nil && !errors.Is(err, browserlogin.ErrOAuthCheckpoint) {
		return errors.New("自动检查点清理未完成，请检查 browser 服务后重试")
	}
	return store.DeleteWorkbenchOAuthCheckpoint(ctx, record.OwnerHash, record.TargetFingerprint, record.ID, record.Revision)
}

func (s *Service) closeOAuthBrowser(value *oauthSession, browser browserlogin.OAuthBrowser) {
	value.mu.Lock()
	preserve := value.automaticRecord != nil && !value.explicitCancel && !value.exchangeStarted && value.view.Status != "authorized" && time.Now().Before(value.expires)
	if preserve {
		value.checkpointSaved = true
	}
	value.mu.Unlock()
	if preserve {
		if closer, ok := browser.(browserlogin.OAuthCheckpointPreserver); ok {
			closer.ClosePreservingOAuthCheckpoint()
			return
		}
	}
	browser.Close()
	_ = s.discardAutomaticCheckpoint(value)
}

func (s *Service) revokeCheckpointForExchange(value *oauthSession) error {
	value.op.Lock()
	defer value.op.Unlock()
	// An uncertain token response must never make the same OAuth transaction recoverable.
	if value.automaticRecord != nil {
		value.mu.Lock()
		browser := value.browser
		value.mu.Unlock()
		if browser != nil {
			browser.Close()
		}
	}
	if err := s.discardAutomaticCheckpoint(value); err != nil {
		return err
	}
	value.mu.Lock()
	value.exchangeStarted = true
	value.view.RecoveryEnabled, value.view.CheckpointID = false, ""
	value.mu.Unlock()
	return nil
}
