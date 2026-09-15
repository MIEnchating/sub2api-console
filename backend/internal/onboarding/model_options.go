package onboarding

import (
	"context"
	"errors"
	"fmt"

	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
)

// Model selection has no follow-up generation request. Keep its credential
// independent from an open probe dialog and finish cleanup before publishing.
func (s *Service) probeModelOptions(ctx context.Context, host, groupID string) ([]string, error) {
	guardedCtx, release, err := mutationguard.Acquire(ctx, s.repository, mutationguard.Upstream(host), mutationguard.UpstreamKeyCatalog(host))
	if err != nil {
		return nil, err
	}
	defer s.releaseProbeMutation(release, host)

	s.probeMu.Lock()
	pending, found := s.probeCleanup[s.probeSessionKey(host, groupID)+"\x00model-options"]
	s.probeMu.Unlock()
	if found {
		if err := s.cleanupProbeCredentialWithContext(context.WithoutCancel(guardedCtx), pending); err != nil {
			return nil, fmt.Errorf("上次模型读取的临时 Key 清理失败：%w", err)
		}
	}
	marker, err := randomID(12)
	if err != nil {
		return nil, err
	}
	credential, err := s.acquireProbeCredentialWithMarker(guardedCtx, host, groupID, "console-probe-options-"+marker)
	if err != nil {
		return nil, err
	}
	credential.modelOptions = true
	finishModels := probeStep(ctx, "models")
	models, requestErr := fetchProbeModels(guardedCtx, credential.auth.BaseURL, credential.key.Secret)
	if requestErr == nil && len(models) == 0 {
		requestErr = errors.New("上游模型接口未返回可选择的模型")
	}
	if requestErr == nil {
		requestErr = guardedCtx.Err()
	}
	finishModels(requestErr)
	cleanupErr := s.cleanupProbeCredentialWithContext(context.WithoutCancel(guardedCtx), credential)
	if cleanupErr != nil {
		cleanupErr = fmt.Errorf("模型读取的临时 Key 清理失败：%w", cleanupErr)
	}
	if err := errors.Join(requestErr, cleanupErr); err != nil {
		return nil, err
	}
	return models, nil
}
