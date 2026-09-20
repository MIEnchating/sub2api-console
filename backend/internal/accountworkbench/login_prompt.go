package accountworkbench

import (
	"context"
	"errors"
	"strings"
	"time"
)

type LoginPrompt struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}
type LoginInput struct {
	PromptID string `json:"prompt_id"`
	Value    string `json:"value"`
	Action   string `json:"action,omitempty"`
}
type activeLoginPrompt struct {
	id, item, kind string
	values         chan LoginInput
	submitted      bool
}

func (s *Service) SubmitLoginInput(ctx context.Context, owner, id, item string, input LoginInput) error {
	record, err := s.readRun(ctx, owner, id)
	if err != nil {
		return err
	}
	if !record.Public.ExpiresAt.After(time.Now().UTC()) || record.Public.Status != "running" {
		return ErrRun
	}
	s.activeMu.Lock()
	active := s.active[id]
	s.activeMu.Unlock()
	if active == nil || active.owner != owner {
		return ErrRun
	}
	active.mu.Lock()
	defer active.mu.Unlock()
	prompt := active.prompt
	if prompt == nil || prompt.item != item || prompt.id != input.PromptID || prompt.submitted {
		return errors.New("验证步骤已变化，请刷新处理记录")
	}
	if input.Action == "resend_email" {
		if prompt.kind != "email_code" || input.Value != "" {
			return errors.New("当前步骤不支持重发邮箱验证码")
		}
	} else if input.Action != "" || !validLoginInput(prompt.kind, input.Value) {
		return errors.New("请输入有效的密码或六位验证码")
	}
	prompt.submitted = true
	prompt.values <- input
	return nil
}
func validLoginInput(kind, value string) bool {
	if kind == "password" {
		return value != "" && len(value) <= 4096 && !strings.ContainsAny(value, "\r\n\x00")
	}
	return (kind == "email_code" || kind == "totp_code") && len(value) == 6 && strings.Trim(value, "0123456789") == ""
}
func (s *Service) waitLoginInputWithMessage(ctx context.Context, value *privateRun, index int, active *activeRun, kind, message string) (LoginInput, error) {
	if value.Automatic {
		return LoginInput{}, errors.New("登录需要人工验证码或密码，请重新导入该账号")
	}
	row := &value.Public.Items[index]
	prompt := &activeLoginPrompt{id: newID(), item: row.ID, kind: kind, values: make(chan LoginInput, 1)}
	active.mu.Lock()
	active.prompt = prompt
	active.mu.Unlock()
	defer func() { active.mu.Lock(); active.prompt = nil; active.mu.Unlock(); row.LoginPrompt = nil }()
	row.LoginPrompt = &LoginPrompt{ID: prompt.id, Kind: kind}
	row.Status = "waiting_input"
	messages := map[string]string{"password": "请输入账号密码继续授权", "email_code": "请输入邮箱收到的六位验证码", "totp_code": "请输入验证器当前显示的六位验证码"}
	row.Message = messages[kind]
	if message != "" {
		row.Message = message
	}
	if err := s.persistRun(value); err != nil {
		return LoginInput{}, err
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return LoginInput{}, errors.New("授权已取消或到期")
		case <-ticker.C:
			if err := s.executionAllowed(ctx, value); err != nil {
				return LoginInput{}, err
			}
		case input := <-prompt.values:
			if err := s.executionAllowed(ctx, value); err != nil {
				return LoginInput{}, err
			}
			row.LoginPrompt = nil
			row.Status = "authorizing"
			row.Message = "正在提交官方验证"
			if err := s.persistRun(value); err != nil {
				return LoginInput{}, err
			}
			return input, nil
		}
	}
}
