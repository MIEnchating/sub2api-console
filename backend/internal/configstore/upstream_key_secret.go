package configstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type UpstreamKeySecret struct {
	Host      string
	KeyID     string
	GroupID   string
	Secret    string
	UpdatedAt string
}

var ErrUpstreamKeyAuthChanged = errors.New("上游授权已变化，请重新读取绑定 Key")

// SaveUpstreamKeySecretForAuth only saves a revealed key while the private
// authorization used to read it still matches. One statement serializes this
// check and write with authorization replacement, host migration and deletion.
func (s *Store) SaveUpstreamKeySecretForAuth(ctx context.Context, value UpstreamKeySecret, expected AuthRecord) error {
	value, err := normalizedUpstreamKeySecret(value)
	if err != nil {
		return err
	}
	expected.Host = CanonicalHost(expected.Host)
	if expected.Host != value.Host {
		return ErrUpstreamKeyAuthChanged
	}
	expected.BaseURL, err = ValidateBaseURL(expected.BaseURL)
	if err != nil {
		return ErrUpstreamKeyAuthChanged
	}
	expected.UpstreamType = strings.ToLower(strings.TrimSpace(expected.UpstreamType))
	expected.AuthMode = strings.TrimSpace(expected.AuthMode)
	headers, err := normalizedHeaders(expected.Headers)
	if err != nil {
		return ErrUpstreamKeyAuthChanged
	}
	cookies, err := normalizedCookies(expected.Cookies)
	if err != nil {
		return ErrUpstreamKeyAuthChanged
	}
	rawHeaders, _ := json.Marshal(headers)
	rawCookies, _ := json.Marshal(cookies)
	result, err := s.db.ExecContext(ctx, `INSERT INTO upstream_key_secrets(host,key_id,group_id,secret,updated_at)
		SELECT ?,?,?,?,? FROM auth_records
		WHERE host=? AND base_url=? AND upstream_type=? AND auth_mode=?
		AND access_token IS ? AND refresh_token IS ? AND admin_key IS ? AND user_id IS ?
		AND headers_json=? AND cookies_json=?
		ON CONFLICT(host,key_id,group_id) DO UPDATE SET secret=excluded.secret,updated_at=excluded.updated_at`,
		value.Host, value.KeyID, value.GroupID, value.Secret, value.UpdatedAt,
		expected.Host, expected.BaseURL, expected.UpstreamType, expected.AuthMode,
		expected.AccessToken, expected.RefreshToken, expected.AdminKey, expected.UserID,
		string(rawHeaders), string(rawCookies),
	)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrUpstreamKeyAuthChanged
	}
	return nil
}

func (s *Store) UpstreamKeySecret(ctx context.Context, host, keyID, groupID string) (*UpstreamKeySecret, error) {
	host = CanonicalHost(host)
	keyID = strings.TrimSpace(keyID)
	groupID = strings.TrimSpace(groupID)
	if host == "" || keyID == "" || groupID == "" {
		return nil, errors.New("本地 Key 查询必须包含 Host、Key ID 和 Group ID")
	}
	var result UpstreamKeySecret
	err := s.db.QueryRowContext(ctx, `SELECT host,key_id,group_id,secret,updated_at
		FROM upstream_key_secrets WHERE host=? AND key_id=? AND group_id=?`, host, keyID, groupID).Scan(
		&result.Host, &result.KeyID, &result.GroupID, &result.Secret, &result.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *Store) SaveUpstreamKeySecret(ctx context.Context, value UpstreamKeySecret) error {
	value, err := normalizedUpstreamKeySecret(value)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO upstream_key_secrets(host,key_id,group_id,secret,updated_at)
		VALUES(?,?,?,?,?) ON CONFLICT(host,key_id,group_id) DO UPDATE SET
		secret=excluded.secret,updated_at=excluded.updated_at`,
		value.Host, value.KeyID, value.GroupID, value.Secret, value.UpdatedAt,
	)
	return err
}

func normalizedUpstreamKeySecret(value UpstreamKeySecret) (UpstreamKeySecret, error) {
	value.Host = CanonicalHost(value.Host)
	value.KeyID = strings.TrimSpace(value.KeyID)
	value.GroupID = strings.TrimSpace(value.GroupID)
	value.Secret = strings.TrimSpace(value.Secret)
	if value.Host == "" || value.KeyID == "" || value.GroupID == "" || value.Secret == "" {
		return UpstreamKeySecret{}, errors.New("本地 Key 必须包含 Host、Key ID、Group ID 和密钥")
	}
	if textLength(value.KeyID) > 255 || textLength(value.GroupID) > 255 || textLength(value.Secret) > 65536 {
		return UpstreamKeySecret{}, errors.New("本地 Key 字段过长")
	}
	value.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return value, nil
}

func (s *Store) DeleteUpstreamKeySecrets(ctx context.Context, host, keyID string) error {
	host = CanonicalHost(host)
	keyID = strings.TrimSpace(keyID)
	if host == "" || keyID == "" {
		return errors.New("本地 Key 删除必须包含 Host 和 Key ID")
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM upstream_key_secrets WHERE host=? AND key_id=?`, host, keyID)
	return err
}
