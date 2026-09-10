package configstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// KumaTemplate contains private request credentials; API handlers must return a summary.
type KumaTemplate struct {
	BodyEncoding   string                  `json:"body_encoding,omitempty"`
	Monitoring     *KumaTemplateMonitoring `json:"monitoring,omitempty"`
	RequestProfile string                  `json:"request_profile,omitempty"`
	Model          string                  `json:"model,omitempty"`
	ID             string                  `json:"id"`
	Revision       int64                   `json:"revision"`
	Name           string                  `json:"name"`
	Method         string                  `json:"method"`
	Headers        string                  `json:"headers"`
	Body           string                  `json:"body"`
	AuthMethod     string                  `json:"auth_method"`
	AuthUsername   string                  `json:"auth_username"`
	AuthPassword   string                  `json:"auth_password"`
}

type KumaTemplateMonitoring struct {
	Type                string   `json:"type"`
	URL                 string   `json:"url"`
	Interval            int      `json:"interval"`
	Timeout             int      `json:"timeout"`
	RetryInterval       int      `json:"retry_interval"`
	MaxRetries          int      `json:"max_retries"`
	MaxRedirects        int      `json:"max_redirects"`
	AcceptedStatusCodes []string `json:"accepted_status_codes"`
	IgnoreTLS           bool     `json:"ignore_tls"`
	UpsideDown          bool     `json:"upside_down"`
	Hostname            string   `json:"hostname"`
	Port                int      `json:"port"`
	Keyword             string   `json:"keyword"`
	DNSRecordType       string   `json:"dns_record_type"`
	DNSResolver         string   `json:"dns_resolver"`
}

// SeedKumaTemplate installs a default only once, including after the user deletes it.
func (s *Store) SeedKumaTemplate(ctx context.Context, item KumaTemplate) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	marker := "uptime_kuma.seed." + item.ID
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO settings(key,value) VALUES(?, '1')`, marker)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n > 0 {
		item.Revision = 1
		raw, err := json.Marshal(item)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO settings(key,value) VALUES(?,?)`, kumaTemplatePrefix+item.ID, string(raw)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

var ErrKumaTemplateConflict = errors.New("模板已被修改或删除，请刷新后重试")

const kumaTemplatePrefix = "uptime_kuma.template."

func (s *Store) KumaTemplates(ctx context.Context) ([]KumaTemplate, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT value FROM settings WHERE key LIKE 'uptime_kuma.template.%' ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []KumaTemplate{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var item KumaTemplate
		if err = json.Unmarshal([]byte(raw), &item); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
func (s *Store) KumaTemplate(ctx context.Context, id string) (KumaTemplate, error) {
	var raw string
	var item KumaTemplate
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, kumaTemplatePrefix+id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return item, ErrKumaTemplateConflict
	}
	if err != nil {
		return item, err
	}
	err = json.Unmarshal([]byte(raw), &item)
	return item, err
}
func (s *Store) SaveKumaTemplate(ctx context.Context, item KumaTemplate) error {
	previous := item.Revision
	item.Revision++
	raw, err := json.Marshal(item)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO settings(key,value) SELECT ?,? WHERE ?=0 OR EXISTS(SELECT 1 FROM settings WHERE key=?)
 ON CONFLICT(key) DO UPDATE SET value=excluded.value WHERE json_extract(settings.value,'$.revision')=?`, kumaTemplatePrefix+item.ID, string(raw), previous, kumaTemplatePrefix+item.ID, previous)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrKumaTemplateConflict
	}
	return nil
}
func (s *Store) DeleteKumaTemplate(ctx context.Context, id string, revision int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM settings WHERE key=? AND json_extract(value,'$.revision')=?`, kumaTemplatePrefix+id, revision)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrKumaTemplateConflict
	}
	return nil
}
