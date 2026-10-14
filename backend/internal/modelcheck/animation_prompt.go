package modelcheck

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

type AnimationPromptTemplate struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Prompt string `json:"prompt"`
}

type AnimationPromptSettings struct {
	Templates []AnimationPromptTemplate `json:"templates"`
	DefaultID string                    `json:"default_id"`
	Prompt    string                    `json:"prompt"`
	Version   int                       `json:"version"`
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
	s.animationPrompt.settings, _ = normalizeAnimationPrompts(AnimationPromptSettings{Prompt: animationPrompt})
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
	value, err = normalizeAnimationPrompts(value)
	if err != nil {
		return err
	}
	s.animationPrompt.settings = value
	return nil
}
func (s *Service) AnimationPromptSettings() AnimationPromptSettings {
	s.animationPrompt.mu.Lock()
	defer s.animationPrompt.mu.Unlock()
	value := s.animationPrompt.settings
	value.Templates = slices.Clone(value.Templates)
	return value
}
func (s *Service) SaveAnimationPrompt(ctx context.Context, value AnimationPromptSettings, actor string) (AnimationPromptSettings, error) {
	s.animationPrompt.mu.Lock()
	defer s.animationPrompt.mu.Unlock()
	// Legacy clients edit only the current default, preserving other templates.
	if value.Templates == nil && value.DefaultID == "" {
		value.Templates = slices.Clone(s.animationPrompt.settings.Templates)
		value.DefaultID = s.animationPrompt.settings.DefaultID
		for i := range value.Templates {
			if value.Templates[i].ID == value.DefaultID {
				value.Templates[i].Prompt = value.Prompt
			}
		}
	}
	var err error
	value, err = normalizeAnimationPrompts(value)
	if err != nil {
		return AnimationPromptSettings{}, err
	}
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
	value.Templates = slices.Clone(value.Templates)
	return value, nil
}

var animationPromptIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// The old single prompt becomes a named default without changing its content.
func normalizeAnimationPrompts(value AnimationPromptSettings) (AnimationPromptSettings, error) {
	if value.Templates == nil {
		if value.DefaultID != "" {
			return value, errors.New("动画提示词默认项无效，请重新选择")
		}
		value.Templates = []AnimationPromptTemplate{{ID: "default", Name: "默认提示词", Prompt: value.Prompt}}
		value.DefaultID = "default"
	}
	if len(value.Templates) == 0 || len(value.Templates) > 32 {
		return value, errors.New("请保留 1～32 套动画提示词")
	}
	value.Templates = slices.Clone(value.Templates)
	ids, names := map[string]bool{}, map[string]bool{}
	value.Prompt = ""
	for i := range value.Templates {
		item := &value.Templates[i]
		item.Name, item.Prompt = strings.TrimSpace(item.Name), strings.TrimSpace(item.Prompt)
		if !animationPromptIDPattern.MatchString(item.ID) || ids[item.ID] {
			return value, errors.New("动画提示词 ID 无效或重复")
		}
		if item.Name == "" || utf8.RuneCountInString(item.Name) > 80 || strings.ContainsFunc(item.Name, unicode.IsControl) || names[item.Name] {
			return value, errors.New("提示词名称须为 1～80 个字符，不能重复或包含控制字符")
		}
		if err := validateAnimationPrompt(item.Prompt); err != nil {
			return value, err
		}
		ids[item.ID], names[item.Name] = true, true
		if item.ID == value.DefaultID {
			value.Prompt = item.Prompt
		}
	}
	if value.Prompt == "" {
		return value, errors.New("请选择有效的默认动画提示词")
	}
	return value, nil
}

// ResolveAnimationPrompt snapshots the selected template at task creation time.
func (s *Service) ResolveAnimationPrompt(id string) (string, error) {
	value := s.AnimationPromptSettings()
	if id == "" {
		return value.Prompt, nil
	}
	for _, item := range value.Templates {
		if item.ID == id {
			return item.Prompt, nil
		}
	}
	return "", errors.New("所选动画提示词已不存在，请刷新后重新选择")
}
