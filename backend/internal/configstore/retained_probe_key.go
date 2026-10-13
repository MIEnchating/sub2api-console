package configstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

// RetainedProbeKey stores only a stable Key ID. Callers must reveal and verify
// its current group with the upstream before using it. The private namespace
// includes the authorization fingerprint so changed identities cannot reuse it.
func (s *Store) RetainedProbeKey(ctx context.Context, auth AuthRecord, groupID string) (string, error) {
	key, err := retainedProbeSettingKey(auth, groupID)
	if err != nil {
		return "", err
	}
	return s.JSONSetting(ctx, key)
}

func (s *Store) SaveRetainedProbeKey(ctx context.Context, auth AuthRecord, groupID, keyID string) error {
	keyID = strings.TrimSpace(keyID)
	if keyID == "" || textLength(keyID) > 255 {
		return errors.New("保留探活 Key 必须包含有效的稳定 Key ID")
	}
	key, err := retainedProbeSettingKey(auth, groupID)
	if err != nil {
		return err
	}
	current, err := s.AuthRecord(ctx, auth.Host)
	if err != nil {
		return err
	}
	if current == nil {
		return ErrUpstreamKeyAuthChanged
	}
	currentKey, err := retainedProbeSettingKey(*current, groupID)
	if err != nil || currentKey != key {
		return ErrUpstreamKeyAuthChanged
	}
	// Even if auth changes after the comparison, the old fingerprint cannot be
	// read using the new authorization. No credential is copied into settings.
	return s.SaveJSONSetting(ctx, key, keyID)
}

func retainedProbeSettingKey(auth AuthRecord, groupID string) (string, error) {
	auth.Host = CanonicalHost(auth.Host)
	groupID = strings.TrimSpace(groupID)
	if auth.Host == "" || groupID == "" || textLength(groupID) > 255 {
		return "", errors.New("保留探活 Key 必须包含有效的上游与分组 ID")
	}
	var err error
	auth.BaseURL, err = ValidateBaseURL(auth.BaseURL)
	if err != nil {
		return "", err
	}
	auth.UpstreamType = strings.ToLower(strings.TrimSpace(auth.UpstreamType))
	auth.AuthMode = strings.TrimSpace(auth.AuthMode)
	auth.Headers, err = normalizedHeaders(auth.Headers)
	if err != nil {
		return "", err
	}
	auth.Cookies, err = normalizedCookies(auth.Cookies)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(auth)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(append(append(raw, 0), []byte(groupID)...))
	return "onboarding.retained_probe_key.v1." + hex.EncodeToString(digest[:]), nil
}
