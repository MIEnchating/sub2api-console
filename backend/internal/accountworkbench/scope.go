package accountworkbench

import (
	"context"
	"errors"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

type ExportScope string

const (
	ScopeManaged     ExportScope = "managed"
	ScopeLocalExport ExportScope = "local-export"
)

func normalizeWorkbenchScope(scope ExportScope) (ExportScope, error) {
	switch scope {
	case "", ScopeManaged:
		return ScopeManaged, nil
	case ScopeLocalExport:
		return ScopeLocalExport, nil
	default:
		return "", errors.New("账号工作台范围无效，请重新选择")
	}
}

// Local export has no management target, including when a target is configured.
func (s *Service) bindWorkbenchScope(ctx context.Context, scope ExportScope, expected *configstore.TargetSettings) (context.Context, configstore.TargetSettings, error) {
	scope, err := normalizeWorkbenchScope(scope)
	if err != nil {
		return ctx, configstore.TargetSettings{}, err
	}
	if err := ctx.Err(); err != nil {
		return ctx, configstore.TargetSettings{}, err
	}
	if scope == ScopeLocalExport {
		return ctx, configstore.TargetSettings{}, nil
	}
	if expected != nil {
		ctx = targetguard.Expect(ctx, *expected)
	}
	ctx, err = targetguard.Pin(ctx, s.private)
	if err != nil {
		return ctx, configstore.TargetSettings{}, err
	}
	target, err := targetguard.Settings(ctx, s.private)
	return ctx, target, err
}

func workbenchScopeFingerprint(scope ExportScope, target configstore.TargetSettings) string {
	if scope == ScopeLocalExport {
		return exportHash("account-workbench:local-export:v1")
	}
	return executionTargetFingerprint(target)
}

func (s *Service) validateOAuthScope(ctx context.Context, value *oauthSession) error {
	_, _, err := s.bindWorkbenchScope(ctx, value.scope, &value.target)
	return err
}
