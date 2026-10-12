package accountworkbench

import (
	"context"
	"errors"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func (s *Service) cleanupMaintenance(ctx context.Context, recovering bool) error {
	now := time.Now().UTC()
	if err := s.documents(ctx, "maintenance-preview:", func(record configstore.WorkbenchDocumentRecord) error {
		var preview privateMaintenancePreview
		if decodePrivate(record.Payload, &preview) != nil {
			return errors.New("维护预览无法读取")
		}
		live, err := s.private.ActiveSessionOwner(ctx, preview.Owner, now)
		if err != nil {
			return err
		}
		if live && preview.Public.ExpiresAt.After(now) {
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
	return s.documents(ctx, "maintenance:", func(record configstore.WorkbenchDocumentRecord) error {
		value := defaultMaintenance()
		if decodePrivate(record.Payload, &value) != nil {
			return errors.New("维护资料无法读取")
		}
		value.Public.Revision = record.Revision
		s.activeMu.Lock()
		active := s.active[record.ID]
		s.activeMu.Unlock()
		if active != nil {
			return nil
		}
		changed := false
		if recovering && value.Public.Running {
			value.Public.Running = false
			value.Public.Message = "服务已重启，按已保存设置继续维护"
			changed = true
		}
		if value.Public.Enabled && value.Public.NextCheckAt == nil {
			next := now.Add(time.Duration(value.Public.IntervalMinutes) * time.Minute)
			value.Public.NextCheckAt = &next
			changed = true
		}
		for _, attempt := range value.Attempts {
			if !attempt.ExpiresAt.After(now) && attempt.Phase != "expired" {
				attempt.Credentials = nil
				attempt.ProxyURL = ""
				attempt.ProxyBinding = nil
				attempt.Phase = "expired"
				changed = true
			}
		}
		if changed {
			return s.saveMaintenance(ctx, &value)
		}
		return nil
	})
}
