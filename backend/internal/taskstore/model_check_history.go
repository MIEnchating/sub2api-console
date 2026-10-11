package taskstore

import (
	"context"
	"time"
)

// ModelCheckHistory projects only public verdict fields. It retains every check in
// the statistics window plus the latest terminal task for each account.
func (s *Store) ModelCheckHistory(ctx context.Context, since time.Time) ([]Task, error) {
	rows, err := s.db.QueryContext(ctx, `WITH terminal AS (
  SELECT id,skill,operation,status,progress,message,result_json,created_at,updated_at,
   CASE WHEN json_type(result_json,'$.account_ids')='array'
    THEN json_extract(result_json,'$.account_ids') ELSE '[]' END account_ids,
   CASE WHEN json_type(result_json,'$.tests')='array'
    THEN json_extract(result_json,'$.tests') ELSE '[]' END tests
  FROM tasks WHERE skill='sub2api-model-check'
   AND status IN ('succeeded','partial','failed','cancelled') AND json_valid(result_json)
 ), test_objects AS (
  SELECT t.id,CASE WHEN r.type='object' THEN r.value ELSE '{}' END body
  FROM terminal t,json_each(t.tests) r
 ), valid_tests AS (
  SELECT id,json_extract(body,'$.account_id') account_id,
   json_object('account_id',json_extract(body,'$.account_id'),
    'claimed_model',json_extract(body,'$.claimed_model'),'verdict',json_extract(body,'$.verdict'),
    'requests',json_extract(body,'$.requests')) verdict
  FROM test_objects
  WHERE json_type(body,'$.account_id')='text' AND json_extract(body,'$.account_id')<>''
 ), accounts AS (
  SELECT t.id,t.updated_at,a.value account_id FROM terminal t,json_each(t.account_ids) a
  WHERE a.type='text' AND a.value<>''
  UNION
  SELECT t.id,t.updated_at,r.account_id
  FROM terminal t JOIN valid_tests r ON r.id=t.id
 ), ranked AS (
  SELECT id,ROW_NUMBER() OVER(PARTITION BY account_id ORDER BY updated_at DESC,id DESC) position FROM accounts
 ) SELECT t.id,t.skill,t.operation,t.status,t.progress,t.message,
 json_object('account_ids',json((
  SELECT json_group_array(a.value) FROM json_each(t.account_ids) a WHERE a.type='text' AND a.value<>''
 )),'tests',json((
  SELECT json_group_array(json(r.verdict)) FROM valid_tests r WHERE r.id=t.id
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
