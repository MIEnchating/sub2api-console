package opstraffic

import (
	"context"
	"errors"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

// AccountTraffic reads a short-lived snapshot of gateway slots. Queue depth and
// historical usage are never interpreted as active requests.
func (s *Service) AccountTraffic(ctx context.Context) (adminclient.AccountTrafficSnapshot, error) {
	s.trafficMu.Lock()
	defer s.trafficMu.Unlock()
	if s.targets == nil {
		return adminclient.AccountTrafficSnapshot{}, errors.New("尚未配置 Sub2API 管理目标")
	}
	target, err := s.targets.TargetSettings(ctx)
	if err != nil {
		return adminclient.AccountTrafficSnapshot{}, err
	}
	if s.trafficCached != nil && target.BaseURL == s.trafficTarget.BaseURL && target.AdminKey == s.trafficTarget.AdminKey && time.Since(s.trafficCachedAt) < 3*time.Second {
		return *s.trafficCached, nil
	}
	s.trafficCached = nil
	readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	client, err := adminclient.New(adminclient.Config{BaseURL: target.BaseURL, AdminKey: target.AdminKey, Timeout: 5 * time.Second, Attempts: 1}, nil)
	if err != nil {
		return adminclient.AccountTrafficSnapshot{}, err
	}
	snapshot, err := client.AccountTraffic(readCtx)
	if err != nil {
		return adminclient.AccountTrafficSnapshot{}, err
	}
	if _, err = targetguard.Pin(targetguard.Expect(readCtx, target), s.targets); err != nil {
		return adminclient.AccountTrafficSnapshot{}, err
	}
	if snapshot.Enabled && (time.Since(snapshot.ObservedAt) > 15*time.Second || time.Until(snapshot.ObservedAt) > 5*time.Second) {
		return adminclient.AccountTrafficSnapshot{}, errors.New("Sub2API 实时并发快照已过期，请检查服务器时间和运维监控")
	}
	s.trafficCached = &snapshot
	s.trafficTarget = target
	s.trafficCachedAt = time.Now()
	return snapshot, nil
}
