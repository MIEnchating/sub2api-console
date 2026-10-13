package onboarding

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

type retainedProbeKeyStore interface {
	RetainedProbeKey(context.Context, configstore.AuthRecord, string) (string, error)
	SaveRetainedProbeKey(context.Context, configstore.AuthRecord, string, string) error
}

func (s *Service) readRetainedProbeKey(ctx context.Context, credential *probeCredential, groupID string) (bool, error) {
	store, ok := s.private.(retainedProbeKeyStore)
	if !ok {
		return false, nil
	}
	id, err := store.RetainedProbeKey(ctx, credential.auth, groupID)
	if err != nil || id == "" {
		return false, err
	}
	finish := probeStep(ctx, "read_key")
	key, err := s.keys.RevealKey(ctx, credential.auth, id, groupID)
	if errors.Is(err, upstreamsync.ErrKeyNotFound) {
		finish(nil)
		return false, nil
	}
	if err == nil && (key.KeyID != id || key.GroupID != groupID || strings.TrimSpace(key.Secret) == "") {
		err = errors.New("保留的探活 Key 绑定读回不一致或密钥不可用")
	}
	finish(err)
	if err != nil {
		return false, fmt.Errorf("保留的探活 Key 读取失败，请重试：%w", err)
	}
	credential.key = key
	return true, nil
}

func (s *Service) retainProbeKey(ctx context.Context, credential probeCredential) error {
	store, ok := s.private.(retainedProbeKeyStore)
	if !ok || credential.candidate.GroupID == nil {
		return errors.New("无法持久保存探活 Key 绑定")
	}
	return store.SaveRetainedProbeKey(ctx, credential.auth, *credential.candidate.GroupID, credential.key.KeyID)
}
