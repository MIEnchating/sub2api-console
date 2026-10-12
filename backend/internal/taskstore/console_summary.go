package taskstore

import (
	"context"
	"errors"
)

// Keep membership identical in the partial index and list query. The index
// contains display fields only; repeated polling never needs full result JSON.
const taskConsoleWhereSQL = `COALESCE(` + taskSystemInfoSQL + `,0)=1 OR operation IN (
	'account-model-animation','account-model-precheck','account-model-combined',
	'managed-model-detection','account-terminal-continuity','account-model-automation','account-model-automation-group'
) OR (
	operation='active-probe' AND json_valid(result_json) AND
	TRIM(COALESCE(CAST(json_extract(result_json,'$.platform') AS TEXT),''))<>'' AND
	TRIM(COALESCE(CAST(json_extract(result_json,'$.model') AS TEXT),''))<>''
)`

const taskConsoleSummaryIndexSQL = `CREATE INDEX IF NOT EXISTS ix_tasks_console_summary ON tasks(
	updated_at DESC,id,skill,operation,status,progress,message,created_at,` + taskRunKeySQL + `,` + taskObjectSQL + `
) WHERE ` + taskConsoleWhereSQL

const defaultConsoleSummaryLimit = 20

func (s *Store) ListConsoleSummaries(ctx context.Context, limit *int) ([]Task, error) {
	if limit == nil {
		value := defaultConsoleSummaryLimit
		limit = &value
	}
	query := `SELECT id,skill,operation,status,progress,message,` +
		taskRunKeySQL + `,` + taskObjectSQL +
		`,created_at,updated_at FROM tasks WHERE ` + taskConsoleWhereSQL + ` ORDER BY updated_at DESC,id`
	arguments := []any{}
	if limit != nil {
		if *limit < 0 || *limit > 100000 {
			return nil, errors.New("limit 必须在 0 到 100000 之间")
		}
		query += ` LIMIT ?`
		arguments = append(arguments, *limit)
	}
	rows, err := s.db.QueryContext(ctx, s.summarySQL(query), arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTaskSummaries(rows)
}
