package accountworkbench

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

type SecurityCheckpointStartInput struct {
	OAuthCheckpointAction
	Operation string `json:"operation"`
	Password  string `json:"password,omitempty"`
}

type securityCheckpoint struct {
	mu                sync.Mutex
	record            configstore.WorkbenchOAuthCheckpoint
	options           browserlogin.SecurityCheckpointOptions
	expectedEmail     string
	expectedWorkspace string
	smsOriginTaskID   string
	claimed           bool
	completed         bool
}

func (s *Service) StartCheckpointSecurity(ctx context.Context, owner, id string, input SecurityCheckpointStartInput) (SecurityView, error) {
	s.cleanupMu.RLock()
	defer s.cleanupMu.RUnlock()
	if owner == "" || !validExportID(id) || input.Revision < 1 || !input.Confirmed {
		return SecurityView{}, errors.New("请确认将所选版本的暂停检查点转入账号安全设置")
	}
	if err := validateSecurityOperation(input.Operation, input.Password); err != nil {
		return SecurityView{}, err
	}
	if _, ok := s.securityFactory.(browserlogin.SecurityCheckpointFactory); !ok {
		return SecurityView{}, errors.New("安全浏览器尚不支持登录检查点，请更新 browser 服务")
	}
	s.securityMu.Lock()
	storage := s.securityStorage
	s.securityMu.Unlock()
	if storage == nil || !storage.available() {
		return SecurityView{}, ErrSecurityStorage
	}
	origin, err := s.prepareSecurityCheckpointOrigin(ctx, owner, id, input.OAuthCheckpointAction)
	if err != nil {
		return SecurityView{}, err
	}
	return s.launchSecurity(ctx, owner, SecurityStartInput{Scope: origin.scope, Operation: input.Operation, Password: input.Password, Confirmed: true}, origin, storage, "", nil)
}

func (s *Service) prepareSecurityCheckpointOrigin(ctx context.Context, owner, id string, input OAuthCheckpointAction) (securityOrigin, error) {
	scope, err := normalizeWorkbenchScope(input.Scope)
	if err != nil {
		return securityOrigin{}, err
	}
	ctx, target, err := s.bindWorkbenchScope(ctx, scope, nil)
	if err != nil {
		return securityOrigin{}, err
	}
	store, err := s.checkpointStore()
	if err != nil {
		return securityOrigin{}, err
	}
	record, err := store.WorkbenchOAuthCheckpoint(ctx, exportHash(owner), workbenchScopeFingerprint(scope, target), id)
	if err != nil || record.Revision != input.Revision || record.Scope != string(scope) || record.ParentID != "" || record.Status != "ready" && !(record.Automatic && record.Status == "watching") {
		return securityOrigin{}, configstore.ErrWorkbenchOAuthCheckpoint
	}
	if s.activeCheckpointSession(record) != nil {
		return securityOrigin{}, errors.New("请先保存并暂停当前授权，再转入安全设置")
	}
	if record.Automatic {
		current, err := s.readCheckpointView(ctx, record)
		if err != nil {
			return securityOrigin{}, err
		}
		if !current.CanRestore || input.CheckpointRevision < 1 || current.CheckpointRevision != input.CheckpointRevision {
			return securityOrigin{}, configstore.ErrWorkbenchOAuthCheckpoint
		}
		record.WorkerRevision, record.Stage = current.CheckpointRevision, current.Stage
	} else if input.CheckpointRevision != 0 && input.CheckpointRevision != record.WorkerRevision {
		return securityOrigin{}, configstore.ErrWorkbenchOAuthCheckpoint
	}
	payload, err := validateCheckpointPayload(record)
	if err != nil {
		return securityOrigin{}, err
	}
	expires, err := time.Parse(time.RFC3339Nano, record.ExpiresAt)
	if err != nil || !time.Now().Before(expires) {
		return securityOrigin{}, configstore.ErrWorkbenchOAuthCheckpoint
	}
	sessionID, err := randomWorkerCheckpointID()
	if err != nil {
		return securityOrigin{}, err
	}
	options := browserlogin.SecurityCheckpointOptions{SessionID: sessionID, Options: payload.Options, Checkpoint: browserlogin.OAuthCheckpoint{ID: record.WorkerID, Owner: record.OwnerHash, Lease: record.WorkerLease, Revision: record.WorkerRevision, Stage: record.Stage, ExpiresAt: expires}}
	if err := options.Validate(); err != nil {
		return securityOrigin{}, err
	}
	smsOriginTaskID := payload.SMSOriginTaskID
	if smsOriginTaskID == "" {
		smsOriginTaskID = record.SourceTaskID
	}
	checkpoint := &securityCheckpoint{record: record, options: options, expectedEmail: payload.ExpectedEmail, expectedWorkspace: payload.ExpectedWorkspace, smsOriginTaskID: smsOriginTaskID}
	return securityOrigin{scope: scope, target: target, checkpoint: checkpoint}, nil
}

func (s *Service) claimSecurityCheckpoint(ctx context.Context, value *securitySession) error {
	if value.checkpoint == nil {
		return nil
	}
	if _, _, err := s.bindWorkbenchScope(ctx, value.view.Scope, &value.target); err != nil {
		return err
	}
	store, err := s.checkpointStore()
	if err != nil {
		return err
	}
	value.checkpoint.mu.Lock()
	defer value.checkpoint.mu.Unlock()
	record := value.checkpoint.record
	record.Status, record.TaskID = "restoring", value.view.TaskID
	saved, err := store.SaveWorkbenchOAuthCheckpoint(ctx, record)
	if err != nil {
		return err
	}
	value.checkpoint.record, value.checkpoint.claimed = saved, true
	return nil
}

func (s *Service) completeSecurityCheckpoint(ctx context.Context, value *securitySession, succeeded bool) error {
	checkpoint := value.checkpoint
	if checkpoint == nil {
		return nil
	}
	checkpoint.mu.Lock()
	defer checkpoint.mu.Unlock()
	if !checkpoint.claimed || checkpoint.completed {
		return nil
	}
	persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	store, err := s.checkpointStore()
	if err != nil {
		return err
	}
	record := checkpoint.record
	record.Status, record.Payload = "failed", nil
	if succeeded {
		record.Status = "restored"
	}
	saved, err := store.SaveWorkbenchOAuthCheckpoint(persist, record)
	if err != nil {
		return err
	}
	checkpoint.record, checkpoint.completed = saved, true
	return nil
}

func (s *Service) validateSecurityCheckpoint(ctx context.Context, value *securitySession) error {
	if _, _, err := s.bindWorkbenchScope(ctx, value.view.Scope, &value.target); err != nil {
		return err
	}
	store, err := s.checkpointStore()
	if err != nil {
		return err
	}
	checkpoint := value.checkpoint
	checkpoint.mu.Lock()
	wanted := checkpoint.record
	checkpoint.mu.Unlock()
	record, err := store.WorkbenchOAuthCheckpoint(ctx, exportHash(value.owner), workbenchScopeFingerprint(value.view.Scope, value.target), value.view.SourceCheckpointID)
	if err != nil || record.Revision != wanted.Revision || record.TaskID != value.view.TaskID || record.Status != "restoring" && record.Status != "restored" || !time.Now().Before(value.expires) {
		return configstore.ErrWorkbenchOAuthCheckpoint
	}
	return nil
}

func (s *Service) securityCheckpointGuard(ctx context.Context, value *securitySession) (context.Context, func() error, error) {
	resource := "workbench-security-checkpoint/" + value.view.SourceCheckpointID
	var guarded context.Context
	var release func() error
	var err error
	if value.view.Scope == ScopeLocalExport {
		guarded, release, err = mutationguard.Acquire(ctx, s.repository, resource)
	} else {
		guarded, release, err = targetguard.Acquire(targetguard.Expect(ctx, value.target), s.repository, resource)
	}
	if err != nil {
		return nil, nil, err
	}
	if err := s.validateSecurityCheckpoint(guarded, value); err != nil {
		_ = release()
		return nil, nil, err
	}
	return guarded, release, nil
}

func (s *Service) openSecurityBrowser(ctx context.Context, value *securitySession, proxyURL string) (browserlogin.SecurityBrowser, error) {
	if value.checkpoint == nil {
		return s.securityFactory.OpenSecurity(ctx, browserlogin.SecurityOptions{Email: value.expected.Email, ProxyURL: proxyURL})
	}
	factory, ok := s.securityFactory.(browserlogin.SecurityCheckpointFactory)
	if !ok {
		return nil, browserlogin.ErrOAuthCheckpoint
	}
	browser, err := factory.OpenSecurityFromCheckpoint(ctx, value.checkpoint.options)
	if browser != nil {
		if _, ok := browser.(browserlogin.SecurityIdentityConfirmation); !ok && err == nil {
			err = browserlogin.ErrSecurityConfirmation
		}
	}
	if saveErr := s.completeSecurityCheckpoint(ctx, value, err == nil && browser != nil); saveErr != nil && err == nil {
		err = saveErr
	}
	return browser, err
}
