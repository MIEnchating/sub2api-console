package opstraffic

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

const (
	traceLookbackMinutes         = 30 * 24 * 60
	recentErrorLookbackMinutes   = 24 * 60
	traceOperationTimeout        = 10 * time.Second
	traceAuxiliaryRequestTimeout = 2 * time.Second
	traceHTTPTimeout             = 12 * time.Second
)

type TargetStore interface {
	TargetSettings(context.Context) (configstore.TargetSettings, error)
}

type AccountStore interface {
	Account(context.Context, string) (*business.AccountDetail, error)
}

type Service struct {
	targets  TargetStore
	accounts AccountStore
}

func New(targets TargetStore, accounts AccountStore) *Service {
	return &Service{targets: targets, accounts: accounts}
}

func (s *Service) RequestTrace(ctx context.Context, requestID string) (business.RequestTrace, error) {
	requestID = strings.TrimSpace(requestID)
	result := business.RequestTrace{RequestID: requestID, Records: []business.UsageRecord{}, RecentErrors: []business.UsageRecord{}}
	if requestID == "" {
		return result, nil
	}
	traceCtx, cancelTrace := context.WithTimeout(ctx, traceOperationTimeout)
	defer cancelTrace()
	client, err := s.adminClient(traceCtx)
	if err != nil {
		return business.RequestTrace{}, err
	}
	rows, err := client.RequestTrace(traceCtx, requestID, traceLookbackMinutes, 100)
	if err != nil {
		return business.RequestTrace{}, err
	}
	matchedRows := make([]map[string]any, 0, len(rows))
	fromSystemLogs := false
	for _, row := range rows {
		if strings.TrimSpace(text(row["request_id"])) != requestID {
			continue
		}
		matchedRows = append(matchedRows, row)
		result.Records = append(result.Records, usageRecord(int64(len(result.Records)+1), row, nil, nil))
	}
	if len(matchedRows) == 0 {
		fallbackCtx, cancelFallback := context.WithTimeout(traceCtx, traceAuxiliaryRequestTimeout)
		rows, err = client.SystemLogsByRequestID(fallbackCtx, requestID, recentErrorLookbackMinutes, 100)
		cancelFallback()
		if err != nil {
			if ctx.Err() != nil {
				return business.RequestTrace{}, ctx.Err()
			}
			if traceCtx.Err() != nil {
				return business.RequestTrace{}, traceCtx.Err()
			}
			return result, nil
		}
		fromSystemLogs = true
		for _, row := range rows {
			if strings.TrimSpace(text(row["request_id"])) != requestID {
				continue
			}
			matchedRows = append(matchedRows, row)
			result.Records = append(result.Records, systemLogRecord(int64(len(result.Records)+1), row, nil, nil))
		}
	}
	result.Matched = len(result.Records) > 0
	if !result.Matched {
		return result, nil
	}

	accountID := strings.TrimSpace(text(matchedRows[0]["account_id"]))
	if !positiveID(accountID) {
		return result, nil
	}
	result.AccountID = stringPointer(accountID)
	detail, detailErr := s.accounts.Account(traceCtx, accountID)
	if detailErr == nil {
		result.AccountName = stringPointer(detail.Name)
		for index := range result.Records {
			if fromSystemLogs {
				result.Records[index] = systemLogRecord(result.Records[index].ID, matchedRows[index], detail, result.AccountName)
			} else {
				result.Records[index] = usageRecord(result.Records[index].ID, matchedRows[index], detail, result.AccountName)
			}
		}
	}

	errorCtx, cancelErrors := context.WithTimeout(traceCtx, traceAuxiliaryRequestTimeout)
	errorRows, err := client.RequestErrors(errorCtx, accountID, recentErrorLookbackMinutes, 20)
	cancelErrors()
	if err != nil {
		if ctx.Err() != nil {
			return business.RequestTrace{}, ctx.Err()
		}
		return result, nil
	}
	for index, row := range errorRows {
		result.RecentErrors = append(result.RecentErrors, usageRecord(int64(index+1), row, detail, result.AccountName))
	}
	return result, nil
}

func systemLogRecord(id int64, row map[string]any, detail *business.AccountDetail, accountName *string) business.UsageRecord {
	extra, _ := row["extra"].(map[string]any)
	level := strings.ToLower(strings.TrimSpace(text(row["level"])))
	isError := level == "error" || level == "fatal" || level == "panic" || level == "dpanic"
	message := optionalText(row["message"])
	accountID := optionalText(row["account_id"])
	if accountID == nil {
		accountID = optionalText(extra["account_id"])
	}
	duration := optionalText(extra["latency_ms"])
	if duration == nil {
		duration = optionalText(extra["duration_ms"])
	}
	firstToken := optionalText(extra["first_token_ms"])
	return business.UsageRecord{
		ID: id, RequestID: strings.TrimSpace(text(row["request_id"])), AccountID: accountID,
		AccountName: accountName, GroupName: groupName(row, detail), IsError: boolPointer(isError),
		ErrorReason: conditionalText(isError, message), FirstTokenMS: firstToken, DurationMS: duration,
		Summary: message, ObservedAt: optionalText(row["created_at"]), Source: "system-log", Payload: row,
	}
}

func (s *Service) adminClient(ctx context.Context) (*adminclient.Client, error) {
	if s.targets == nil {
		return nil, errors.New("未配置 Sub2API 管理目标")
	}
	target, err := s.targets.TargetSettings(ctx)
	if err != nil {
		return nil, err
	}
	timeout := time.Duration(target.TimeoutSeconds) * time.Second
	if timeout <= 0 || timeout > traceHTTPTimeout {
		timeout = traceHTTPTimeout
	}
	return adminclient.New(adminclient.Config{
		BaseURL: target.BaseURL, AdminKey: target.AdminKey,
		Timeout: timeout, Attempts: 1,
	}, nil)
}

func usageRecord(id int64, row map[string]any, detail *business.AccountDetail, accountName *string) business.UsageRecord {
	accountID := optionalText(row["account_id"])
	groupName := groupName(row, detail)
	kind := strings.ToLower(strings.TrimSpace(text(row["kind"])))
	isError := kind == "error"
	var reason *string
	if isError {
		reason = optionalText(row["message"])
	}
	return business.UsageRecord{
		ID: id, RequestID: strings.TrimSpace(text(row["request_id"])), AccountID: accountID,
		AccountName: accountName, GroupName: groupName, IsError: boolPointer(isError), ErrorReason: reason,
		DurationMS: optionalText(row["duration_ms"]), ObservedAt: optionalText(row["created_at"]),
		Source: "traffic", Payload: row,
	}
}

func groupName(row map[string]any, detail *business.AccountDetail) *string {
	if detail == nil {
		return nil
	}
	groupID := strings.TrimSpace(text(row["group_id"]))
	for name, id := range detail.GroupIDs {
		if id != nil && strings.TrimSpace(*id) == groupID {
			return stringPointer(name)
		}
	}
	return nil
}

func text(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func optionalText(value any) *string {
	valueText := strings.TrimSpace(text(value))
	if valueText == "" || valueText == "<nil>" {
		return nil
	}
	return stringPointer(valueText)
}

func conditionalText(condition bool, value *string) *string {
	if condition {
		return value
	}
	return nil
}

func stringPointer(value string) *string {
	copy := value
	return &copy
}

func boolPointer(value bool) *bool {
	copy := value
	return &copy
}

func positiveID(value string) bool {
	parsed, err := strconv.ParseUint(value, 10, 64)
	return err == nil && parsed > 0
}
