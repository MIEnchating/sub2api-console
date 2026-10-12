package taskstore

import (
	"context"
	"errors"
	"net/mail"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
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

func (s *Store) historyWhere(skill string, input HistoryQuery) (string, []any, error) {
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
		switch s.db.Driver() {
		case dbconn.Postgres:
			where += ` AND EXISTS (SELECT 1 FROM jsonb_path_query(CASE WHEN result_json IS JSON THEN result_json::jsonb ELSE '{}'::jsonb END,'$.**.email') AS field(value) WHERE jsonb_typeof(field.value)='string' AND lower(field.value #>> '{}') IN (` + strings.Join(marks, ",") + `))`
		case dbconn.MySQL:
			where += ` AND EXISTS (SELECT 1 FROM JSON_TABLE(JSON_EXTRACT(CASE WHEN JSON_VALID(result_json) THEN result_json ELSE '{}' END,'$**.email'),'$[*]' COLUMNS(value JSON PATH '$')) AS field WHERE JSON_TYPE(field.value)='STRING' AND lower(JSON_UNQUOTE(field.value)) IN (` + strings.Join(marks, ",") + `))`
		default:
			where += " AND EXISTS (SELECT 1 FROM json_tree(CASE WHEN json_valid(tasks.result_json) THEN tasks.result_json ELSE '{}' END) AS field WHERE field.key='email' AND field.type='text' AND lower(field.value) IN (" + strings.Join(marks, ",") + "))"
		}
	}
	if s.db.Driver() == dbconn.Postgres {
		where = strings.ReplaceAll(where, "instr(", "strpos(")
	}
	return where, args, nil
}

func (s *Store) QueryBySkill(ctx context.Context, skill string, input HistoryQuery) (HistoryPage, error) {
	if input.Limit == 0 {
		input.Limit = 50
	}
	where, args, err := s.historyWhere(skill, input)
	if err != nil {
		return HistoryPage{}, err
	}
	tx, err := s.db.BeginSnapshot(ctx)
	if err != nil {
		return HistoryPage{}, err
	}
	defer tx.Rollback()
	page := HistoryPage{Items: []Task{}, Offset: input.Offset, Limit: input.Limit}
	args = append(args, input.Limit, input.Offset)
	// Keep large result bodies out of the count window and its temporary sort.
	// Materialize only the requested IDs before looking up their full records.
	query := `WITH page AS MATERIALIZED (
		SELECT id,updated_at,COUNT(*) OVER() AS total FROM tasks WHERE ` + where + `
		ORDER BY updated_at DESC,id LIMIT ? OFFSET ?
	) SELECT t.id,t.skill,t.operation,t.status,t.progress,t.message,t.result_json,t.created_at,t.updated_at,page.total
	FROM page JOIN tasks t ON t.id=page.id ORDER BY page.updated_at DESC,page.id`
	if s.db.Driver() == dbconn.MySQL {
		query = strings.Replace(query, "AS MATERIALIZED", "AS", 1)
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return HistoryPage{}, err
	}
	defer rows.Close()
	hasRows := false
	for rows.Next() {
		var total int
		task, err := scanTaskWithTotal(rows, &total)
		if err != nil {
			return HistoryPage{}, err
		}
		if page.Total == 0 {
			page.Total = total
		}
		hasRows = true
		page.Items = append(page.Items, task)
	}
	if err := rows.Err(); err != nil {
		return HistoryPage{}, err
	}
	if !hasRows {
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM tasks WHERE "+where, args[:len(args)-2]...).Scan(&page.Total); err != nil {
			return HistoryPage{}, err
		}
	}
	return page, nil
}

func scanTaskWithTotal(scanner scanner, total *int) (Task, error) {
	var task Task
	var raw string
	if err := scanner.Scan(
		&task.ID,
		&task.Skill,
		&task.Operation,
		&task.Status,
		&task.Progress,
		&task.Message,
		&raw,
		&task.CreatedAt,
		&task.UpdatedAt,
		total,
	); err != nil {
		return Task{}, err
	}
	return decodeTaskResult(task, raw)
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
