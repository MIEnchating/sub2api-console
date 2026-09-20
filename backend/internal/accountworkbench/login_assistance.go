package accountworkbench

import (
	"context"
	"errors"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/workbenchprovider"
)

type loginAssistance struct {
	details     LoginDetails
	mailbox     *workbenchprovider.Mailbox
	http        *workbenchprovider.HTTP
	baseline    workbenchprovider.MailBaseline
	requestedAt time.Time
	fatal       error
}

func (s *Service) prepareLoginAssistance(ctx context.Context, value *privateRun, index int) (*loginAssistance, error) {
	input := &value.Items[index]
	details, err := ParseLoginDetails(input.LoginSource)
	if err != nil {
		return nil, err
	}
	if input.LoginPassword != "" {
		details.Password = input.LoginPassword
	}
	state := &loginAssistance{details: details}
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
		// A mailbox outage permits manual code input for interactive tasks.
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
