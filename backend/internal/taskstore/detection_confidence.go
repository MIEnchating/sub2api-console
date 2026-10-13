package taskstore

import (
	"context"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
)

// DetectionConfidenceResult projects verdicts without generated code, answers or credentials.
type DetectionConfidenceResult struct {
	AccountID   string
	CompletedAt string
	Status      string
	Verdict     string
}

// DetectionConfidenceHistory keeps every completed stage in the statistics window,
// independent of the latest-result retention used by the animation gallery.
func (s *Store) DetectionConfidenceHistory(ctx context.Context, since time.Time) ([]DetectionConfidenceResult, error) {
	if s.db.Driver() != dbconn.SQLite {
		return s.serverDetectionConfidenceHistory(ctx, since)
	}
	rows, err := s.db.QueryContext(ctx, `WITH detection_tasks AS MATERIALIZED (
 SELECT operation, result_json FROM tasks
 WHERE skill='sub2api-model-animation' AND json_valid(result_json)
 AND status IN ('succeeded','partial','failed','cancelled')
 AND operation IN ('account-model-animation','account-model-precheck','account-model-combined','managed-model-detection')
 ), result_rows AS (
 SELECT t.operation,json_extract(t.result_json,'$.mode') task_mode,
 CASE WHEN r.type='object' THEN r.value ELSE '{}' END body
 FROM detection_tasks t,json_each(CASE WHEN json_type(t.result_json,'$.animations')='array'
 THEN json_extract(t.result_json,'$.animations') ELSE '[]' END) r
 ), verdicts AS (
 SELECT json_extract(body,'$.account_id') account_id,
 console_history_time(json_extract(body,'$.completed_at')) completed_at,
 json_extract(body,'$.status') status,
 CASE WHEN json_extract(body,'$.mode')='precheck' OR (
 COALESCE(json_extract(body,'$.mode'),'')='' AND (
 json_type(body,'$.precheck')='object' OR operation='account-model-precheck' OR task_mode='precheck'))
 THEN json_extract(body,'$.precheck.verdict') ELSE json_extract(body,'$.manual_verdict.verdict') END verdict
 FROM result_rows WHERE json_type(body,'$.account_id')='text'
 AND COALESCE(json_extract(body,'$.mode'),'') IN ('','animation','precheck')
 ) SELECT account_id,completed_at,status,COALESCE(verdict,'') FROM verdicts
 WHERE completed_at>=? AND status IN ('succeeded','failed')`, since.UTC().Format(storageTimeLayout))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []DetectionConfidenceResult{}
	for rows.Next() {
		var row DetectionConfidenceResult
		if err := rows.Scan(&row.AccountID, &row.CompletedAt, &row.Status, &row.Verdict); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}
