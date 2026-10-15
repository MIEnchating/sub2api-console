package modelcheck

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
)

var ErrDetectionTimeout = errors.New("检测超时设置无效，请选择动画、前置检测或评审，超时须为 5～120 秒")
var ErrDetectionTimeoutConflict = errors.New("检测超时已被修改，请重新读取后保存")

type DetectionTimeout struct {
	TimeoutSeconds int `json:"timeout_seconds"`
	Version        int `json:"version"`
}
type detectionTimeoutRepository interface {
	LoadDetectionTimeouts(context.Context) ([]byte, error)
	SaveDetectionTimeouts(context.Context, []byte, string) error
}
type detectionTimeoutState struct {
	mu         sync.RWMutex
	values     map[string]DetectionTimeout
	repository detectionTimeoutRepository
}

func (s *Service) loadDetectionTimeouts(ctx context.Context) error {
	s.detectionTimeouts.values = map[string]DetectionTimeout{"animation": {TimeoutSeconds: 120}, "precheck": {TimeoutSeconds: 120}}
	repo, ok := s.accounts.(detectionTimeoutRepository)
	if !ok {
		return nil
	}
	s.detectionTimeouts.repository = repo
	raw, err := repo.LoadDetectionTimeouts(ctx)
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		return nil
	}
	var values map[string]DetectionTimeout
	if json.Unmarshal(raw, &values) != nil || len(values) != 2 {
		return ErrDetectionTimeout
	}
	for _, mode := range []string{"animation", "precheck"} {
		v, ok := values[mode]
		if !ok || !validDetectionTimeout(v) {
			return ErrDetectionTimeout
		}
	}
	s.detectionTimeouts.values = values
	return nil
}
func validDetectionTimeout(v DetectionTimeout) bool {
	return v.TimeoutSeconds >= 5 && v.TimeoutSeconds <= 120 && v.Version >= 0
}
func (s *Service) DetectionTimeout(mode string) (DetectionTimeout, error) {
	if mode == "review" {
		v := s.AnimationQualitySettings()
		return DetectionTimeout{TimeoutSeconds: v.TimeoutSeconds, Version: v.Version}, nil
	}
	s.detectionTimeouts.mu.RLock()
	defer s.detectionTimeouts.mu.RUnlock()
	v, ok := s.detectionTimeouts.values[mode]
	if !ok {
		return v, ErrDetectionTimeout
	}
	return v, nil
}
func (s *Service) SaveDetectionTimeout(ctx context.Context, mode string, v DetectionTimeout, actor string) (DetectionTimeout, error) {
	if !validDetectionTimeout(v) || !validQualityText(actor, 512, false) {
		return v, ErrDetectionTimeout
	}
	if mode == "review" {
		return s.saveQualityTimeout(ctx, v, actor)
	}
	s.detectionTimeouts.mu.Lock()
	defer s.detectionTimeouts.mu.Unlock()
	current, ok := s.detectionTimeouts.values[mode]
	if !ok {
		return v, ErrDetectionTimeout
	}
	if current.Version != v.Version {
		return v, ErrDetectionTimeoutConflict
	}
	if s.detectionTimeouts.repository == nil {
		return v, errors.New("检测超时存储尚未就绪")
	}
	values := map[string]DetectionTimeout{}
	for key, value := range s.detectionTimeouts.values {
		values[key] = value
	}
	v.Version++
	values[mode] = v
	raw, err := json.Marshal(values)
	if err != nil {
		return v, err
	}
	if err = s.detectionTimeouts.repository.SaveDetectionTimeouts(ctx, raw, actor); err != nil {
		return v, err
	}
	s.detectionTimeouts.values = values
	return v, nil
}

// Timeout-only updates preserve reviewer drafts and permit disabling or repairing
// an unavailable reviewer without resolving credentials or calling an upstream.
func (s *Service) saveQualityTimeout(ctx context.Context, v DetectionTimeout, actor string) (DetectionTimeout, error) {
	s.qualitySettings.mu.Lock()
	defer s.qualitySettings.mu.Unlock()
	current := s.qualitySettings.value
	if v.Version != current.Version {
		return v, ErrDetectionTimeoutConflict
	}
	if s.qualitySettings.repository == nil {
		return v, errors.New("自动质量标注配置存储尚未就绪")
	}
	current.TimeoutSeconds = v.TimeoutSeconds
	current.Version++
	raw, err := json.Marshal(current)
	if err != nil {
		return v, err
	}
	if err = s.qualitySettings.repository.SaveAnimationQualitySettings(ctx, raw, actor); err != nil {
		return v, err
	}
	s.qualitySettings.value = current
	v.Version = current.Version
	return v, nil
}
