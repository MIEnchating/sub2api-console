package configstore

import (
	"context"
	"database/sql"
	"errors"
)

// UptimeKumaConfig is private backend storage. Never serialize it into API responses.
type UptimeKumaConfig struct {
	BaseURL, APIKey, Username, Password, Token string
	Revision                                   int64
}

var ErrKumaConfigConflict = errors.New("接入配置已更改，请刷新后重试")

func (s *Store) UptimeKuma(ctx context.Context) (UptimeKumaConfig, error) {
	var item UptimeKumaConfig
	err := s.db.QueryRowContext(ctx, `SELECT base_url,api_key,username,password,token,revision FROM uptime_kuma_config WHERE id=1`).Scan(
		&item.BaseURL, &item.APIKey, &item.Username, &item.Password, &item.Token, &item.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		return item, nil
	}
	return item, err
}

func (s *Store) SaveUptimeKuma(ctx context.Context, item UptimeKumaConfig) error {
	result, err := s.db.ExecContext(ctx, `INSERT INTO uptime_kuma_config(id,base_url,api_key,username,password,token,revision)
		SELECT 1,?,?,?,?,?,? WHERE ?=0 OR EXISTS(SELECT 1 FROM uptime_kuma_config WHERE id=1)
		ON CONFLICT(id) DO UPDATE SET base_url=excluded.base_url,api_key=excluded.api_key,
		username=excluded.username,password=excluded.password,token=excluded.token,revision=excluded.revision
		WHERE uptime_kuma_config.revision=?`, item.BaseURL, item.APIKey, item.Username, item.Password, item.Token, item.Revision+1, item.Revision, item.Revision)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrKumaConfigConflict
	}
	return nil
}
