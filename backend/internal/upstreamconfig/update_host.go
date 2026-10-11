package upstreamconfig

import (
	"context"
	"fmt"
	"slices"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type catalogIdentityStore interface {
	UpstreamCatalogIdentityID(context.Context, string) (string, error)
}

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
	if store, ok := s.business.(catalogIdentityStore); ok {
		identity, err := store.UpstreamCatalogIdentityID(ctx, host)
		return identity, identity != "", err
	}
	summary, err := s.business.Upstreams(ctx)
	if err != nil {
		return "", false, err
	}
	for _, upstream := range summary.Hosts {
		if upstream.Host == host || slices.Contains(upstream.Hosts, host) {
			return upstream.UpstreamID, true, nil
		}
	}
	return "", false, nil
}
