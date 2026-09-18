package accountworkbench

import (
	"context"
	"errors"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/workbenchprovider"
	"github.com/pquerna/otp/totp"
)

type loginAssistance struct {
	details     LoginDetails
	mailbox     *workbenchprovider.Mailbox
	http        *workbenchprovider.HTTP
	baseline    workbenchprovider.MailBaseline
	requestedAt time.Time
	nextPoll    time.Time
	used        map[string]bool
	fatal       error
	automatic   bool
}

func (s *Service) prepareLoginAssistance(ctx context.Context, value *privateRun, index int) (*loginAssistance, error) {
	input := &value.Items[index]
	details, err := ParseLoginDetails(input.LoginSource)
	if err != nil {
		return nil, err
	}
	state := &loginAssistance{details: details, used: map[string]bool{}, automatic: value.Automatic}
	if details.Mail.Kind == "" {
		return state, nil
	}
	if input.MailRefreshToken != "" {
		details.Mail.RefreshToken = input.MailRefreshToken
	}
	state.http = workbenchprovider.NewHTTP(s.mailTransport)
	mailbox, err := workbenchprovider.NewMailbox(details.Mail, state.http, func(_ context.Context, token string) error {
		input.MailRefreshToken = token
		if err := s.persistRun(value); err != nil {
			state.fatal = errors.New("邮箱旋转凭据保存失败，已停止授权")
			return state.fatal
		}
		return nil
	})
	if err != nil {
		state.close()
		return nil, errors.New("邮箱收码配置无效")
	}
	state.mailbox = mailbox
	state.baseline, err = mailbox.Baseline(ctx)
	if state.fatal != nil {
		state.close()
		return nil, state.fatal
	}
	if err != nil {
		// A mailbox outage leaves the official form available for manual input.
		mailbox.Close()
		state.mailbox = nil
	}
	return state, nil
}

func (a *loginAssistance) close() {
	if a.mailbox != nil {
		a.mailbox.Close()
	}
	if a.http != nil {
		a.http.Close()
	}
	a.details = LoginDetails{}
}

func (a *loginAssistance) apply(ctx context.Context, active *activeRun) error {
	active.mu.Lock()
	automation, ok := active.browser.(browserlogin.OAuthAutomation)
	if !ok {
		active.mu.Unlock()
		return nil
	}
	page, err := automation.InspectAuth(ctx)
	active.mu.Unlock()
	if err != nil || a.used[page.Stage] {
		return nil
	}
	value := ""
	switch page.Stage {
	case "email":
		value = a.details.Email
	case "password":
		value = a.details.Password
	case "totp_code":
		if a.details.TOTP != "" {
			value, _ = totp.GenerateCode(a.details.TOTP, time.Now().UTC())
		}
	case "email_code":
		if a.mailbox == nil || a.requestedAt.IsZero() || time.Now().Before(a.nextPoll) {
			return nil
		}
		a.nextPoll = time.Now().Add(3 * time.Second)
		candidates, err := a.mailbox.Fetch(ctx, a.requestedAt, a.baseline)
		if a.fatal != nil {
			return a.fatal
		}
		if err == nil && len(candidates) == 1 {
			value = candidates[0].Code
		}
	}
	if value == "" {
		if a.automatic && page.Stage != "email_code" {
			return errors.New("登录需要人工验证，请重新导入该账号")
		}
		return nil
	}
	a.used[page.Stage] = true
	if a.requestedAt.IsZero() {
		a.requestedAt = time.Now().UTC()
	}
	active.mu.Lock()
	defer active.mu.Unlock()
	// Revision validation rejects any form changed by manual input while mail
	// was being fetched. An uncertain submission is never automatically repeated.
	_ = automation.ApplyAuth(ctx, browserlogin.AuthAction{Stage: page.Stage, Revision: page.Revision, Value: value})
	return nil
}
