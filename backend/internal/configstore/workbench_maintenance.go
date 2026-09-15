package configstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

type WorkbenchMaintenance struct {
	Enabled                 bool     `json:"enabled"`
	ReauthorizeWithProfiles bool     `json:"reauthorize_with_profiles"`
	IntervalMinutes         int      `json:"interval_minutes"`
	CooldownMinutes         int      `json:"cooldown_minutes"`
	GroupIDs                []string `json:"group_ids"`
	CheckAfterRepair        bool     `json:"check_after_repair"`
	Model                   string   `json:"model"`
	Revision                int64    `json:"revision"`
}

type WorkbenchMaintenanceRuntime struct {
	LastRunAt  string            `json:"last_run_at,omitempty"`
	LastTaskID string            `json:"last_task_id,omitempty"`
	Cooldowns  map[string]string `json:"cooldowns"`
}

func workbenchMaintenanceKey(target, suffix string) string {
	digest := sha256.Sum256([]byte(strings.TrimRight(strings.TrimSpace(target), "/")))
	return "account_workbench.maintenance." + hex.EncodeToString(digest[:]) + "." + suffix
}

func (s *Store) WorkbenchMaintenance(ctx context.Context, target string) (WorkbenchMaintenance, error) {
	result := WorkbenchMaintenance{IntervalMinutes: 5, CooldownMinutes: 10, GroupIDs: []string{}, CheckAfterRepair: true, Model: "gpt-5.6-sol"}
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, workbenchMaintenanceKey(target, "config")).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	err = json.Unmarshal([]byte(raw), &result)
	return result, err
}

func (s *Store) SaveWorkbenchMaintenance(ctx context.Context, target string, input WorkbenchMaintenance) error {
	previous := input.Revision
	input.Revision++
	raw, err := json.Marshal(input)
	if err != nil {
		return err
	}
	key := workbenchMaintenanceKey(target, "config")
	result, err := s.db.ExecContext(ctx, `INSERT INTO settings(key,value) SELECT ?,? WHERE ?=0 OR EXISTS(SELECT 1 FROM settings WHERE key=?)
ON CONFLICT(key) DO UPDATE SET value=excluded.value WHERE json_extract(settings.value,'$.revision')=?`, key, string(raw), previous, key, previous)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.New("自动维护配置已变化，请刷新后重试")
	}
	return nil
}

func (s *Store) WorkbenchMaintenanceRuntime(ctx context.Context, target string) (WorkbenchMaintenanceRuntime, error) {
	result := WorkbenchMaintenanceRuntime{Cooldowns: map[string]string{}}
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, workbenchMaintenanceKey(target, "runtime")).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	err = json.Unmarshal([]byte(raw), &result)
	if result.Cooldowns == nil {
		result.Cooldowns = map[string]string{}
	}
	return result, err
}

func (s *Store) SaveWorkbenchMaintenanceRuntime(ctx context.Context, target string, input WorkbenchMaintenanceRuntime) error {
	raw, err := json.Marshal(input)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, workbenchMaintenanceKey(target, "runtime"), string(raw))
	return err
}
