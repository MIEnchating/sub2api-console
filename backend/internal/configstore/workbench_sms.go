package configstore

import (
	"context"
	"errors"
	"strings"
	"time"
)

type WorkbenchSMSReceipt struct {
	OperationID string
	Action      string
	State       string
	Owner       string
	Target      string
	TaskID      string
	Provider    string
	ConfigHash  string
	RequestID   string
	Phone       string
	CreatedAt   string
	UpdatedAt   string
}

var ErrWorkbenchSMSReceipt = errors.New("短信订单记录已变化或结果尚未确定，请核对原订单")

func (s *Store) RecordWorkbenchSMS(ctx context.Context, value WorkbenchSMSReceipt) error {
	if !workbenchIdentifier.MatchString(value.OperationID) || !workbenchIdentifier.MatchString(value.TaskID) || len(value.Owner) != 64 || len(value.Target) != 64 || len(value.ConfigHash) != 64 || strings.Trim(value.Owner+value.Target+value.ConfigHash, "0123456789abcdef") != "" || len(value.RequestID) > 128 || len(value.Phone) > 32 || strings.ContainsAny(value.RequestID+value.Phone, "\r\n\x00") {
		return ErrWorkbenchSMSReceipt
	}
	switch value.Provider {
	case "smsbower", "luban", "custom":
	default:
		return ErrWorkbenchSMSReceipt
	}
	switch value.Action {
	case "acquire", "ready", "complete", "release":
	default:
		return ErrWorkbenchSMSReceipt
	}
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000000000Z")
	if value.State == "submitted" {
		result, err := s.db.ExecContext(ctx, `INSERT INTO workbench_sms_receipts(operation_id,action,state,owner,target,task_id,provider,config_hash,request_id,phone,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(operation_id,action) DO NOTHING`, value.OperationID, value.Action, value.State, value.Owner, value.Target, value.TaskID, value.Provider, value.ConfigHash, value.RequestID, value.Phone, now, now)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrWorkbenchSMSReceipt
		}
		return nil
	}
	if value.State != "confirmed" && value.State != "uncertain" {
		return ErrWorkbenchSMSReceipt
	}
	result, err := s.db.ExecContext(ctx, `UPDATE workbench_sms_receipts SET state=?,request_id=?,phone=?,updated_at=? WHERE operation_id=? AND action=? AND state='submitted' AND owner=? AND target=? AND task_id=? AND provider=? AND config_hash=?`, value.State, value.RequestID, value.Phone, now, value.OperationID, value.Action, value.Owner, value.Target, value.TaskID, value.Provider, value.ConfigHash)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrWorkbenchSMSReceipt
	}
	return nil
}

func (s *Store) WorkbenchSMSReceipts(ctx context.Context, owner, target string) ([]WorkbenchSMSReceipt, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT operation_id,action,state,owner,target,task_id,provider,config_hash,request_id,phone,created_at,updated_at FROM workbench_sms_receipts WHERE owner=? AND target=? ORDER BY created_at,operation_id,action`, owner, target)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []WorkbenchSMSReceipt{}
	for rows.Next() {
		var value WorkbenchSMSReceipt
		if err := rows.Scan(&value.OperationID, &value.Action, &value.State, &value.Owner, &value.Target, &value.TaskID, &value.Provider, &value.ConfigHash, &value.RequestID, &value.Phone, &value.CreatedAt, &value.UpdatedAt); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}
