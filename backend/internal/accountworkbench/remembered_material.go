package accountworkbench

import (
	"context"
	"encoding/json"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

// Persist corrected passwords and rotated mailbox tokens before acknowledging
// them to the authorization flow, including a flow that subsequently fails.
func (s *Service) persistRememberedLoginMaterial(ctx context.Context, value *privateRun) error {
	if value.RememberedSource == nil || len(value.Items) != 1 {
		return nil
	}
	s.rememberMu.Lock()
	defer s.rememberMu.Unlock()
	source := value.RememberedSource
	saved, err := s.readRememberedAccount(ctx, value.Target, source.ID)
	if err != nil {
		return err
	}
	if saved.Public.Revision != source.Revision {
		return configstore.ErrWorkbenchVersion
	}
	input := value.Items[0]
	if input.LoginPassword == saved.Input.LoginPassword && input.MailRefreshToken == saved.Input.MailRefreshToken {
		return nil
	}
	saved.Input.LoginPassword = input.LoginPassword
	saved.Input.MailRefreshToken = input.MailRefreshToken
	raw, err := json.Marshal(saved)
	if err != nil {
		return err
	}
	revision, err := s.private.SaveWorkbenchDocument(ctx, rememberedAccountKey(value.Target, source.ID), source.Revision, raw)
	if err == nil {
		source.Revision = revision
	}
	return err
}
