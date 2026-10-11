package modelcheck

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

type AnimationPromptSettings struct {
	Prompt  string `json:"prompt"`
	Version int    `json:"version"`
}
type animationPromptRepository interface {
	LoadAnimationPrompt(context.Context) ([]byte, error)
	SaveAnimationPrompt(context.Context, []byte, string) error
}
type animationPromptState struct {
	mu         sync.Mutex
	settings   AnimationPromptSettings
	repository animationPromptRepository
}

func validateAnimationPrompt(prompt string) error {
	if strings.TrimSpace(prompt) == "" || utf8.RuneCountInString(prompt) > maximumAnimationPromptLength || strings.ContainsFunc(prompt, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' }) {
		return errors.New("动画提示词不能为空、超过 10000 个字符或包含控制字符")
	}
	return nil
}
func (s *Service) loadAnimationPrompt(ctx context.Context) error {
	s.animationPrompt.settings = AnimationPromptSettings{Prompt: animationPrompt}
	repository, ok := s.accounts.(animationPromptRepository)
	if !ok {
		return nil
	}
	s.animationPrompt.repository = repository
	raw, err := repository.LoadAnimationPrompt(ctx)
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		return nil
	}
	var value AnimationPromptSettings
	if json.Unmarshal(raw, &value) != nil || value.Version < 1 {
		return errors.New("动画提示词配置无效")
	}
	if err := validateAnimationPrompt(value.Prompt); err != nil {
		return err
	}
	s.animationPrompt.settings = value
	return nil
}
func (s *Service) AnimationPromptSettings() AnimationPromptSettings {
	s.animationPrompt.mu.Lock()
	defer s.animationPrompt.mu.Unlock()
	return s.animationPrompt.settings
}
func (s *Service) SaveAnimationPrompt(ctx context.Context, value AnimationPromptSettings, actor string) (AnimationPromptSettings, error) {
	value.Prompt = strings.TrimSpace(value.Prompt)
	if err := validateAnimationPrompt(value.Prompt); err != nil {
		return AnimationPromptSettings{}, err
	}
	s.animationPrompt.mu.Lock()
	defer s.animationPrompt.mu.Unlock()
	if s.animationPrompt.repository == nil {
		return AnimationPromptSettings{}, errors.New("动画提示词存储尚未就绪")
	}
	if value.Version != s.animationPrompt.settings.Version {
		return AnimationPromptSettings{}, errors.New("动画提示词已变化，请重新打开设置后再保存")
	}
	value.Version++
	raw, err := json.Marshal(value)
	if err == nil {
		err = s.animationPrompt.repository.SaveAnimationPrompt(ctx, raw, actor)
	}
	if err != nil {
		return AnimationPromptSettings{}, err
	}
	s.animationPrompt.settings = value
	return value, nil
}
