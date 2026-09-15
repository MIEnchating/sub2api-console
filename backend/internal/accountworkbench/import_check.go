package accountworkbench

import (
	"context"
	"errors"
)

type oauthProxyChecker interface {
	CheckOAuthWithProxy(context.Context, string, string, map[string]any, string, int, string) (map[string]any, error)
}

func (s *Service) checkImportOAuth(ctx context.Context, prepared *preparedImport, row ResultItem, credentials map[string]any) (map[string]any, error) {
	if prepared.proxyURL == "" {
		return s.checker.CheckOAuth(ctx, row.AccountID, row.Name, credentials, prepared.view.Model, 60)
	}
	checker, ok := s.checker.(oauthProxyChecker)
	if !ok {
		return nil, errors.New("检测服务不支持本批代理，请更新检测服务后重试")
	}
	return checker.CheckOAuthWithProxy(ctx, row.AccountID, row.Name, credentials, prepared.view.Model, 60, prepared.proxyURL)
}
