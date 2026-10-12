package taskstore

import (
	"context"
	"time"
)

func (s *Store) serverAnimationHistory(ctx context.Context) ([]Task, error) {
	rows, err := s.db.QueryContext(ctx, `WITH eligible AS (
 SELECT id,created_at,updated_at,status FROM tasks WHERE skill='sub2api-model-animation'
 AND operation IN ('account-model-animation','account-model-precheck','account-model-combined','managed-model-detection') AND `+s.validResultSQL()+`
 ), ranked AS (
 SELECT h.task_id,ROW_NUMBER() OVER(PARTITION BY h.account_hash,h.account_id,h.stage,h.entry_kind ORDER BY h.completed_seconds DESC,h.completed_nanos DESC,t.created_at DESC,t.id DESC) AS position
 FROM task_animation_history h JOIN eligible t ON t.id=h.task_id
 ), selected AS (
 SELECT id FROM (SELECT id FROM eligible ORDER BY updated_at DESC,id LIMIT 20) AS recent
 UNION SELECT id FROM eligible WHERE status IN ('queued','running','waiting_input')
 UNION SELECT task_id FROM ranked WHERE position=1
 ) SELECT t.id,t.skill,t.operation,t.status,t.progress,t.message,'{}',t.created_at,t.updated_at
 FROM tasks t JOIN selected s ON s.id=t.id ORDER BY t.updated_at DESC,t.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Task{}
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, task)
	}
	return result, rows.Err()
}

func (s *Store) serverModelHistory(ctx context.Context, since time.Time) ([]Task, error) {
	rows, err := s.db.QueryContext(ctx, `WITH ranked AS (
 SELECT a.task_id,ROW_NUMBER() OVER(PARTITION BY a.account_hash,a.account_id ORDER BY t.updated_at DESC,t.id DESC) AS position
 FROM task_model_accounts a JOIN tasks t ON t.id=a.task_id
 ) SELECT t.id,t.skill,t.operation,t.status,t.progress,t.message,h.payload,t.created_at,t.updated_at
 FROM tasks t JOIN task_model_history h ON h.task_id=t.id
 WHERE t.updated_at>=? OR t.id IN (SELECT task_id FROM ranked WHERE position=1)
 ORDER BY t.updated_at DESC,t.id DESC`, since.UTC().Format(storageTimeLayout))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Task{}
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, task)
	}
	return result, rows.Err()
}
