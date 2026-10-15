package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/animationquality"
)

var ErrQualityRecheckConflict = errors.New("复核记录已变化，请重新读取后重试")

const qualityRecheckPrefix = "animation-quality-recheck/"

func (s *Store) AnimationQualityRecheck(ctx context.Context, id string) (*animationquality.Recheck, error) {
	raw, err := s.loadModelCheckState(ctx, qualityRecheckPrefix+id)
	if err != nil || len(raw) == 0 {
		return nil, err
	}
	var value animationquality.Recheck
	if err = json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return &value, nil
}

func (s *Store) SaveAnimationQualityRecheck(ctx context.Context, id string, value animationquality.Recheck, expectedVersion int) (animationquality.Recheck, error) {
	key := qualityRecheckPrefix + id
	tx, err := s.beginResourceTx(ctx, []string{key})
	if err != nil {
		return value, err
	}
	defer tx.Rollback()
	var previous string
	err = tx.QueryRowContext(ctx, `SELECT value_json FROM app_state WHERE "key"=?`, key).Scan(&previous)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return value, err
	}
	var saved animationquality.Recheck
	if previous != "" {
		if err = json.Unmarshal([]byte(previous), &saved); err != nil {
			return value, err
		}
	}
	if saved.Version != expectedVersion || previous != "" && (saved.SourceSHA256 != value.SourceSHA256 || saved.PromptSHA256 != value.PromptSHA256) {
		return value, ErrQualityRecheckConflict
	}
	value.Version = expectedVersion + 1
	raw, err := json.Marshal(value)
	if err != nil {
		return value, err
	}
	_, err = tx.ExecContext(ctx, tx.Dialect(`INSERT INTO app_state("key",value_json,updated_at) VALUES(?,?,?) ON CONFLICT("key") DO UPDATE SET value_json=excluded.value_json,updated_at=excluded.updated_at`, `INSERT INTO app_state("key",value_json,updated_at) VALUES(?,?,?) ON DUPLICATE KEY UPDATE value_json=VALUES(value_json),updated_at=VALUES(updated_at)`), key, string(raw), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return value, err
	}
	return value, tx.Commit()
}

// Read only the requested samples, in bounded batches, for task result badges.
func (s *Store) AnimationQualityRechecks(ctx context.Context, ids []string) (map[string]animationquality.Recheck, error) {
	result := make(map[string]animationquality.Recheck)
	for start := 0; start < len(ids); start += 200 {
		end := min(start+200, len(ids))
		args := make([]any, 0, end-start)
		for _, id := range ids[start:end] {
			args = append(args, qualityRecheckPrefix+id)
		}
		rows, err := s.db.QueryContext(ctx, `SELECT "key",value_json FROM app_state WHERE "key" IN (`+strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var key, raw string
			if err = rows.Scan(&key, &raw); err != nil {
				break
			}
			var value animationquality.Recheck
			if err = json.Unmarshal([]byte(raw), &value); err != nil {
				break
			}
			result[strings.TrimPrefix(key, qualityRecheckPrefix)] = value
		}
		rowErr := rows.Err()
		_ = rows.Close()
		if err != nil {
			return nil, err
		}
		if rowErr != nil {
			return nil, rowErr
		}
	}
	return result, nil
}
