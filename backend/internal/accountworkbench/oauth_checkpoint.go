package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type oauthCheckpointStore interface {
	WorkbenchOAuthCheckpoints(context.Context, string, string) ([]configstore.WorkbenchOAuthCheckpoint, error)
	WorkbenchOAuthCheckpoint(context.Context, string, string, string) (configstore.WorkbenchOAuthCheckpoint, error)
	SaveWorkbenchOAuthCheckpoint(context.Context, configstore.WorkbenchOAuthCheckpoint) (configstore.WorkbenchOAuthCheckpoint, error)
	DeleteWorkbenchOAuthCheckpoint(context.Context, string, string, string, int64) error
}

type OAuthCheckpointView struct {
	Active             bool        `json:"active,omitempty"`
	ParentTaskID       string      `json:"parent_task_id,omitempty"`
	Automatic          bool        `json:"automatic,omitempty"`
	CheckpointRevision int64       `json:"checkpoint_revision,omitempty"`
	Scope              ExportScope `json:"scope"`
	ID                 string      `json:"id"`
	SourceTaskID       string      `json:"source_task_id"`
	TaskID             string      `json:"task_id,omitempty"`
	Status             string      `json:"status"`
	Stage              string      `json:"stage,omitempty"`
	Revision           int64       `json:"revision"`
	CreatedAt          string      `json:"created_at"`
	ExpiresAt          string      `json:"expires_at"`
	CanRestore         bool        `json:"can_restore"`
}

type OAuthCheckpointAction struct {
	CheckpointRevision int64       `json:"checkpoint_revision,omitempty"`
	Scope              ExportScope `json:"scope,omitempty"`
	Revision           int64       `json:"revision"`
	Confirmed          bool        `json:"confirmed"`
}

type oauthCheckpointPayload struct {
	SMSOriginTaskID   string                    `json:"sms_origin_task_id,omitempty"`
	ParentID          string                    `json:"parent_id,omitempty"`
	Scope             ExportScope               `json:"scope"`
	Options           browserlogin.OAuthOptions `json:"options"`
	Verifier          string                    `json:"verifier"`
	ExpectedEmail     string                    `json:"expected_email,omitempty"`
	ExpectedWorkspace string                    `json:"expected_workspace,omitempty"`
}

func checkpointView(record configstore.WorkbenchOAuthCheckpoint) OAuthCheckpointView {
	expires, _ := time.Parse(time.RFC3339Nano, record.ExpiresAt)
	return OAuthCheckpointView{ParentTaskID: record.ParentID, Automatic: record.Automatic, CheckpointRevision: record.WorkerRevision, Scope: ExportScope(record.Scope), ID: record.ID, SourceTaskID: record.SourceTaskID, TaskID: record.TaskID, Status: record.Status, Stage: record.Stage, Revision: record.Revision, CreatedAt: record.CreatedAt, ExpiresAt: record.ExpiresAt, CanRestore: record.Status == "ready" && time.Now().Before(expires)}
}

func (s *Service) checkpointStore() (oauthCheckpointStore, error) {
	store, ok := s.private.(oauthCheckpointStore)
	if !ok {
		return nil, errors.New("授权检查点私有存储尚未就绪")
	}
	return store, nil
}

func (s *Service) OAuthCheckpoints(ctx context.Context, owner string) ([]OAuthCheckpointView, error) {
	return s.OAuthCheckpointsScoped(ctx, owner, ScopeManaged)
}

func (s *Service) OAuthCheckpointsScoped(ctx context.Context, owner string, scope ExportScope) ([]OAuthCheckpointView, error) {
	if owner == "" {
		return nil, browserlogin.ErrSession
	}
	store, err := s.checkpointStore()
	if err != nil {
		return nil, err
	}
	ctx, target, err := s.bindWorkbenchScope(ctx, scope, nil)
	if err != nil {
		return nil, err
	}
	records, err := store.WorkbenchOAuthCheckpoints(ctx, exportHash(owner), workbenchScopeFingerprint(scope, target))
	if err != nil {
		return nil, err
	}
	views := make([]OAuthCheckpointView, 0, len(records))
	for _, record := range records {
		view, err := s.readCheckpointView(ctx, record)
		if err != nil {
			return nil, err
		}
		if record.ParentID != "" {
			view.CanRestore = false
		}
		views = append(views, view)
	}
	return views, nil
}

func (s *Service) SaveOAuthCheckpoint(ctx context.Context, owner, id string, confirmed bool) (OAuthCheckpointView, error) {
	if !confirmed {
		return OAuthCheckpointView{}, errors.New("请确认私有保存登录检查点并暂停当前授权")
	}
	store, err := s.checkpointStore()
	if err != nil {
		return OAuthCheckpointView{}, err
	}
	value, err := s.oauthSession(owner, id)
	if err != nil {
		return OAuthCheckpointView{}, err
	}
	value.op.Lock()
	defer value.op.Unlock()
	value.mu.Lock()
	browser, supported := value.browser.(browserlogin.OAuthCheckpointBrowser)
	if !supported || value.options.Recovery == nil || value.view.Status != "waiting" || value.checkpointSaved || value.profile != nil || value.batchID != "" {
		value.mu.Unlock()
		return OAuthCheckpointView{}, errors.New("当前授权不能保存检查点，请等待独立授权页面就绪")
	}
	value.assistPaused = true
	value.view.Status, value.view.Message = "checkpointing", "正在保存私有登录检查点"
	options, verifier := value.options, value.verifier
	expectedEmail, expectedWorkspace := value.expectedEmail, value.expectedWorkspace
	value.mu.Unlock()
	defer func() {
		value.mu.Lock()
		if value.view.Status == "checkpointing" {
			value.view.Status, value.view.Message = "waiting", "检查点未保存，请检查官方页面后人工继续"
		}
		value.mu.Unlock()
	}()
	if err := s.validateOAuthScope(ctx, value); err != nil {
		return OAuthCheckpointView{}, err
	}
	payload, err := json.Marshal(oauthCheckpointPayload{SMSOriginTaskID: value.smsOriginTaskID, Scope: value.scope, Options: options, Verifier: verifier, ExpectedEmail: expectedEmail, ExpectedWorkspace: expectedWorkspace})
	if err != nil {
		return OAuthCheckpointView{}, errors.New("授权检查点私有数据无法保存")
	}
	recordID, err := randomID()
	if err != nil {
		return OAuthCheckpointView{}, err
	}
	record := configstore.WorkbenchOAuthCheckpoint{Scope: string(value.scope), ID: recordID, OwnerHash: exportHash(owner), TargetFingerprint: workbenchScopeFingerprint(value.scope, value.target), TargetURL: value.target.BaseURL, SourceTaskID: id, Status: "saving", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), ExpiresAt: value.expires.UTC().Format(time.RFC3339Nano), Payload: payload}
	if value.automaticRecord != nil {
		record, err = store.WorkbenchOAuthCheckpoint(ctx, exportHash(owner), workbenchScopeFingerprint(value.scope, value.target), value.automaticRecord.ID)
		if err != nil || record.Status != "watching" {
			return OAuthCheckpointView{}, configstore.ErrWorkbenchOAuthCheckpoint
		}
		record.Status, record.Payload = "saving", payload
	}
	saved, err := store.SaveWorkbenchOAuthCheckpoint(ctx, record)
	if err != nil {
		return OAuthCheckpointView{}, errors.New("检查点意图保存失败，未暂停官方登录")
	}
	record = saved
	record.Payload = payload
	value.mu.Lock()
	value.checkpointSaved = true
	value.mu.Unlock()
	checkpoint, suspendErr := browser.SuspendOAuth(ctx)
	persist, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer stop()
	if suspendErr != nil {
		record.Status, record.Payload = "failed", nil
		_, _ = store.SaveWorkbenchOAuthCheckpoint(persist, record)
		if errors.Is(suspendErr, browserlogin.ErrOAuthCheckpointUnsafe) {
			value.mu.Lock()
			value.checkpointSaved = false
			value.mu.Unlock()
		} else {
			s.stopCheckpointSession(value, "", true)
		}
		return OAuthCheckpointView{}, browserlogin.ErrOAuthCheckpointUnsafe
	}
	// Suspend closes the original browser before returning. Preserve any pending
	// SMS order for the separately persisted operator-review ledger.
	if !validOAuthCheckpointReceipt(checkpoint, options) {
		s.stopCheckpointSession(value, "", true)
		record.Status, record.Payload = "failed", nil
		_, _ = store.SaveWorkbenchOAuthCheckpoint(persist, record)
		return OAuthCheckpointView{}, errors.New("浏览器检查点绑定不一致，当前授权已暂停，请重新授权")
	}
	record.WorkerID, record.WorkerLease, record.Stage, record.Status = checkpoint.ID, checkpoint.Lease, checkpoint.Stage, "ready"
	record.WorkerRevision = checkpoint.Revision
	if err := s.validateOAuthScope(persist, value); err != nil {
		s.stopCheckpointSession(value, "", true)
		record.Status, record.Payload = "failed", nil
		_, _ = store.SaveWorkbenchOAuthCheckpoint(persist, record)
		return OAuthCheckpointView{}, err
	}
	saved, err = store.SaveWorkbenchOAuthCheckpoint(persist, record)
	if err != nil {
		record.Status, record.Payload = "failed", nil
		_, _ = store.SaveWorkbenchOAuthCheckpoint(persist, record)
		s.stopCheckpointSession(value, "", true)
		return OAuthCheckpointView{}, errors.New("浏览器已暂停但检查点结果未保存，请核对任务后重新授权")
	}
	s.stopCheckpointSession(value, record.ID, true)
	return checkpointView(saved), nil
}

func (s *Service) stopCheckpointSession(value *oauthSession, id string, saved bool) {
	value.mu.Lock()
	defer value.mu.Unlock()
	value.checkpointSaved, value.checkpointID = saved, id
	value.view.Status, value.view.Message = "cancelled", "当前授权已暂停"
	if value.cancel != nil {
		value.cancel()
	}
}

func validOAuthCheckpointReceipt(checkpoint browserlogin.OAuthCheckpoint, options browserlogin.OAuthOptions) bool {
	if options.Recovery == nil || len(checkpoint.ID) != 48 || strings.Trim(checkpoint.ID, "0123456789abcdef") != "" || checkpoint.Owner != options.Recovery.Owner || checkpoint.Lease != options.Recovery.Lease || !checkpoint.ExpiresAt.Equal(options.Recovery.ExpiresAt) {
		return false
	}
	if options.Recovery.AutoCheckpoint && (checkpoint.ID != options.Recovery.CheckpointID || checkpoint.Revision < 1) {
		return false
	}
	switch checkpoint.Stage {
	case "email", "password", "email_code", "totp_code", "phone", "sms_code", "workspace", "manual":
		return true
	}
	return false
}
