package upstreamconfig

import (
	"context"
	"fmt"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type recoveryPreferenceStore interface {
	AuthRecoveryPreference(context.Context, string) (*configstore.AuthRecoveryPreference, error)
	SaveAuthRecoveryPreference(context.Context, configstore.AuthRecoveryPreference) error
}

// Record only the entry used by a verified, committed login. Existing recovery
// preferences also cover credentials selected outside the upstream editor.
func (s *Service) saveVaultSelection(ctx context.Context, record configstore.AuthRecord, input Input) error {
	preferences, ok := s.private.(recoveryPreferenceStore)
	if !ok || !isVaultLogin(input.AuthMode) || input.Entry == nil {
		return nil
	}
	entry := strings.TrimSpace(*input.Entry)
	if entry == "" || !input.Present["entry"] {
		return nil
	}
	writeCtx, cancel := detachedOperationContext(ctx)
	defer cancel()
	if err := preferences.SaveAuthRecoveryPreference(writeCtx, configstore.AuthRecoveryPreference{
		Host: record.Host, AuthMode: record.AuthMode, RecoveryMethod: "vault", VaultEntry: &entry,
	}); err != nil {
		return fmt.Errorf("上游配置已保存，但密码箱选择记录失败：%w", err)
	}
	return nil
}
