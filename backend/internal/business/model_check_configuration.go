package business

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const modelCheckConfigurationKey = "model-check-configuration"

func (s *Store) LoadModelCheckConfiguration(ctx context.Context) ([]byte, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value_json FROM app_state WHERE key=?`, modelCheckConfigurationKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return []byte(raw), nil
}

func (s *Store) SaveModelCheckConfiguration(ctx context.Context, raw []byte, actor, action string) error {
	if !json.Valid(raw) {
		return errors.New("模型检测画像配置不是有效 JSON")
	}
	action = strings.TrimSpace(action)
	if action == "" {
		return errors.New("模型检测画像配置操作不能为空")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var previous string
	readErr := tx.QueryRowContext(ctx, `SELECT value_json FROM app_state WHERE key=?`, modelCheckConfigurationKey).Scan(&previous)
	if readErr != nil && !errors.Is(readErr, sql.ErrNoRows) {
		return readErr
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT INTO app_state(key,value_json,updated_at) VALUES(?,?,?)
		ON CONFLICT(key) DO UPDATE SET value_json=excluded.value_json,updated_at=excluded.updated_at`,
		modelCheckConfigurationKey, string(raw), now); err != nil {
		return err
	}
	before := configurationAuditValue(previous)
	after := configurationAuditValue(string(raw))
	var minimum sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MIN(source_id) FROM operation_audit WHERE source_id < 0`).Scan(&minimum); err != nil {
		return err
	}
	sourceID := int64(-1)
	if minimum.Valid && minimum.Int64 <= -1 {
		sourceID = minimum.Int64 - 1
	}
	operationID := fmt.Sprintf("model-check-profile-%d", time.Now().UnixNano())
	if _, err := tx.ExecContext(ctx, `INSERT INTO operation_audit(
		source_id,operation_id,operation_type,state,phase,actor,source,remote_confirmed,readback_confirmed,
		object_type,object_id,object_name,group_names_json,field_name,before_json,after_json,writeback,created_at
	) VALUES(?,?,?,'succeeded',?,?, 'console',0,1,'model-check-profile','active','模型检测画像','[]','configuration',?,?,0,?)`,
		sourceID, operationID, "model-check.profile."+action, action, strings.TrimSpace(actor), before, after, now); err != nil {
		return err
	}
	return tx.Commit()
}

func configurationAuditValue(raw string) any {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	digest := sha256.Sum256([]byte(raw))
	encoded, _ := json.Marshal(map[string]any{
		"configured":  true,
		"fingerprint": hex.EncodeToString(digest[:]),
		"bytes":       len(raw),
	})
	return string(encoded)
}
