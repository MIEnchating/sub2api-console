package taskstore

import (
	"context"
	"time"
)

// ModelCheckHistory projects only public verdict fields. It retains every check in
// the statistics window plus the latest terminal task for each account.
func (s *Store) ModelCheckHistory(ctx context.Context, since time.Time) ([]Task, error) {
	rows, err := s.db.QueryContext(ctx, `WITH terminal AS (
  SELECT * FROM tasks WHERE skill='sub2api-model-check'
   AND status IN ('succeeded','partial','failed','cancelled') AND json_valid(result_json)
 ), accounts AS (
  SELECT t.id,t.updated_at,CAST(a.value AS TEXT) account_id FROM terminal t,json_each(t.result_json,'$.account_ids') a
  UNION
  SELECT t.id,t.updated_at,CAST(json_extract(r.value,'$.account_id') AS TEXT) FROM terminal t,json_each(t.result_json,'$.tests') r
 ), ranked AS (
  SELECT id,ROW_NUMBER() OVER(PARTITION BY account_id ORDER BY updated_at DESC,id DESC) position FROM accounts WHERE account_id IS NOT NULL
 ) SELECT t.id,t.skill,t.operation,t.status,t.progress,t.message,
 json_object('account_ids',json_extract(t.result_json,'$.account_ids'),'tests',json((
  SELECT json_group_array(json_object('account_id',json_extract(r.value,'$.account_id'),
   'claimed_model',json_extract(r.value,'$.claimed_model'),'verdict',json_extract(r.value,'$.verdict'),
   'requests',json_extract(r.value,'$.requests'))) FROM json_each(t.result_json,'$.tests') r
 ))),t.created_at,t.updated_at FROM terminal t
 WHERE t.updated_at>=? OR t.id IN (SELECT id FROM ranked WHERE position=1)
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
