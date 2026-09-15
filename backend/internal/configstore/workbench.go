package configstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/decimalutil"
)

// WorkbenchTemplateConfig contains only transferable account settings. It must
// pass NormalizeWorkbenchTemplateConfig before being saved or applied.
type WorkbenchTemplateConfig map[string]json.RawMessage

type WorkbenchTemplateMatch struct {
	PlanType    string `json:"plan_type"`
	EmailDomain string `json:"email_domain"`
}

type WorkbenchTemplate struct {
	ID              string                  `json:"id"`
	Revision        int64                   `json:"revision"`
	Name            string                  `json:"name"`
	TargetURL       string                  `json:"target_url"`
	SourceAccountID string                  `json:"source_account_id,omitempty"`
	SourceName      string                  `json:"source_name,omitempty"`
	SourceRevision  string                  `json:"source_revision,omitempty"`
	SourceSyncedAt  string                  `json:"source_synced_at,omitempty"`
	Preferred       bool                    `json:"preferred"`
	Match           WorkbenchTemplateMatch  `json:"match"`
	Priority        int64                   `json:"priority"`
	Config          WorkbenchTemplateConfig `json:"config"`
}

var ErrWorkbenchTemplateConflict = errors.New("账号模板已被修改、删除或不属于当前管理目标，请刷新后重试")

var workbenchIdentifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)
var workbenchDomain = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`)

const workbenchTemplatePrefix = "account_workbench.template."

func (s *Store) WorkbenchTemplates(ctx context.Context, targetURL string) ([]WorkbenchTemplate, error) {
	target, err := workbenchTarget(targetURL)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT value FROM settings WHERE key LIKE ? ORDER BY key`, workbenchTargetPrefix(target)+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []WorkbenchTemplate{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		item, err := decodeWorkbenchTemplate(raw, target)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Priority != result[j].Priority {
			return result[i].Priority > result[j].Priority
		}
		return result[i].ID < result[j].ID
	})
	return result, rows.Err()
}

func (s *Store) WorkbenchTemplate(ctx context.Context, targetURL, id string) (WorkbenchTemplate, error) {
	target, err := workbenchTarget(targetURL)
	if err != nil {
		return WorkbenchTemplate{}, err
	}
	if !workbenchIdentifier.MatchString(id) {
		return WorkbenchTemplate{}, ErrWorkbenchTemplateConflict
	}
	var raw string
	err = s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, workbenchTargetPrefix(target)+id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return WorkbenchTemplate{}, ErrWorkbenchTemplateConflict
	}
	if err != nil {
		return WorkbenchTemplate{}, err
	}
	return decodeWorkbenchTemplate(raw, target)
}

// SaveWorkbenchTemplate compares the supplied revision atomically; the stored
// revision is incremented only after a successful write.
func (s *Store) SaveWorkbenchTemplate(ctx context.Context, item WorkbenchTemplate) error {
	item, err := validateWorkbenchTemplate(item)
	if err != nil {
		return err
	}
	previous := item.Revision
	item.Revision++
	raw, err := json.Marshal(item)
	if err != nil {
		return err
	}
	key := workbenchTargetPrefix(item.TargetURL) + item.ID
	s.workbenchTemplateWriteMu.Lock()
	defer s.workbenchTemplateWriteMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `INSERT INTO settings(key,value) SELECT ?,? WHERE ?=0 OR EXISTS(SELECT 1 FROM settings WHERE key=?)
 ON CONFLICT(key) DO UPDATE SET value=excluded.value WHERE json_extract(settings.value,'$.revision')=?`, key, string(raw), previous, key, previous)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrWorkbenchTemplateConflict
	}
	if item.Preferred {
		if err := clearOtherWorkbenchPreferences(ctx, tx, item); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) DeleteWorkbenchTemplate(ctx context.Context, targetURL, id string, revision int64) error {
	target, err := workbenchTarget(targetURL)
	if err != nil {
		return err
	}
	if !workbenchIdentifier.MatchString(id) || revision < 1 {
		return ErrWorkbenchTemplateConflict
	}
	s.workbenchTemplateWriteMu.Lock()
	defer s.workbenchTemplateWriteMu.Unlock()
	result, err := s.db.ExecContext(ctx, `DELETE FROM settings WHERE key=? AND json_extract(value,'$.revision')=?`, workbenchTargetPrefix(target)+id, revision)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrWorkbenchTemplateConflict
	}
	return nil
}

func workbenchTarget(raw string) (string, error) {
	value, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || value.Hostname() == "" || (value.Scheme != "http" && value.Scheme != "https") || value.User != nil || value.RawQuery != "" || value.Fragment != "" {
		return "", errors.New("账号模板管理目标地址无效")
	}
	value.Host = strings.ToLower(value.Host)
	value.Path = strings.TrimSuffix(strings.TrimRight(value.Path, "/"), "/api/v1/admin")
	value.RawPath = ""
	return strings.TrimRight(value.String(), "/"), nil
}

func workbenchTargetPrefix(target string) string {
	hash := sha256.Sum256([]byte(target))
	return workbenchTemplatePrefix + hex.EncodeToString(hash[:]) + "."
}

func decodeWorkbenchTemplate(raw, target string) (WorkbenchTemplate, error) {
	var item WorkbenchTemplate
	if err := json.Unmarshal([]byte(raw), &item); err != nil {
		return item, errors.New("账号模板记录损坏，请重新保存模板")
	}
	if item.TargetURL != target {
		return WorkbenchTemplate{}, ErrWorkbenchTemplateConflict
	}
	return validateWorkbenchTemplate(item)
}

func validateWorkbenchTemplate(item WorkbenchTemplate) (WorkbenchTemplate, error) {
	if !workbenchIdentifier.MatchString(item.ID) || item.Revision < 0 || item.Revision >= 1<<53 {
		return item, errors.New("账号模板 ID 或版本无效")
	}
	item.Name = strings.TrimSpace(item.Name)
	if item.Name == "" || textLength(item.Name) > 255 || strings.ContainsAny(item.Name, "\r\n\x00") {
		return item, errors.New("账号模板名称需要 1～255 个字符")
	}
	if item.Priority < 0 || item.Priority > 1_000_000 {
		return item, errors.New("账号模板匹配优先级需要在 0～1000000 之间")
	}
	target, err := workbenchTarget(item.TargetURL)
	if err != nil {
		return item, err
	}
	item.TargetURL = target
	if item.SourceAccountID != "" {
		id, err := workbenchInteger(item.SourceAccountID, 1)
		if err != nil || strconv.FormatInt(id, 10) != item.SourceAccountID {
			return item, errors.New("来源账号 ID 无效")
		}
		if len(item.SourceRevision) != 64 || strings.Trim(item.SourceRevision, "0123456789abcdef") != "" {
			return item, errors.New("来源账号配置版本无效，请重新提取预览")
		}
		if _, err := time.Parse(time.RFC3339Nano, item.SourceSyncedAt); err != nil {
			return item, errors.New("来源同步时间无效，请重新提取预览")
		}
	} else if item.SourceName != "" || item.SourceRevision != "" || item.SourceSyncedAt != "" {
		return item, errors.New("来源元数据缺少稳定账号 ID")
	}
	if textLength(item.SourceName) > 255 || strings.ContainsAny(item.SourceName, "\r\n\x00") {
		return item, errors.New("来源账号名称过长")
	}
	item.Match.PlanType = strings.ToLower(strings.TrimSpace(item.Match.PlanType))
	item.Match.EmailDomain = strings.ToLower(strings.TrimSpace(item.Match.EmailDomain))
	if len(item.Match.PlanType) > 64 || strings.ContainsAny(item.Match.PlanType, "\r\n\x00") {
		return item, errors.New("套餐匹配条件无效")
	}
	if item.Match.EmailDomain != "" && (len(item.Match.EmailDomain) > 253 || !workbenchDomain.MatchString(item.Match.EmailDomain) || strings.Contains(item.Match.EmailDomain, "..")) {
		return item, errors.New("邮箱域名匹配条件无效")
	}
	item.Config, err = NormalizeWorkbenchTemplateConfig(item.Config)
	return item, err
}

// NormalizeWorkbenchTemplateConfig is the shared boundary for extracted and
// manually edited templates. Decimal values remain decimal JSON numbers.
func NormalizeWorkbenchTemplateConfig(config WorkbenchTemplateConfig) (WorkbenchTemplateConfig, error) {
	if len(config) > 20 {
		return nil, errors.New("账号模板包含过多配置字段")
	}
	result := WorkbenchTemplateConfig{}
	for key, raw := range config {
		if len(raw) > 256*1024 {
			return nil, fmt.Errorf("账号模板字段 %s 过大", key)
		}
		value, err := workbenchJSON(raw)
		if err != nil {
			return nil, fmt.Errorf("账号模板字段 %s 不是有效 JSON", key)
		}
		normalized, err := normalizeWorkbenchField(key, value)
		if err != nil {
			return nil, fmt.Errorf("账号模板字段 %s 无效：%w", key, err)
		}
		result[key], err = json.Marshal(normalized)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func workbenchJSON(raw []byte) (any, error) {
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, errors.New("JSON 包含多余内容")
	}
	return value, nil
}

func normalizeWorkbenchField(key string, value any) (any, error) {
	switch key {
	case "concurrency":
		return workbenchInteger(value, 1)
	case "priority":
		return workbenchInteger(value, 0)
	case "proxy_id":
		if value == nil {
			return nil, nil
		}
		return workbenchInteger(value, 1)
	case "rate_multiplier", "load_factor":
		if value == nil && key == "load_factor" {
			return nil, nil
		}
		return workbenchDecimal(value)
	case "group_ids":
		if value == nil {
			return []int64{}, nil
		}
		values, ok := value.([]any)
		if !ok || len(values) > 1000 {
			return nil, errors.New("需要不超过 1000 个稳定分组 ID")
		}
		ids := make([]int64, 0, len(values))
		seen := map[int64]bool{}
		for _, raw := range values {
			id, err := workbenchInteger(raw, 1)
			if err != nil {
				return nil, err
			}
			if !seen[id] {
				ids = append(ids, id)
				seen[id] = true
			}
		}
		return ids, nil
	case "auto_pause_on_expired", "upstream_billing_probe_enabled", "confirm_mixed_channel_risk":
		if flag, ok := value.(bool); ok {
			return flag, nil
		}
		return nil, errors.New("需要布尔值")
	case "notes":
		return workbenchText(value, 16384)
	case "expires_at":
		if value == nil {
			return nil, nil
		}
		if text, ok := value.(string); ok {
			if _, err := time.Parse(time.RFC3339, text); err == nil {
				return text, nil
			}
		}
		return workbenchInteger(value, 0)
	case "credential_extras":
		return workbenchNestedConfig(value, true)
	case "extra":
		return workbenchNestedConfig(value, false)
	default:
		return nil, errors.New("不允许复制此配置字段")
	}
}

func workbenchInteger(value any, minimum int64) (int64, error) {
	var raw string
	switch number := value.(type) {
	case json.Number:
		raw = number.String()
	case string:
		raw = strings.TrimSpace(number)
	case int:
		raw = strconv.Itoa(number)
	case int64:
		raw = strconv.FormatInt(number, 10)
	default:
		return 0, errors.New("需要十进制整数或整数字符串")
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || parsed < minimum || parsed > 9_007_199_254_740_991 {
		return 0, errors.New("整数超出允许范围")
	}
	return parsed, nil
}

func workbenchDecimal(value any) (json.Number, error) {
	var raw string
	switch number := value.(type) {
	case json.Number:
		raw = number.String()
	case string:
		raw = strings.TrimSpace(number)
	case int:
		raw = strconv.Itoa(number)
	case int64:
		raw = strconv.FormatInt(number, 10)
	default:
		return "", errors.New("倍率必须使用十进制数字或字符串")
	}
	parsed, ok := decimalutil.Parse(raw)
	if !ok || parsed.Sign() < 0 {
		return "", errors.New("倍率必须是非负十进制数")
	}
	// JSON numeric syntax is intentionally narrower than math/big syntax.
	if !json.Valid([]byte(raw)) || strings.HasPrefix(raw, "\"") {
		return "", errors.New("倍率不是有效十进制 JSON 数字")
	}
	return json.Number(raw), nil
}

func workbenchText(value any, maximum int) (string, error) {
	text, ok := value.(string)
	if !ok || len(text) > maximum || strings.ContainsRune(text, '\x00') {
		return "", errors.New("文本类型或长度无效")
	}
	return text, nil
}

func workbenchNestedConfig(value any, credentials bool) (map[string]any, error) {
	fields, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("需要配置对象")
	}
	result := map[string]any{}
	for key, value := range fields {
		var normalized any
		var err error
		if credentials {
			switch key {
			case "model_mapping", "compact_model_mapping":
				normalized, err = workbenchModelMap(value)
			case "intercept_warmup_requests":
				normalized, ok = value.(bool)
				if !ok {
					err = errors.New("需要布尔值")
				}
			case "user_agent":
				normalized, err = workbenchText(value, 2048)
				if text, _ := normalized.(string); strings.ContainsAny(text, "\r\n") {
					err = errors.New("User-Agent 不能包含换行")
				}
			default:
				return nil, errors.New("凭据附加配置包含不允许复制的字段")
			}
		} else {
			switch key {
			case "openai_long_context_billing_enabled", "openai_oauth_responses_websockets_v2_enabled", "openai_responses_flatten_namespaces", "openai_ws_allow_store_recovery", "openai_ws_enabled", "openai_ws_force_http":
				normalized, ok = value.(bool)
				if !ok {
					err = errors.New("需要布尔值")
				}
			case "openai_oauth_responses_websockets_v2_mode":
				normalized, err = workbenchText(value, 128)
			case "model_rate_limits":
				normalized = value
				err = workbenchRateLimits(value, 0)
			default:
				return nil, errors.New("附加配置包含不允许复制的字段")
			}
		}
		if err != nil {
			return nil, fmt.Errorf("%s：%w", key, err)
		}
		result[key] = normalized
	}
	return result, nil
}

func workbenchModelMap(value any) (map[string]string, error) {
	fields, ok := value.(map[string]any)
	if !ok || len(fields) > 1000 {
		return nil, errors.New("模型映射需要不超过 1000 项的字符串对象")
	}
	result := map[string]string{}
	for key, raw := range fields {
		text, err := workbenchText(raw, 1000)
		if err != nil || key == "" || len(key) > 1000 {
			return nil, errors.New("模型映射的名称及目标必须是有效字符串")
		}
		result[key] = text
	}
	return result, nil
}

func workbenchRateLimits(value any, depth int) error {
	if depth > 8 {
		return errors.New("模型限速配置嵌套过深")
	}
	switch node := value.(type) {
	case map[string]any:
		if len(node) > 1000 {
			return errors.New("模型限速配置过大")
		}
		for key, child := range node {
			if strings.ContainsAny(key, "\r\n\x00") || len(key) > 1000 {
				return errors.New("模型限速配置字段无效")
			}
			lower := strings.ToLower(key)
			if strings.Contains(lower, "token") && lower != "tokens" && lower != "tokens_per_minute" && lower != "max_tokens" {
				return errors.New("模型限速配置不能包含凭据")
			}
			if strings.Contains(lower, "password") || strings.Contains(lower, "secret") || lower == "email" || lower == "account_id" || lower == "user_id" {
				return errors.New("模型限速配置不能包含身份或凭据")
			}
			if err := workbenchRateLimits(child, depth+1); err != nil {
				return err
			}
		}
	case []any:
		if len(node) > 1000 {
			return errors.New("模型限速配置过大")
		}
		for _, child := range node {
			if err := workbenchRateLimits(child, depth+1); err != nil {
				return err
			}
		}
	case json.Number:
		if _, err := workbenchDecimal(node); err != nil {
			return err
		}
	case string:
		if _, err := workbenchText(node, 1000); err != nil {
			return err
		}
	case bool, nil:
	default:
		return errors.New("模型限速配置类型无效")
	}
	if depth == 0 {
		if _, ok := value.(map[string]any); !ok {
			return errors.New("模型限速配置必须为对象")
		}
	}
	return nil
}
