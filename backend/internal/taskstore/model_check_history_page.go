package taskstore

import (
	"context"
	"errors"
)

// ModelCheckHistoryPage reads a bounded page without loading answer bodies.
func (s *Store) ModelCheckHistoryPage(ctx context.Context, page int) ([]Task, bool, error) {
	if page < 1 || page > 100000 {
		return nil, false, errors.New("page 必须在 1 到 100000 之间")
	}
	const size = 20
	rows, err := s.db.QueryContext(ctx, `SELECT id,skill,operation,status,progress,message,created_at,updated_at FROM tasks WHERE skill=? ORDER BY created_at DESC,id DESC LIMIT ? OFFSET ?`, "sub2api-model-check", size+1, (page-1)*size)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	result := []Task{}
	for rows.Next() {
		var task Task
		if err := rows.Scan(&task.ID, &task.Skill, &task.Operation, &task.Status, &task.Progress, &task.Message, &task.CreatedAt, &task.UpdatedAt); err != nil {
			return nil, false, err
		}
		result = append(result, task)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(result) > size
	if more {
		result = result[:size]
	}
	return result, more, nil
}
