package configstore

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type NewAPIPlatform struct {
	ID        string
	Name      string
	BaseURL   string
	AdminKey  string
	UserID    string
	UpdatedAt string
}

type NewAPIPlatformSummary struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	BaseURL            string `json:"base_url"`
	UserID             string `json:"user_id"`
	AdminKeyConfigured bool   `json:"admin_key_configured"`
	UpdatedAt          string `json:"updated_at"`
}

func (s *Store) NewAPIPlatforms(ctx context.Context) ([]NewAPIPlatformSummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,base_url,user_id,admin_key,updated_at FROM newapi_platforms ORDER BY updated_at,id LIMIT 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []NewAPIPlatformSummary{}
	for rows.Next() {
		var item NewAPIPlatformSummary
		var adminKey string
		if err := rows.Scan(&item.ID, &item.Name, &item.BaseURL, &item.UserID, &adminKey, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.AdminKeyConfigured = strings.TrimSpace(adminKey) != ""
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) NewAPIPlatform(ctx context.Context, id string) (*NewAPIPlatform, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("New API 平台 ID 不能为空")
	}
	var item NewAPIPlatform
	err := s.db.QueryRowContext(ctx, `SELECT id,name,base_url,admin_key,user_id,updated_at FROM newapi_platforms WHERE id=?`, id).Scan(
		&item.ID, &item.Name, &item.BaseURL, &item.AdminKey, &item.UserID, &item.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &item, err
}

func (s *Store) SaveNewAPIPlatform(ctx context.Context, item NewAPIPlatform) (NewAPIPlatformSummary, error) {
	item.ID = strings.TrimSpace(item.ID)
	item.Name = strings.TrimSpace(item.Name)
	item.UserID = strings.TrimSpace(item.UserID)
	item.AdminKey = strings.TrimSpace(item.AdminKey)
	if item.Name == "" || len(item.Name) > 120 {
		return NewAPIPlatformSummary{}, errors.New("New API 平台名称长度必须为 1 到 120 个字符")
	}
	baseURL, err := ValidateBaseURL(item.BaseURL)
	if err != nil {
		return NewAPIPlatformSummary{}, errors.New("New API 平台地址无效")
	}
	item.BaseURL = baseURL
	if item.UserID == "" || len(item.UserID) > 128 {
		return NewAPIPlatformSummary{}, errors.New("New API User ID 不能为空")
	}
	if item.ID == "" {
		item.ID = "primary"
	}
	current, err := s.NewAPIPlatform(ctx, item.ID)
	if err != nil {
		return NewAPIPlatformSummary{}, err
	}
	if item.AdminKey == "" && current != nil {
		item.AdminKey = current.AdminKey
	}
	if item.AdminKey == "" || len(item.AdminKey) > 4096 {
		return NewAPIPlatformSummary{}, errors.New("New API Admin Key 不能为空")
	}
	item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return NewAPIPlatformSummary{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO newapi_platforms(id,name,base_url,admin_key,user_id,updated_at)
		VALUES(?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,base_url=excluded.base_url,
		admin_key=excluded.admin_key,user_id=excluded.user_id,updated_at=excluded.updated_at`,
		item.ID, item.Name, item.BaseURL, item.AdminKey, item.UserID, item.UpdatedAt)
	if err != nil {
		return NewAPIPlatformSummary{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM newapi_platforms WHERE id<>?`, item.ID); err != nil {
		return NewAPIPlatformSummary{}, err
	}
	if err := tx.Commit(); err != nil {
		return NewAPIPlatformSummary{}, err
	}
	return NewAPIPlatformSummary{ID: item.ID, Name: item.Name, BaseURL: item.BaseURL, UserID: item.UserID, AdminKeyConfigured: true, UpdatedAt: item.UpdatedAt}, nil
}

func (s *Store) DeleteNewAPIPlatform(ctx context.Context, id string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM newapi_platforms WHERE id=?`, strings.TrimSpace(id))
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count > 0, err
}
