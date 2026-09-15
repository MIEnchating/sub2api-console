package taskstore

import (
	"context"
	"errors"
	"net/mail"
	"strings"
)

type HistoryQuery struct {
	Search    string   `json:"search"`
	Emails    []string `json:"emails"`
	Status    string   `json:"status"`
	Operation string   `json:"operation"`
	Offset    int      `json:"offset"`
	Limit     int      `json:"limit"`
}

type HistoryPage struct {
	Items  []Task `json:"items"`
	Total  int    `json:"total"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
}

func historyWhere(skill string, input HistoryQuery) (string, []any, error) {
	if strings.TrimSpace(skill) == "" || input.Offset < 0 || input.Offset > 1000000 || input.Limit < 1 || input.Limit > 100 || len(input.Search) > 1024 || len(input.Operation) > 128 || len(input.Emails) > 500 {
		return "", nil, errors.New("历史查询范围无效，每页限 1～100 条、最多 500 个邮箱")
	}
	where, args := "skill=?", []any{skill}
	if input.Status != "" {
		if _, ok := validStatuses[input.Status]; !ok {
			return "", nil, errors.New("处理状态无效")
		}
		where += " AND status=?"
		args = append(args, input.Status)
	}
	if input.Operation != "" {
		where += " AND operation=?"
		args = append(args, input.Operation)
	}
	if search := strings.ToLower(strings.TrimSpace(input.Search)); search != "" {
		where += " AND (instr(lower(id),?)>0 OR instr(lower(message),?)>0 OR instr(lower(result_json),?)>0)"
		args = append(args, search, search, search)
	}
	if len(input.Emails) > 0 {
		marks := make([]string, 0, len(input.Emails))
		seen := make(map[string]bool, len(input.Emails))
		for _, raw := range input.Emails {
			email := strings.ToLower(strings.TrimSpace(raw))
			parsed, err := mail.ParseAddress(email)
			if err != nil || parsed.Address != email || len(email) > 320 || strings.ContainsAny(email, "\r\n\x00") {
				return "", nil, errors.New("邮箱查询只接受完整邮箱地址")
			}
			if seen[email] {
				continue
			}
			seen[email] = true
			marks = append(marks, "?")
			args = append(args, email)
		}
		// Only explicit email metadata participates in exact matching. A name
		// or diagnostic mentioning an email cannot bind an unrelated record.
		where += " AND EXISTS (SELECT 1 FROM json_tree(CASE WHEN json_valid(tasks.result_json) THEN tasks.result_json ELSE '{}' END) AS field WHERE field.key='email' AND field.type='text' AND lower(field.value) IN (" + strings.Join(marks, ",") + "))"
	}
	return where, args, nil
}

func (s *Store) QueryBySkill(ctx context.Context, skill string, input HistoryQuery) (HistoryPage, error) {
	if input.Limit == 0 {
		input.Limit = 50
	}
	where, args, err := historyWhere(skill, input)
	if err != nil {
		return HistoryPage{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return HistoryPage{}, err
	}
	defer tx.Rollback()
	page := HistoryPage{Items: []Task{}, Offset: input.Offset, Limit: input.Limit}
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM tasks WHERE "+where, args...).Scan(&page.Total); err != nil {
		return HistoryPage{}, err
	}
	args = append(args, input.Limit, input.Offset)
	rows, err := tx.QueryContext(ctx, "SELECT id,skill,operation,status,progress,message,result_json,created_at,updated_at FROM tasks WHERE "+where+" ORDER BY updated_at DESC,id LIMIT ? OFFSET ?", args...)
	if err != nil {
		return HistoryPage{}, err
	}
	defer rows.Close()
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return HistoryPage{}, err
		}
		page.Items = append(page.Items, task)
	}
	return page, rows.Err()
}

// ActiveBySkill returns the entire current cancellation scope independently of
// history pages and filters. Results omit task payloads to keep this bounded.
func (s *Store) ActiveBySkill(ctx context.Context, skill string) ([]Task, error) {
	if strings.TrimSpace(skill) == "" {
		return nil, errors.New("任务模块不能为空")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,skill,operation,status,progress,message,'{}',created_at,updated_at FROM tasks WHERE skill=? AND status IN ('queued','running','waiting_input') ORDER BY updated_at DESC,id LIMIT 10001`, skill)
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
		if len(result) > 10000 {
			return nil, errors.New("活动任务超过 10000 个，请先分批取消后重新读取范围")
		}
	}
	return result, rows.Err()
}
