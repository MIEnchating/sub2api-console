package taskstore

import (
	"context"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
)

// Project only statistics fields: generated HTML and answers stay in the database.
// The gallery index selects candidate tasks, without limiting results per account.
func (s *Store) serverDetectionConfidenceHistory(ctx context.Context, since time.Time) ([]DetectionConfidenceResult, error) {
	field := func(path string) string {
		return "COALESCE(JSON_UNQUOTE(JSON_EXTRACT(r.body,'$." + path + "')),'')"
	}
	array := "JSON_EXTRACT(t.result_json,'$.animations')"
	expand := "JOIN JSON_TABLE(CASE WHEN JSON_TYPE(" + array + ")='ARRAY' THEN " + array + " ELSE JSON_ARRAY() END, '$[*]' COLUMNS(body JSON PATH '$')) r ON TRUE"
	accountType := "JSON_TYPE(JSON_EXTRACT(r.body,'$.account_id'))='STRING'"
	precheckObject := "JSON_TYPE(JSON_EXTRACT(r.body,'$.precheck'))='OBJECT'"
	taskMode := "COALESCE(JSON_UNQUOTE(JSON_EXTRACT(t.result_json,'$.mode')),'')"
	if s.db.Driver() == dbconn.Postgres {
		field = func(path string) string {
			return "COALESCE(r.body #>> '{" + strings.ReplaceAll(path, ".", ",") + "}','')"
		}
		array = "t.result_json::jsonb->'animations'"
		expand = "CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(" + array + ")='array' THEN " + array + " ELSE '[]'::jsonb END) r(body)"
		accountType = "jsonb_typeof(r.body->'account_id')='string'"
		precheckObject = "jsonb_typeof(r.body->'precheck')='object'"
		taskMode = "COALESCE(t.result_json::jsonb->>'mode','')"
	}
	mode := field("mode")
	precheck := mode + "='precheck' OR (" + mode + "='' AND (" + precheckObject + " OR t.operation='account-model-precheck' OR " + taskMode + "='precheck'))"
	verdict := "CASE WHEN " + precheck + " THEN " + field("precheck.verdict") + " ELSE CASE " + field("quality.status") + " WHEN 'normal' THEN 'passed' WHEN 'suspicious' THEN 'not_passed' ELSE 'inconclusive' END END"
	query := `WITH eligible AS (
 SELECT * FROM tasks WHERE skill='sub2api-model-animation'
 AND status IN ('succeeded','partial','failed','cancelled')
 AND operation IN ('account-model-animation','account-model-precheck','account-model-combined','managed-model-detection')
 AND ` + s.validResultSQL() + `
 AND EXISTS (SELECT 1 FROM task_animation_history h WHERE h.task_id=tasks.id AND h.entry_kind='result' AND h.completed_seconds>=?)
 ) SELECT t.id,` + field("request_id") + ",CASE WHEN " + precheck + " THEN FALSE ELSE TRUE END," + field("account_id") + "," + field("completed_at") + "," + field("status") + "," + verdict + " FROM eligible t " + expand + " WHERE " + accountType + " AND " + mode + " IN ('','animation','precheck') AND " + field("status") + " IN ('succeeded','failed')"
	rows, err := s.db.QueryContext(ctx, query, since.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []DetectionConfidenceResult{}
	for rows.Next() {
		var row DetectionConfidenceResult
		if err := rows.Scan(&row.TaskID, &row.RequestID, &row.Animation, &row.AccountID, &row.CompletedAt, &row.Status, &row.Verdict); err != nil {
			return nil, err
		}
		at, err := time.Parse(time.RFC3339Nano, row.CompletedAt)
		if err != nil || at.Before(since) {
			continue
		}
		row.CompletedAt = at.UTC().Format(storageTimeLayout)
		result = append(result, row)
	}
	return result, rows.Err()
}
