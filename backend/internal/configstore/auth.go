package configstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"golang.org/x/net/http/httpguts"
)

const (
	maximumCustomHeaderCount      = 100
	maximumCustomHeaderNameBytes  = 255
	maximumCustomHeaderValueBytes = 64 << 10
	maximumCustomHeaderTotalBytes = 128 << 10
)

var forbiddenCustomHeaders = map[string]struct{}{
	"Connection":          {},
	"Content-Length":      {},
	"Cookie":              {},
	"Host":                {},
	"Keep-Alive":          {},
	"Proxy-Authorization": {},
	"Proxy-Connection":    {},
	"Te":                  {},
	"Trailer":             {},
	"Transfer-Encoding":   {},
	"Upgrade":             {},
}

type AuthRecord struct {
	Host         string
	BaseURL      string
	UpstreamType string
	AuthMode     string
	AccessToken  *string
	RefreshToken *string
	AdminKey     *string
	UserID       *string
	Headers      map[string]string
	Cookies      map[string]string
}

type AuthRecordSummary struct {
	Host            string   `json:"host"`
	BaseURL         string   `json:"base_url"`
	UpstreamType    string   `json:"upstream_type"`
	AuthMode        string   `json:"auth_mode"`
	HasAccessToken  bool     `json:"has_access_token"`
	HasRefreshToken bool     `json:"has_refresh_token"`
	HasAdminKey     bool     `json:"has_admin_key"`
	HasUserID       bool     `json:"has_user_id"`
	HeaderNames     []string `json:"header_names"`
	CookieNames     []string `json:"cookie_names"`
	UpdatedAt       string   `json:"updated_at"`
}

func CanonicalHost(value string) string {
	normalized := strings.TrimRight(strings.TrimSpace(value), "/")
	if strings.Contains(normalized, "://") {
		if parsed, err := url.Parse(normalized); err == nil {
			if parsed.Host != "" {
				normalized = parsed.Host
			} else {
				normalized = parsed.Path
			}
		}
	}
	return strings.ToLower(strings.TrimRight(normalized, "/"))
}

func (s *Store) AuthRecord(ctx context.Context, host string) (*AuthRecord, error) {
	normalizedHost := CanonicalHost(host)
	if normalizedHost == "" {
		return nil, errors.New("上游 Host 不能为空")
	}
	var record AuthRecord
	var accessToken, refreshToken, adminKey, userID sql.NullString
	var rawHeaders, rawCookies string
	err := s.db.QueryRowContext(ctx, `SELECT host,base_url,upstream_type,auth_mode,access_token,refresh_token,
		admin_key,user_id,headers_json,cookies_json FROM auth_records WHERE host=?`, normalizedHost).Scan(
		&record.Host, &record.BaseURL, &record.UpstreamType, &record.AuthMode, &accessToken, &refreshToken,
		&adminKey, &userID, &rawHeaders, &rawCookies,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	record.AccessToken, record.RefreshToken, record.AdminKey, record.UserID = nullableText(accessToken), nullableText(refreshToken), nullableText(adminKey), nullableText(userID)
	record.Headers, err = decodeStringMap(rawHeaders)
	if err != nil {
		return nil, fmt.Errorf("上游 %s 的授权 Headers 记录损坏", normalizedHost)
	}
	record.Cookies, err = decodeStringMap(rawCookies)
	if err != nil {
		return nil, fmt.Errorf("上游 %s 的授权 Cookie 记录损坏", normalizedHost)
	}
	return &record, nil
}

func (s *Store) SaveAuthRecord(ctx context.Context, record AuthRecord, present map[string]bool) error {
	record.Host = CanonicalHost(record.Host)
	if record.Host == "" {
		return errors.New("授权记录必须包含 Host")
	}
	current, err := s.AuthRecord(ctx, record.Host)
	if err != nil {
		return err
	}
	if current != nil && present != nil {
		mergeAuthRecord(&record, *current, present)
	}
	baseURL, err := ValidateBaseURL(record.BaseURL)
	if err != nil {
		return errors.New("授权记录必须包含有效 Base URL")
	}
	record.BaseURL = baseURL
	record.UpstreamType = strings.ToLower(strings.TrimSpace(record.UpstreamType))
	record.AuthMode = strings.TrimSpace(record.AuthMode)
	if record.UpstreamType == "" || record.AuthMode == "" {
		return errors.New("授权记录必须包含平台类型和鉴权方式")
	}
	headers, err := normalizedHeaders(record.Headers)
	if err != nil {
		return err
	}
	cookies, err := normalizedCookies(record.Cookies)
	if err != nil {
		return err
	}
	rawHeaders, _ := json.Marshal(headers)
	rawCookies, _ := json.Marshal(cookies)
	_, err = s.db.ExecContext(ctx, `INSERT INTO auth_records(
		host,base_url,upstream_type,auth_mode,access_token,refresh_token,admin_key,user_id,headers_json,cookies_json,updated_at
	) VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(host) DO UPDATE SET
		base_url=excluded.base_url,upstream_type=excluded.upstream_type,auth_mode=excluded.auth_mode,
		access_token=excluded.access_token,refresh_token=excluded.refresh_token,admin_key=excluded.admin_key,
		user_id=excluded.user_id,headers_json=excluded.headers_json,cookies_json=excluded.cookies_json,updated_at=excluded.updated_at`,
		record.Host, record.BaseURL, record.UpstreamType, record.AuthMode, record.AccessToken, record.RefreshToken,
		record.AdminKey, record.UserID, string(rawHeaders), string(rawCookies), time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *Store) DeleteAuthRecord(ctx context.Context, host string) (bool, error) {
	host = CanonicalHost(host)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `DELETE FROM auth_records WHERE host=?`, host)
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM upstream_key_secrets WHERE host=?`, host); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM auth_recovery_preferences WHERE host=?`, host); err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Store) AuthRecordIndex(ctx context.Context) ([]AuthRecordSummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT host,base_url,upstream_type,auth_mode,access_token,refresh_token,
		admin_key,user_id,headers_json,cookies_json,updated_at FROM auth_records ORDER BY host`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []AuthRecordSummary{}
	for rows.Next() {
		var item AuthRecordSummary
		var accessToken, refreshToken, adminKey, userID sql.NullString
		var rawHeaders, rawCookies string
		if err := rows.Scan(&item.Host, &item.BaseURL, &item.UpstreamType, &item.AuthMode, &accessToken, &refreshToken, &adminKey, &userID, &rawHeaders, &rawCookies, &item.UpdatedAt); err != nil {
			return nil, err
		}
		headers, err := decodeStringMap(rawHeaders)
		if err != nil {
			return nil, fmt.Errorf("上游 %s 的授权 Headers 记录损坏", item.Host)
		}
		cookies, err := decodeStringMap(rawCookies)
		if err != nil {
			return nil, fmt.Errorf("上游 %s 的授权 Cookie 记录损坏", item.Host)
		}
		item.HasAccessToken, item.HasRefreshToken = nonempty(accessToken), nonempty(refreshToken)
		item.HasAdminKey, item.HasUserID = nonempty(adminKey), nonempty(userID)
		item.HeaderNames, item.CookieNames = sortedMapKeys(headers), sortedMapKeys(cookies)
		result = append(result, item)
	}
	return result, rows.Err()
}

func mergeAuthRecord(target *AuthRecord, current AuthRecord, present map[string]bool) {
	if !present["base_url"] {
		target.BaseURL = current.BaseURL
	}
	if !present["upstream_type"] {
		target.UpstreamType = current.UpstreamType
	}
	if !present["auth_mode"] {
		target.AuthMode = current.AuthMode
	}
	if !present["access_token"] {
		target.AccessToken = current.AccessToken
	}
	if !present["refresh_token"] {
		target.RefreshToken = current.RefreshToken
	}
	if !present["admin_key"] {
		target.AdminKey = current.AdminKey
	}
	if !present["user_id"] {
		target.UserID = current.UserID
	}
	if !present["headers"] {
		target.Headers = current.Headers
	}
	if !present["cookies"] {
		target.Cookies = current.Cookies
	}
}

func normalizedHeaders(values map[string]string) (map[string]string, error) {
	if len(values) > maximumCustomHeaderCount {
		return nil, errors.New("自定义 Header 数量不能超过 100")
	}
	result := make(map[string]string, len(values))
	totalBytes := 0
	for key, value := range values {
		key = strings.TrimSpace(key)
		if !httpguts.ValidHeaderFieldName(key) || !httpguts.ValidHeaderFieldValue(value) ||
			len(key) > maximumCustomHeaderNameBytes || len(value) > maximumCustomHeaderValueBytes {
			return nil, errors.New("自定义 Header 名称或值无效")
		}
		canonicalKey := http.CanonicalHeaderKey(key)
		if _, forbidden := forbiddenCustomHeaders[canonicalKey]; forbidden {
			return nil, errors.New("自定义 Header 不能覆盖 HTTP 传输层保留字段")
		}
		if _, duplicate := result[canonicalKey]; duplicate {
			return nil, errors.New("自定义 Header 名称不能重复")
		}
		totalBytes += len(canonicalKey) + len(value) + 4
		if totalBytes > maximumCustomHeaderTotalBytes {
			return nil, errors.New("自定义 Header 总大小不能超过 128 KiB")
		}
		result[canonicalKey] = value
	}
	return result, nil
}

func normalizedCookies(values map[string]string) (map[string]string, error) {
	if len(values) > maximumCustomHeaderCount {
		return nil, errors.New("自定义 Cookie 数量不能超过 100")
	}
	result := make(map[string]string, len(values))
	totalBytes := 0
	for key, value := range values {
		key = strings.TrimSpace(key)
		cookie := http.Cookie{Name: key, Value: value}
		if cookie.Valid() != nil || len(key) > maximumCustomHeaderNameBytes || len(value) > maximumCustomHeaderValueBytes {
			return nil, errors.New("自定义 Cookie 名称或值无效")
		}
		if _, duplicate := result[key]; duplicate {
			return nil, errors.New("自定义 Cookie 名称不能重复")
		}
		totalBytes += len(key) + len(value) + 2
		if totalBytes > maximumCustomHeaderTotalBytes {
			return nil, errors.New("自定义 Cookie 总大小不能超过 128 KiB")
		}
		result[key] = value
	}
	return result, nil
}

func decodeStringMap(raw string) (map[string]string, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	var value map[string]string
	if err := decoder.Decode(&value); err != nil || value == nil {
		return nil, errors.New("JSON 字符串对象无效")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, errors.New("JSON 包含尾随数据")
	}
	return value, nil
}

func nullableText(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func nonempty(value sql.NullString) bool { return value.Valid && strings.TrimSpace(value.String) != "" }

func sortedMapKeys(values map[string]string) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
