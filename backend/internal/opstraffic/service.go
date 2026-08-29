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
	recentSystemLogMinutes       = 60
	recentErrorLookbackMinutes   = 24 * 60
	traceOperationTimeout        = 30 * time.Second
	traceAuxiliaryRequestTimeout = 3 * time.Second
	traceHTTPTimeout             = 30 * time.Second
)

var errSystemLogLookupFailed = errors.New("Sub2API 系统日志查询失败")

type TargetStore interface {
	TargetSettings(context.Context) (configstore.TargetSettings, error)
}

type AccountStore interface {
	Accounts(context.Context) ([]business.AccountStatus, error)
	Account(context.Context, string) (*business.AccountDetail, error)
}

type Service struct {
	targets  TargetStore
	accounts AccountStore
}

type SystemLogQuery struct {
	TimeRange       string
	StartTime       string
	EndTime         string
	Host            string
	Level           string
	Component       string
	RequestID       string
	ClientRequestID string
	UserID          string
	APIKeyID        string
	AccountID       string
	Platform        string
	Model           string
	Keyword         string
	Page            int
	PageSize        int
}

func New(targets TargetStore, accounts AccountStore) *Service {
	return &Service{targets: targets, accounts: accounts}
}

func (s *Service) SearchSystemLogs(ctx context.Context, query SystemLogQuery) (business.SystemLogPage, error) {
	searchCtx, cancel := context.WithTimeout(ctx, traceOperationTimeout)
	defer cancel()
	client, err := s.adminClient(searchCtx)
	if err != nil {
		return business.SystemLogPage{}, err
	}
	filters := map[string]string{
		"time_range": query.TimeRange, "start_time": query.StartTime, "end_time": query.EndTime,
		"host": query.Host, "level": query.Level, "component": query.Component,
		"request_id": query.RequestID, "client_request_id": query.ClientRequestID,
		"user_id": query.UserID, "api_key_id": query.APIKeyID, "account_id": query.AccountID,
		"platform": query.Platform, "model": query.Model, "q": query.Keyword,
	}
	page, err := client.SystemLogs(searchCtx, filters, query.Page, query.PageSize)
	if err != nil {
		return business.SystemLogPage{}, err
	}
	page.Items = enrichLegacyAccountTestLogs(page.Items)
	result := business.SystemLogPage{
		Items: make([]business.UsageRecord, 0, len(page.Items)), Total: page.Total, Page: page.Page, PageSize: page.PageSize,
	}
	accountNames := map[string]*string{}
	if s.accounts != nil {
		accounts, accountErr := s.accounts.Accounts(searchCtx)
		if accountErr == nil {
			for _, account := range accounts {
				accountNames[account.ID] = stringPointer(account.Name)
			}
		}
	}
	for index, row := range page.Items {
		recordID := int64(index + 1)
		if parsedID, parseErr := strconv.ParseInt(strings.TrimSpace(text(row["id"])), 10, 64); parseErr == nil && parsedID > 0 {
			recordID = parsedID
		}
		result.Items = append(result.Items, systemLogRecord(recordID, row, nil, accountNames[accountIDFromRow(row)]))
	}
	return result, nil
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
		rows, err = systemLogsByRequestID(traceCtx, client, requestID)
		if err != nil {
			if ctx.Err() != nil {
				return business.RequestTrace{}, ctx.Err()
			}
			if traceCtx.Err() != nil {
				return business.RequestTrace{}, traceCtx.Err()
			}
			return result, fmt.Errorf("%w: %w", errSystemLogLookupFailed, err)
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

	accountID := accountIDFromRow(matchedRows[0])
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

func systemLogsByRequestID(ctx context.Context, client *adminclient.Client, requestID string) ([]map[string]any, error) {
	rows, err := client.SystemLogsByRequestID(ctx, requestID, recentSystemLogMinutes, 100)
	if err != nil || len(rows) > 0 {
		return rows, err
	}
	return client.SystemLogsByRequestID(ctx, requestID, traceLookbackMinutes, 100)
}

func accountIDFromRow(row map[string]any) string {
	accountID := strings.TrimSpace(text(row["account_id"]))
	if positiveID(accountID) {
		return accountID
	}
	extra, _ := row["extra"].(map[string]any)
	accountID = strings.TrimSpace(text(extra["account_id"]))
	if positiveID(accountID) {
		return accountID
	}
	return accountIDFromTestPath(row)
}

func enrichLegacyAccountTestLogs(rows []map[string]any) []map[string]any {
	result := append([]map[string]any(nil), rows...)
	for index, row := range rows {
		if accountIDFromRow(row) != "" || !isAccountTestError(row) {
			continue
		}
		accountID := correlatedAccountTestID(row, rows)
		if accountID == "" {
			continue
		}
		copy := make(map[string]any, len(row)+1)
		for key, value := range row {
			copy[key] = value
		}
		copy["account_id"] = accountID
		result[index] = copy
	}
	return result
}

func correlatedAccountTestID(target map[string]any, rows []map[string]any) string {
	targetTime, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(text(target["created_at"])))
	if err != nil {
		return ""
	}
	targetHost := strings.TrimSpace(text(target["host"]))
	candidates := map[string]bool{}
	for _, row := range rows {
		accountID := accountIDFromTestPath(row)
		if accountID == "" {
			continue
		}
		host := strings.TrimSpace(text(row["host"]))
		if targetHost != "" && host != "" && targetHost != host {
			continue
		}
		observedAt, parseErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(text(row["created_at"])))
		if parseErr != nil {
			continue
		}
		delta := observedAt.Sub(targetTime)
		if delta >= 0 && delta <= 5*time.Millisecond {
			candidates[accountID] = true
		}
	}
	if len(candidates) != 1 {
		return ""
	}
	for accountID := range candidates {
		return accountID
	}
	return ""
}

func accountIDFromTestPath(row map[string]any) string {
	path := strings.TrimSpace(text(row["path"]))
	if path == "" {
		extra, _ := row["extra"].(map[string]any)
		path = strings.TrimSpace(text(extra["path"]))
	}
	const marker = "/admin/accounts/"
	markerIndex := strings.Index(path, marker)
	if markerIndex < 0 {
		return ""
	}
	parts := strings.Split(strings.Trim(path[markerIndex+len(marker):], "/"), "/")
	if len(parts) < 2 || parts[1] != "test" || !positiveID(parts[0]) {
		return ""
	}
	return parts[0]
}

func isAccountTestError(row map[string]any) bool {
	message := strings.ToLower(strings.TrimSpace(text(row["message"])))
	return strings.HasPrefix(message, "account test error:")
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
	if accountID == nil {
		accountID = optionalText(accountIDFromTestPath(row))
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
