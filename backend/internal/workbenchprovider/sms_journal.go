package workbenchprovider

import (
	"context"
	"errors"
	"time"
)

// SMSJournal receives receipt metadata only, never API keys or received codes.
// A submitted entry must be durable before a supplier write can begin.
type SMSJournal interface {
	RecordSMS(context.Context, SMSJournalEvent) error
}

type SMSJournalEvent struct {
	OperationID string
	Action      string
	State       string
	Number      SMSNumber
}

func (s *SMS) UseJournal(journal SMSJournal) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || len(s.acquisitions) != 0 || s.journal != nil || journal == nil {
		return errors.New("接码订单记录必须在申请号码前配置")
	}
	s.journal = journal
	return nil
}

func (s *SMS) journalWrite(ctx context.Context, operation, action, state string, number SMSNumber) error {
	if s.journal == nil {
		return nil
	}
	if state != "submitted" {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
	}
	if err := s.journal.RecordSMS(ctx, SMSJournalEvent{OperationID: operation, Action: action, State: state, Number: number}); err != nil {
		return &SMSError{Code: "sms_receipt_unavailable", Message: "短信订单记录无法可靠保存，请核对供应商订单；本次操作不会自动重发", Terminal: true, Uncertain: state != "submitted"}
	}
	return nil
}

// RestoreActivation permits explicit read-only reconciliation of a known order.
// Callers must verify the private receipt and matching supplier configuration.
func (s *SMS) RestoreActivation(operation string, number SMSNumber) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || !smsRequestID.MatchString(operation) || !smsRequestID.MatchString(number.RequestID) || !smsPhone.MatchString(number.Phone) || len(s.activations) != 0 {
		return errors.New("待核对短信订单格式无效")
	}
	activation := &smsActivation{number: number, operation: operation, actions: make(map[string]error), readOnly: true}
	if s.config.Provider == "custom" {
		for _, entry := range s.entries {
			if entry.phone == number.Phone {
				copy := entry
				activation.custom = &copy
				activation.seenKeys, activation.seenCodes = make(map[string]bool), make(map[string]bool)
				activation.acquiredAt = time.Now()
				break
			}
		}
		if activation.custom == nil {
			return errors.New("自定义接码配置未包含原订单号码")
		}
	}
	s.activations[number.RequestID] = activation
	s.acquisitions[operation] = smsAcquisition{number: number}
	return nil
}
