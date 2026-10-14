package modelcheck

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

var ErrQualitySettings = errors.New("自动质量标注设置无效，请选择可用评审账号、模型及 5～120 秒超时")
var ErrQualitySettingsConflict = errors.New("自动质量标注设置已变化，请重新打开后保存")

type AnimationQualitySettings struct {
	Enabled        bool   `json:"enabled"`
	AccountID      string `json:"account_id"`
	Model          string `json:"model"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	Version        int    `json:"version"`
}
type qualitySettingsRepository interface {
	LoadAnimationQualitySettings(context.Context) ([]byte, error)
	SaveAnimationQualitySettings(context.Context, []byte, string) error
}
type qualitySettingsState struct {
	mu         sync.RWMutex
	value      AnimationQualitySettings
	repository qualitySettingsRepository
	slots      chan struct{}
}

func validQualitySettings(v AnimationQualitySettings) bool {
	return v.Version >= 0 && v.TimeoutSeconds >= 5 && v.TimeoutSeconds <= 120 &&
		(v.AccountID == "" || stablePositiveID(v.AccountID)) &&
		(v.Model == "" || validQualityText(v.Model, 256, false)) &&
		(!v.Enabled || (v.AccountID != "" && v.Model != ""))
}
func (s *Service) loadQualitySettings(ctx context.Context) error {
	s.qualitySettings.value = AnimationQualitySettings{TimeoutSeconds: 60}
	s.qualitySettings.slots = make(chan struct{}, 1)
	repo, ok := s.accounts.(qualitySettingsRepository)
	if !ok {
		return nil
	}
	s.qualitySettings.repository = repo
	raw, err := repo.LoadAnimationQualitySettings(ctx)
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		return nil
	}
	var v AnimationQualitySettings
	if json.Unmarshal(raw, &v) != nil || !validQualitySettings(v) {
		return ErrQualitySettings
	}
	s.qualitySettings.value = v
	return nil
}
func (s *Service) AnimationQualitySettings() AnimationQualitySettings {
	s.qualitySettings.mu.RLock()
	defer s.qualitySettings.mu.RUnlock()
	return s.qualitySettings.value
}
func (s *Service) qualityReviewer(ctx context.Context, id string) (selectedAccount, error) {
	if err := business.CheckAccountUpstreamManaged(ctx, s.accounts, id); err != nil {
		return selectedAccount{}, err
	}
	detail, err := s.detectionAccount(ctx, id)
	if err != nil || detail == nil {
		return selectedAccount{}, ErrQualitySettings
	}
	account := directAccountSelection(detail.AccountStatus, detail)
	if account.AccountType != "oauth" && account.CredentialError != "" {
		return account, ErrQualitySettings
	}
	if account.Platform != "openai" && account.Platform != "anthropic" && account.Platform != "gemini" {
		return account, ErrQualitySettings
	}
	return account, nil
}
func (s *Service) SaveAnimationQualitySettings(ctx context.Context, v AnimationQualitySettings, actor string) (AnimationQualitySettings, error) {
	v.Model = strings.TrimSpace(v.Model)
	if !validQualitySettings(v) || !validQualityText(actor, 512, false) {
		return v, ErrQualitySettings
	}
	if v.Enabled {
		if _, err := s.qualityReviewer(ctx, v.AccountID); err != nil {
			return v, ErrQualitySettings
		}
	}
	s.qualitySettings.mu.Lock()
	defer s.qualitySettings.mu.Unlock()
	if v.Version != s.qualitySettings.value.Version {
		return v, ErrQualitySettingsConflict
	}
	if s.qualitySettings.repository == nil {
		return v, errors.New("自动质量标注配置存储尚未就绪")
	}
	v.Version++
	raw, err := json.Marshal(v)
	if err != nil {
		return v, err
	}
	if err = s.qualitySettings.repository.SaveAnimationQualitySettings(ctx, raw, actor); err != nil {
		return v, err
	}
	s.qualitySettings.value = v
	return v, nil
}
