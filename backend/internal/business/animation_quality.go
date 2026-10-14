package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/animationquality"
)

var ErrQualityAnnotationConflict = errors.New("质量标注已被修改，请重新读取后保存")

const qualitySamplePrefix = "animation-quality-sample/"

func (s *Store) AnimationQualitySample(ctx context.Context, id string) (*animationquality.Sample, error) {
	raw, err := s.loadModelCheckState(ctx, qualitySamplePrefix+id)
	if err != nil || len(raw) == 0 {
		return nil, err
	}
	var value animationquality.Sample
	if err = json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return &value, nil
}

func (s *Store) SaveAnimationQualitySample(ctx context.Context, value animationquality.Sample, expectedVersion int) (animationquality.Sample, error) {
	key := qualitySamplePrefix + value.ID
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
	version := 0
	if previous != "" {
		var saved animationquality.Sample
		if err = json.Unmarshal([]byte(previous), &saved); err != nil {
			return value, err
		}
		version = saved.Annotation.Version
		if saved.SourceSHA256 != value.SourceSHA256 || saved.PromptSHA256 != value.PromptSHA256 {
			return value, ErrQualityAnnotationConflict
		}
		value.CreatedAt = saved.CreatedAt
		value.Prediction = saved.Prediction
	}
	if expectedVersion != version {
		return value, ErrQualityAnnotationConflict
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	value.Annotation.Version = version + 1
	value.Annotation.UpdatedAt = now
	if value.CreatedAt == "" {
		value.CreatedAt = now
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return value, err
	}
	if _, err = tx.ExecContext(ctx, tx.Dialect(`INSERT INTO app_state("key",value_json,updated_at) VALUES(?,?,?) ON CONFLICT("key") DO UPDATE SET value_json=excluded.value_json,updated_at=excluded.updated_at`, `INSERT INTO app_state("key",value_json,updated_at) VALUES(?,?,?) ON DUPLICATE KEY UPDATE value_json=VALUES(value_json),updated_at=VALUES(updated_at)`), key, string(raw), now); err != nil {
		return value, err
	}
	sourceID, err := nextLocalEventID(ctx, tx, "operation_audit")
	if err != nil {
		return value, err
	}
	// Audits retain only fingerprints, never source or annotation prose.
	_, err = tx.ExecContext(ctx, `INSERT INTO operation_audit(source_id,operation_id,operation_type,state,phase,actor,source,remote_confirmed,readback_confirmed,object_type,object_id,object_name,group_names_json,field_name,before_json,after_json,writeback,created_at) VALUES(?,?,?,'succeeded','annotation.saved',?,'console',0,1,'animation-quality-sample',?,'动画质量标注','[]','annotation',?,?,0,?)`, sourceID, fmt.Sprintf("animation-quality-%d", time.Now().UnixNano()), "animation.quality.annotation.saved", value.Annotation.UpdatedBy, value.ID, configurationAuditValue(previous), configurationAuditValue(string(raw)), now)
	if err != nil {
		return value, err
	}
	return value, tx.Commit()
}

// Keyset pagination bounds memory and survives independent task cleanup.
func (s *Store) AnimationQualitySamples(ctx context.Context, after string, limit int) (animationquality.SamplePage, error) {
	result := animationquality.SamplePage{Items: []animationquality.Sample{}}
	if limit < 1 || limit > 100 {
		return result, errors.New("每批样本数量必须为 1～100")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT "key",value_json FROM app_state WHERE "key" LIKE ? AND "key">? ORDER BY "key" LIMIT ?`, qualitySamplePrefix+"%", qualitySamplePrefix+after, limit+1)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, raw string
		if err = rows.Scan(&key, &raw); err != nil {
			return result, err
		}
		if len(result.Items) == limit {
			result.NextCursor = result.Items[len(result.Items)-1].ID
			break
		}
		var value animationquality.Sample
		if err = json.Unmarshal([]byte(raw), &value); err != nil {
			return result, err
		}
		if value.ID != strings.TrimPrefix(key, qualitySamplePrefix) {
			return result, errors.New("质量样本索引无效")
		}
		result.Items = append(result.Items, value)
	}
	return result, rows.Err()
}
