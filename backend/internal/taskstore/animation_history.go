package taskstore

import (
	"context"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
)

// AnimationHistory returns lightweight task references, retaining the latest
// result and attempt for each stable target/stage as well as every active task.
// A busy account must not evict another account's last animation or precheck.
func (s *Store) AnimationHistory(ctx context.Context) ([]Task, error) {
	if s.db.Driver() != dbconn.SQLite {
		return s.serverAnimationHistory(ctx)
	}
	rows, err := s.db.QueryContext(ctx, `WITH animation_tasks AS (
  SELECT id,created_at,status,operation,result_json,updated_at FROM tasks
   WHERE skill='sub2api-model-animation' AND json_valid(result_json)
   AND operation IN ('account-model-animation','account-model-precheck','account-model-combined','managed-model-detection')
 ), result_rows AS (
  SELECT t.id,t.created_at,t.status,t.operation,json_extract(t.result_json,'$.mode') task_mode,
   CASE WHEN r.type='object' THEN r.value ELSE '{}' END value
  FROM animation_tasks t,json_each(t.result_json,'$.animations') r
 ), result_fields AS MATERIALIZED (
  SELECT id,created_at,status,operation,task_mode,
   jsonb_extract(value,'$.account_id','$.mode','$.completed_at','$.status','$.phase') fields,
   json_type(value,'$.precheck')='object' has_precheck
  FROM result_rows
 ), results AS MATERIALIZED (
  SELECT id,created_at,json_extract(fields,'$[0]') account_id,
   CASE WHEN json_extract(fields,'$[1]')='precheck' OR (
    COALESCE(json_extract(fields,'$[1]'),'')='' AND (
     has_precheck OR operation='account-model-precheck' OR task_mode='precheck'
    )) THEN 'precheck' ELSE 'animation' END stage,
   console_history_time(json_extract(fields,'$[2]')) completed_at
  FROM result_fields WHERE json_type(fields,'$[0]')='text'
   AND json_extract(fields,'$[0]')<>''
   AND json_extract(fields,'$[3]') IN ('succeeded','failed')
   AND console_history_time(json_extract(fields,'$[2]')) IS NOT NULL
   AND (status IN ('succeeded','partial','failed','cancelled') OR COALESCE(json_extract(fields,'$[4]'),'completed')='completed')
 ), result_ranks AS (
  SELECT id,ROW_NUMBER() OVER(PARTITION BY account_id,stage ORDER BY completed_at DESC,created_at DESC,id DESC) position FROM results
 ), scopes AS (
  SELECT t.id,CAST(a.value AS TEXT) account_id FROM animation_tasks t,json_each(t.result_json,'$.account_ids') a WHERE a.type='text'
  UNION
  SELECT t.id,json_extract(CASE WHEN a.type='object' THEN a.value ELSE '{}' END,'$.account_id') FROM animation_tasks t,json_each(t.result_json,'$.targets') a
  UNION
  SELECT id,account_id FROM results
 ), stages AS (
  SELECT t.id,t.created_at,s.stage FROM animation_tasks t CROSS JOIN (SELECT 'animation' stage UNION ALL SELECT 'precheck') s
  WHERE CASE WHEN t.operation='managed-model-detection' AND json_type(t.result_json,'$.configuration')='object'
   THEN CASE WHEN s.stage='animation' THEN COALESCE(json_extract(t.result_json,'$.configuration.animation'),1)=1
    ELSE json_extract(t.result_json,'$.configuration.precheck')=1 END
   ELSE (s.stage='animation' AND t.operation<>'account-model-precheck' AND COALESCE(json_extract(t.result_json,'$.mode'),'')<>'precheck')
    OR (s.stage='precheck' AND (t.operation IN ('account-model-precheck','account-model-combined') OR json_extract(t.result_json,'$.mode') IN ('precheck','both'))) END
 ), attempt_ranks AS (
  SELECT s.id,ROW_NUMBER() OVER(PARTITION BY a.account_id,s.stage ORDER BY s.created_at DESC,s.id DESC) position
  FROM stages s JOIN scopes a ON a.id=s.id WHERE a.account_id IS NOT NULL AND a.account_id<>''
 ), selected AS (
  SELECT id FROM (SELECT id FROM animation_tasks ORDER BY updated_at DESC,id LIMIT 20)
  UNION SELECT id FROM animation_tasks WHERE status IN ('queued','running','waiting_input')
  UNION SELECT id FROM result_ranks WHERE position=1
  UNION SELECT id FROM attempt_ranks WHERE position=1
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
