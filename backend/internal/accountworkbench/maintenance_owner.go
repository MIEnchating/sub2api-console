package accountworkbench

import (
	"context"
	"errors"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

type maintenanceSessionStore interface {
	ActiveSessionOwner(context.Context, string, time.Time) (bool, error)
	SessionOwnerExpires(context.Context, string) (time.Time, error)
	SessionOwnerChanges() <-chan struct{}
}

type maintenanceOwner struct {
	owner    string
	revision int64
	target   configstore.TargetSettings
	ctx      context.Context
	cancel   context.CancelFunc
	batchID  string
	importID string
}

type MaintenanceAuthorizationView struct {
	Attached                 bool   `json:"attached"`
	CurrentReauthorizationID string `json:"current_reauthorization_id,omitempty"`
	CurrentImportTaskID      string `json:"current_import_task_id,omitempty"`
}

func (s *Service) BindMaintenanceOwner(ctx context.Context, owner string, revision int64) error {
	ctx, err := targetguard.Pin(ctx, s.private)
	if err != nil {
		return err
	}
	target, err := targetguard.Settings(ctx, s.private)
	if err != nil {
		return err
	}
	store, ok := s.private.(maintenanceStore)
	if !ok {
		return errors.New("自动维护存储尚未就绪")
	}
	config, err := store.WorkbenchMaintenance(ctx, target.BaseURL)
	if err != nil {
		return err
	}
	if config.Revision != revision {
		return errors.New("自动维护配置已变化，请刷新后重新启用")
	}
	if !config.Enabled || !config.ReauthorizeWithProfiles {
		s.detachMaintenanceOwner()
		return nil
	}
	sessions, ok := s.private.(maintenanceSessionStore)
	if !ok {
		return browserlogin.ErrSession
	}
	changes := sessions.SessionOwnerChanges()
	expires, err := sessions.SessionOwnerExpires(ctx, owner)
	if err != nil {
		return err
	}
	if !expires.After(time.Now()) {
		return browserlogin.ErrSession
	}
	bound, cancel := context.WithDeadline(context.Background(), expires)
	value := &maintenanceOwner{owner: owner, revision: revision, target: target, ctx: bound, cancel: cancel}
	s.maintenanceMu.Lock()
	previous := s.maintenanceOwner
	s.maintenanceOwner = value
	s.maintenanceMu.Unlock()
	if previous != nil {
		previous.cancel()
	}
	go func() {
		defer cancel()
		for {
			select {
			case <-bound.Done():
				return
			case <-changes:
			}
			changes = sessions.SessionOwnerChanges()
			active, err := sessions.ActiveSessionOwner(bound, owner, time.Now())
			if err != nil || !active {
				return
			}
		}
	}()
	return nil
}

func (s *Service) detachMaintenanceOwner() {
	s.maintenanceMu.Lock()
	value := s.maintenanceOwner
	s.maintenanceOwner = nil
	s.maintenanceMu.Unlock()
	if value != nil {
		value.cancel()
	}
}

func (s *Service) currentMaintenanceOwner(ctx context.Context, target configstore.TargetSettings, revision int64) (*maintenanceOwner, error) {
	s.maintenanceMu.Lock()
	value := s.maintenanceOwner
	s.maintenanceMu.Unlock()
	if value == nil || value.ctx.Err() != nil || value.revision != revision || executionTargetFingerprint(value.target) != executionTargetFingerprint(target) {
		return nil, errors.New("自动重新授权未绑定当前登录会话，请重新启用自动维护")
	}
	if err := s.validateMaintenanceReauthorization(ctx, target, revision); err != nil {
		value.cancel()
		return nil, err
	}
	sessions, ok := s.private.(maintenanceSessionStore)
	if !ok {
		return nil, browserlogin.ErrSession
	}
	active, err := sessions.ActiveSessionOwner(ctx, value.owner, time.Now())
	if err != nil {
		return nil, err
	}
	if !active {
		value.cancel()
		return nil, browserlogin.ErrSession
	}
	return value, nil
}

func (s *Service) MaintenanceAuthorization(ctx context.Context, owner string) (MaintenanceAuthorizationView, error) {
	target, err := s.private.TargetSettings(ctx)
	if err != nil {
		return MaintenanceAuthorizationView{}, err
	}
	s.maintenanceMu.Lock()
	value := s.maintenanceOwner
	s.maintenanceMu.Unlock()
	if value == nil {
		return MaintenanceAuthorizationView{}, nil
	}
	value, err = s.currentMaintenanceOwner(ctx, target, value.revision)
	if err != nil || value.owner != owner {
		return MaintenanceAuthorizationView{}, nil
	}
	s.maintenanceMu.Lock()
	defer s.maintenanceMu.Unlock()
	if s.maintenanceOwner != value {
		return MaintenanceAuthorizationView{}, nil
	}
	return MaintenanceAuthorizationView{Attached: true, CurrentReauthorizationID: value.batchID, CurrentImportTaskID: value.importID}, nil
}
