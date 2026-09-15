package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/workbenchprovider"
)

type smsReceiptStore interface {
	RecordWorkbenchSMS(context.Context, configstore.WorkbenchSMSReceipt) error
	WorkbenchSMSReceipts(context.Context, string, string) ([]configstore.WorkbenchSMSReceipt, error)
}

type smsReceiptJournal struct {
	store smsReceiptStore
	scope configstore.WorkbenchSMSReceipt
}

func (j *smsReceiptJournal) RecordSMS(ctx context.Context, event workbenchprovider.SMSJournalEvent) error {
	value := j.scope
	value.OperationID, value.Action, value.State = event.OperationID, event.Action, event.State
	value.RequestID, value.Phone = event.Number.RequestID, event.Number.Phone
	return j.store.RecordWorkbenchSMS(ctx, value)
}

func smsConfigurationHash(input OAuthSMSInput) string {
	provider := strings.ToLower(strings.TrimSpace(input.Provider))
	identity := struct{ Provider, APIKey, ServiceID, CustomEntries string }{Provider: provider}
	switch provider {
	case "smsbower":
		identity.APIKey = strings.TrimSpace(input.APIKey)
	case "luban":
		identity.APIKey, identity.ServiceID = strings.TrimSpace(input.APIKey), strings.TrimSpace(input.ServiceID)
	case "custom":
		identity.CustomEntries = strings.TrimSpace(input.CustomEntries)
	}
	raw, _ := json.Marshal(identity)
	return exportHash(string(raw))
}

func (s *Service) journalOAuthSMS(owner, taskID string, target configstore.TargetSettings, assist *oauthAssist) error {
	return s.journalOAuthSMSScoped(owner, taskID, ScopeManaged, target, assist)
}

func (s *Service) journalOAuthSMSScoped(owner, taskID string, scope ExportScope, target configstore.TargetSettings, assist *oauthAssist) error {
	if assist == nil || assist.sms == nil {
		return nil
	}
	store, ok := s.private.(smsReceiptStore)
	if !ok {
		return errors.New("短信订单私有记录服务尚未就绪")
	}
	return assist.sms.UseJournal(&smsReceiptJournal{store: store, scope: configstore.WorkbenchSMSReceipt{Owner: exportHash(owner), Target: workbenchScopeFingerprint(scope, target), TaskID: taskID, Provider: assist.smsProvider, ConfigHash: assist.smsConfigHash}})
}

type SMSReceiptView struct {
	Scope      ExportScope `json:"scope"`
	ID         string      `json:"id"`
	TaskID     string      `json:"task_id"`
	Provider   string      `json:"provider"`
	OrderID    string      `json:"order_id,omitempty"`
	Phone      string      `json:"phone,omitempty"`
	Action     string      `json:"action"`
	State      string      `json:"state"`
	UpdatedAt  string      `json:"updated_at"`
	CanInspect bool        `json:"can_inspect"`
}

func (s *Service) SMSReceipts(ctx context.Context, owner string) ([]SMSReceiptView, error) {
	return s.SMSReceiptsScoped(ctx, owner, ScopeManaged)
}

func (s *Service) SMSReceiptsScoped(ctx context.Context, owner string, scope ExportScope) ([]SMSReceiptView, error) {
	scope, err := normalizeWorkbenchScope(scope)
	if err != nil {
		return nil, err
	}
	records, err := s.smsReceipts(ctx, owner, scope)
	if err != nil {
		return nil, err
	}
	views := make(map[string]SMSReceiptView)
	for _, record := range records {
		view := views[record.OperationID]
		view.Scope = scope
		if view.ID == "" || view.UpdatedAt <= record.UpdatedAt {
			view.ID, view.TaskID, view.Provider = record.OperationID, record.TaskID, record.Provider
			view.Action, view.State, view.UpdatedAt = record.Action, record.State, record.UpdatedAt
		}
		if record.RequestID != "" {
			view.OrderID = record.RequestID
		}
		if record.Phone != "" {
			view.Phone = record.Phone
		}
		view.CanInspect = view.OrderID != "" && view.Phone != "" && !(view.State == "confirmed" && (view.Action == "complete" || view.Action == "release"))
		views[record.OperationID] = view
	}
	result := make([]SMSReceiptView, 0, len(views))
	for _, view := range views {
		result = append(result, view)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt > result[j].UpdatedAt })
	return result, nil
}

func (s *Service) smsReceipts(ctx context.Context, owner string, scope ExportScope) ([]configstore.WorkbenchSMSReceipt, error) {
	if owner == "" {
		return nil, errors.New("当前登录会话无效")
	}
	store, ok := s.private.(smsReceiptStore)
	if !ok {
		return nil, errors.New("短信订单记录服务尚未就绪")
	}
	ctx, target, err := s.bindWorkbenchScope(ctx, scope, nil)
	if err != nil {
		return nil, err
	}
	return store.WorkbenchSMSReceipts(ctx, exportHash(owner), workbenchScopeFingerprint(scope, target))
}

type SMSInspectResult struct {
	Pending       bool   `json:"pending"`
	CodeAvailable bool   `json:"code_available"`
	Message       string `json:"message"`
}

// InspectSMSReceipt reads a previously purchased order without acquiring a new
// number, resending an SMS, submitting its code, or retaining the supplied key.
func (s *Service) InspectSMSReceipt(ctx context.Context, owner, id string, input OAuthSMSInput) (SMSInspectResult, error) {
	return s.InspectSMSReceiptScoped(ctx, owner, id, ScopeManaged, input)
}

func (s *Service) InspectSMSReceiptScoped(ctx context.Context, owner, id string, scope ExportScope, input OAuthSMSInput) (SMSInspectResult, error) {
	ctx, target, err := s.bindWorkbenchScope(ctx, scope, nil)
	if err != nil {
		return SMSInspectResult{}, err
	}
	records, err := s.smsReceipts(ctx, owner, scope)
	if err != nil {
		return SMSInspectResult{}, err
	}
	var receipt *configstore.WorkbenchSMSReceipt
	for i := range records {
		if records[i].OperationID != id {
			continue
		}
		if records[i].State == "confirmed" && (records[i].Action == "complete" || records[i].Action == "release") {
			return SMSInspectResult{}, errors.New("短信订单已结束，请核对供应商结果")
		}
		if records[i].RequestID != "" && records[i].Phone != "" {
			receipt = &records[i]
		}
	}
	if receipt == nil || receipt.ConfigHash != smsConfigurationHash(input) {
		return SMSInspectResult{}, errors.New("订单不属于当前会话或供应商配置与原订单不一致")
	}
	client := workbenchprovider.NewHTTP(s.providerTransport)
	defer client.Close()
	provider, err := workbenchprovider.NewSMS(input.config(), client)
	if err != nil {
		return SMSInspectResult{}, err
	}
	defer provider.Close()
	if err := provider.RestoreActivation(id, workbenchprovider.SMSNumber{RequestID: receipt.RequestID, Phone: receipt.Phone}); err != nil {
		return SMSInspectResult{}, err
	}
	bound, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if _, _, err := s.bindWorkbenchScope(bound, scope, &target); err != nil {
		return SMSInspectResult{}, err
	}
	message, err := provider.Poll(bound, receipt.RequestID)
	if err != nil {
		return SMSInspectResult{}, err
	}
	result := SMSInspectResult{Pending: message.Pending, CodeAvailable: message.Code != "", Message: "原短信订单仍在等待验证码"}
	if result.CodeAvailable {
		result.Message = "原订单已收到验证码，请在供应商查看并人工完成原登录页面"
	}
	return result, nil
}
