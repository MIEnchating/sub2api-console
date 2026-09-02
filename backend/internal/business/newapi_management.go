package business

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type NewAPILocalGroup struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Ratio *string `json:"ratio"`
}

type NewAPIGroupBinding struct {
	PlatformID      string `json:"platform_id"`
	NewAPIGroupID   string `json:"newapi_group_id"`
	NewAPIGroupName string `json:"newapi_group_name"`
	Sub2APIGroupID  string `json:"sub2api_group_id"`
	SyncRatio       bool   `json:"sync_ratio"`
}

func (s *Store) NewAPILocalGroups(ctx context.Context) ([]NewAPILocalGroup, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT remote_id,name,rate_multiplier FROM local_groups
		WHERE remote_id IS NOT NULL AND TRIM(remote_id)<>'' ORDER BY name,remote_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []NewAPILocalGroup{}
	for rows.Next() {
		var item NewAPILocalGroup
		var ratio sql.NullString
		if err := rows.Scan(&item.ID, &item.Name, &ratio); err != nil {
			return nil, err
		}
		item.Ratio = nullString(ratio)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) NewAPIGroupBindings(ctx context.Context, platformID string) ([]NewAPIGroupBinding, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT platform_id,newapi_group_id,newapi_group_name,sub2api_group_id,sync_ratio
		FROM newapi_group_bindings WHERE platform_id=? ORDER BY newapi_group_name,newapi_group_id`, strings.TrimSpace(platformID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []NewAPIGroupBinding{}
	for rows.Next() {
		var item NewAPIGroupBinding
		if err := rows.Scan(&item.PlatformID, &item.NewAPIGroupID, &item.NewAPIGroupName, &item.Sub2APIGroupID, &item.SyncRatio); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) ReplaceNewAPIGroupBindings(ctx context.Context, platformID string, items []NewAPIGroupBinding) error {
	platformID = strings.TrimSpace(platformID)
	if platformID == "" {
		return errors.New("New API 平台 ID 不能为空")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM newapi_group_bindings WHERE platform_id=?`, platformID); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	seen := map[string]struct{}{}
	for _, item := range items {
		item.NewAPIGroupID = strings.TrimSpace(item.NewAPIGroupID)
		item.NewAPIGroupName = strings.TrimSpace(item.NewAPIGroupName)
		item.Sub2APIGroupID = strings.TrimSpace(item.Sub2APIGroupID)
		if item.NewAPIGroupID == "" || item.NewAPIGroupName == "" || item.Sub2APIGroupID == "" {
			return errors.New("New API 分组绑定包含空 ID 或名称")
		}
		if _, duplicate := seen[item.NewAPIGroupID]; duplicate {
			return errors.New("同一 New API 分组不能重复绑定")
		}
		seen[item.NewAPIGroupID] = struct{}{}
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM local_groups WHERE remote_id=?)`, item.Sub2APIGroupID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return errors.New("绑定目标不是已登记的 Sub2API 稳定分组 ID")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO newapi_group_bindings(
			platform_id,newapi_group_id,newapi_group_name,sub2api_group_id,sync_ratio,updated_at
		) VALUES(?,?,?,?,?,?)`, platformID, item.NewAPIGroupID, item.NewAPIGroupName, item.Sub2APIGroupID, item.SyncRatio, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) DeleteNewAPIGroupBindings(ctx context.Context, platformID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM newapi_group_bindings WHERE platform_id=?`, strings.TrimSpace(platformID))
	return err
}
