package business

import (
	"context"
	"encoding/json"
	"strings"

	q "github.com/MIEnchating/sub2api-console/backend/internal/animationquality"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
)

// Read annotation metadata in bounded batches without loading generated source.
func (s *Store) AnimationQualityAnnotations(ctx context.Context, ids []string) (map[string]q.AnnotationRecord, error) {
	result := make(map[string]q.AnnotationRecord)
	field := func(name string) string { return "json_extract(value_json,'$." + name + "')" }
	if s.db.Driver() == dbconn.Postgres {
		field = func(name string) string { return "value_json::jsonb->>'" + name + "'" }
	}
	if s.db.Driver() == dbconn.MySQL {
		field = func(name string) string { return "JSON_UNQUOTE(JSON_EXTRACT(value_json,'$." + name + "'))" }
	}
	for start := 0; start < len(ids); start += 200 {
		end := min(start+200, len(ids))
		args := make([]any, 0, end-start)
		for _, id := range ids[start:end] {
			args = append(args, qualitySamplePrefix+id)
		}
		rows, err := s.db.QueryContext(ctx, `SELECT "key",`+field("source_sha256")+","+field("prompt_sha256")+","+field("annotation")+` FROM app_state WHERE "key" IN (`+strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var key, raw string
			var value q.AnnotationRecord
			if err = rows.Scan(&key, &value.SourceSHA256, &value.PromptSHA256, &raw); err != nil {
				break
			}
			if err = json.Unmarshal([]byte(raw), &value.Annotation); err != nil {
				break
			}
			result[strings.TrimPrefix(key, qualitySamplePrefix)] = value
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
