package upstreamconfig

import (
	"context"
	"fmt"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

// resolveUpdateHost runs under the source and candidate mutation guards. An
// existing connection address or an explicit stable alias updates the source
// configuration without moving its catalogs, credentials or account bindings.
func (s *Service) resolveUpdateHost(ctx context.Context, previous Configuration, targetHost string, deriveHost bool) (string, error) {
	if targetHost == previous.Host || (deriveHost && targetHost == configstore.CanonicalHost(previous.BaseURL)) {
		return previous.Host, nil
	}
	identity, found, err := s.updateHostIdentity(ctx, targetHost)
	if err != nil {
		return "", err
	}
	if found {
		if previous.UpstreamID != "" && identity == previous.UpstreamID {
			return previous.Host, nil
		}
		return "", inputError(fmt.Errorf("%w：%s 属于另一条上游，请编辑该上游或使用未占用的地址", ErrConflict, targetHost))
	}
	record, err := s.private.AuthRecord(ctx, targetHost)
	if err != nil {
		return "", err
	}
	if record != nil {
		return "", inputError(fmt.Errorf("%w：%s 已有独立鉴权配置，请使用未占用的地址或先处理原有鉴权配置", ErrConflict, targetHost))
	}
	return targetHost, nil
}

func (s *Service) updateHostIdentity(ctx context.Context, host string) (string, bool, error) {
	identity, err := s.business.UpstreamCatalogIdentityID(ctx, host)
	return identity, identity != "", err
}
