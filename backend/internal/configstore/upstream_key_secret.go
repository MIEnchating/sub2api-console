package configstore

import (
	"context"
	"database/sql"
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
	value.Host = CanonicalHost(value.Host)
	value.KeyID = strings.TrimSpace(value.KeyID)
	value.GroupID = strings.TrimSpace(value.GroupID)
	value.Secret = strings.TrimSpace(value.Secret)
	if value.Host == "" || value.KeyID == "" || value.GroupID == "" || value.Secret == "" {
		return errors.New("本地 Key 必须包含 Host、Key ID、Group ID 和密钥")
	}
	if len(value.KeyID) > 255 || len(value.GroupID) > 255 || len(value.Secret) > 65536 {
		return errors.New("本地 Key 字段过长")
	}
	value.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `INSERT INTO upstream_key_secrets(host,key_id,group_id,secret,updated_at)
		VALUES(?,?,?,?,?) ON CONFLICT(host,key_id,group_id) DO UPDATE SET
		secret=excluded.secret,updated_at=excluded.updated_at`,
		value.Host, value.KeyID, value.GroupID, value.Secret, value.UpdatedAt,
	)
	return err
}
