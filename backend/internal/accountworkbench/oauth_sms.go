package accountworkbench

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/workbenchprovider"
)

type OAuthSMSInput struct {
	Provider      string `json:"provider"`
	APIKey        string `json:"api_key,omitempty"`
	ServiceID     string `json:"service_id,omitempty"`
	Service       string `json:"service,omitempty"`
	Country       string `json:"country,omitempty"`
	MaxPrice      string `json:"max_price,omitempty"`
	CustomEntries string `json:"custom_entries,omitempty"`
	Confirmed     bool   `json:"confirmed"`
}

func (input OAuthSMSInput) config() workbenchprovider.SMSConfig {
	return workbenchprovider.SMSConfig{Provider: input.Provider, APIKey: input.APIKey, ServiceID: input.ServiceID, Service: input.Service, Country: input.Country, MaxPrice: input.MaxPrice, CustomEntries: input.CustomEntries}
}

func (s *Service) SMSOptions(ctx context.Context, input OAuthSMSInput) ([]workbenchprovider.SMSOption, error) {
	if input.Provider != "smsbower" {
		return nil, errors.New("仅 SMSBower 支持国家价格查询")
	}
	client := workbenchprovider.NewHTTP(s.providerTransport)
	defer client.Close()
	provider, err := workbenchprovider.NewSMS(input.config(), client)
	if err != nil {
		return nil, err
	}
	defer provider.Close()
	return provider.Options(ctx)
}

func (a *oauthAssist) prepareSMS(input *OAuthSMSInput, pool *workbenchprovider.SMSPool) error {
	if input == nil {
		return nil
	}
	if !input.Confirmed {
		return errors.New("请先确认手机号绑定及短信供应商费用")
	}
	if input.Provider == "smsbower" && (input.Country == "" || input.MaxPrice == "") {
		return errors.New("请查询并选择 SMSBower 国家价格，设置本次最高价格")
	}
	provider, err := workbenchprovider.NewSMS(input.config(), a.http, pool)
	if err != nil {
		return err
	}
	a.sms = provider
	a.smsProvider, a.smsConfigHash = input.Provider, smsConfigurationHash(*input)
	a.smsOperation, err = randomID()
	return err
}

func (a *oauthAssist) smsValue(ctx context.Context, stage string) (string, error) {
	if a.sms == nil {
		return "", nil
	}
	if stage == "phone" {
		number, err := a.sms.Acquire(ctx, a.smsOperation)
		if number.RequestID != "" {
			a.smsNumber = number
		}
		if err != nil {
			return "", err
		}
		return number.Phone, nil
	}
	if a.smsNumber.RequestID == "" {
		return "", nil
	}
	if time.Since(a.lastSMSPoll) < 5*time.Second {
		return "", nil
	}
	a.lastSMSPoll = time.Now()
	message, err := a.sms.Poll(ctx, a.smsNumber.RequestID)
	if err != nil {
		return "", err
	}
	if message.Pending {
		return "", nil
	}
	return message.Code, nil
}

// Order completion/cancellation uses a fresh bounded context even when the
// authorization task is cancelled. Uncertain writes are never repeated.
func (s *Service) closeOAuthAssist(value *oauthSession, task *taskstore.Task, assist *oauthAssist) {
	if assist == nil {
		return
	}
	defer assist.close()
	value.mu.Lock()
	suspended := value.checkpointSaved
	value.mu.Unlock()
	if suspended {
		return
	}
	if assist.sms == nil || assist.smsNumber.RequestID == "" {
		return
	}
	authorized := task.Status == "succeeded"
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var err error
	if authorized || assist.smsVerified {
		err = assist.sms.Complete(ctx, assist.smsNumber.RequestID)
	} else {
		err = assist.sms.Release(ctx, assist.smsNumber.RequestID)
	}
	if err == nil {
		return
	}
	value.mu.Lock()
	defer value.mu.Unlock()
	value.view.Message += "；短信订单结束状态未确认，请到供应商核对订单"
	task.Message = value.view.Message
	task.Result = maps.Clone(task.Result)
	if task.Result == nil {
		task.Result = make(map[string]any)
	}
	task.Result["sms_order_status"] = "review"
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	persist, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	if err := s.tasks.Save(persist, *task); err != nil {
		slog.Error("账号授权短信订单核对状态保存失败", "task_id", task.ID)
	}
}
