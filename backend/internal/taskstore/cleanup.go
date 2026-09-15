package taskstore

import (
	"context"
	"errors"
)

func (s *Store) CleanupHistory(ctx context.Context, skill string) ([]Task, error) {
	if skill == "" {
		return nil, errors.New("清理模块不能为空")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,skill,operation,status,progress,message,result_json,created_at,updated_at FROM tasks WHERE skill=? ORDER BY id LIMIT 10001`, skill)
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
	if len(result) > 10000 {
		return nil, errors.New("工作台任务超过一万条，请先分批清理历史记录")
	}
	return result, rows.Err()
}
