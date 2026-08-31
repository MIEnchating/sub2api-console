package taskstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("任务不存在或已过期")

const (
	taskRunKeySQL = `CASE WHEN json_valid(result_json) THEN CAST(json_extract(result_json,'$.run_key') AS TEXT) END`
	taskObjectSQL = `CASE WHEN json_valid(result_json) THEN CAST(COALESCE(
		json_extract(result_json,'$.host'),json_extract(result_json,'$.account_name'),
		json_extract(result_json,'$.account_id'),json_extract(result_json,'$.object_label')
	) AS TEXT) END`
	taskRequestSQL = `CASE WHEN json_valid(result_json) THEN CAST(COALESCE(
		json_extract(result_json,'$.request_id'),json_extract(result_json,'$.req'),
		json_extract(result_json,'$.client_request_id'),json_extract(result_json,'$.client_req')
	) AS TEXT) END`
	taskModelSQL = `CASE WHEN json_valid(result_json) THEN CAST(json_extract(result_json,'$.model') AS TEXT) END`
	taskErrorSQL = `CASE WHEN json_valid(result_json) THEN CAST(COALESCE(
		json_extract(result_json,'$.error'),json_extract(result_json,'$.detail'),json_extract(result_json,'$.summary')
	) AS TEXT) END`
)

var validStatuses = map[string]struct{}{
	"queued": {}, "running": {}, "waiting_input": {}, "succeeded": {}, "failed": {}, "cancelled": {},
}

type Task struct {
	ID        string         `json:"id"`
	Skill     string         `json:"skill"`
	Operation string         `json:"operation"`
	Status    string         `json:"status"`
	Progress  int            `json:"progress"`
	Message   string         `json:"message"`
	Result    map[string]any `json:"result"`
	CreatedAt string         `json:"created_at"`
	UpdatedAt string         `json:"updated_at"`
}

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if err := sqliteutil.Prepare(path); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout%285000%29&_pragma=journal_mode%28WAL%29")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	store := &Store{db: db}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,skill TEXT NOT NULL,operation TEXT NOT NULL,status TEXT NOT NULL,progress INTEGER NOT NULL,
		message TEXT NOT NULL,result_json TEXT NOT NULL,created_at TEXT NOT NULL,updated_at TEXT NOT NULL
	)`); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	for _, statement := range []string{
		`CREATE INDEX IF NOT EXISTS ix_tasks_updated_at ON tasks(updated_at DESC,id)`,
		`CREATE INDEX IF NOT EXISTS ix_tasks_status_updated_at ON tasks(status,updated_at,id)`,
		`CREATE INDEX IF NOT EXISTS ix_tasks_skill_updated_at ON tasks(skill,updated_at DESC,id)`,
		`CREATE INDEX IF NOT EXISTS ix_tasks_log_listing ON tasks(
			updated_at DESC,id,skill,operation,status,progress,message,created_at,
			` + taskRunKeySQL + `,` + taskObjectSQL + `
		)`,
		`CREATE INDEX IF NOT EXISTS ix_tasks_log_search ON tasks(
			updated_at DESC,id,skill,operation,status,progress,message,created_at,
			` + taskRunKeySQL + `,` + taskObjectSQL + `,` + taskRequestSQL + `,` + taskModelSQL + `,` + taskErrorSQL + `
		)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			return nil, errors.Join(err, db.Close())
		}
	}
	if err := sqliteutil.Secure(path); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Save(ctx context.Context, task Task) error {
	if err := validateTask(task); err != nil {
		return err
	}
	encoded, err := json.Marshal(task.Result)
	if err != nil {
		return fmt.Errorf("任务结果无法严格 JSON 序列化：%w", err)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO tasks(id,skill,operation,status,progress,message,result_json,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET
		status=excluded.status,progress=excluded.progress,message=excluded.message,result_json=excluded.result_json,updated_at=excluded.updated_at`,
		task.ID, task.Skill, task.Operation, task.Status, task.Progress, task.Message, string(encoded), task.CreatedAt, task.UpdatedAt)
	return err
}

func (s *Store) Get(ctx context.Context, id string) (Task, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id,skill,operation,status,progress,message,result_json,created_at,updated_at FROM tasks WHERE id=?`, strings.TrimSpace(id))
	return scanTask(row)
}

func (s *Store) List(ctx context.Context, limit *int) ([]Task, error) {
	query := `SELECT id,skill,operation,status,progress,message,result_json,created_at,updated_at FROM tasks ORDER BY updated_at DESC`
	arguments := []any{}
	if limit != nil {
		if *limit < 0 || *limit > 100000 {
			return nil, errors.New("limit 必须在 0 到 100000 之间")
		}
		query += ` LIMIT ?`
		arguments = append(arguments, *limit)
	}
	rows, err := s.db.QueryContext(ctx, query, arguments...)
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

func (s *Store) ListBySkill(ctx context.Context, skill string) ([]Task, error) {
	skill = strings.TrimSpace(skill)
	if skill == "" {
		return nil, errors.New("skill 不能为空")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,skill,operation,status,progress,message,result_json,created_at,updated_at
		FROM tasks WHERE skill=? ORDER BY updated_at DESC,id`, skill)
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

func (s *Store) ListLogSummaries(ctx context.Context, limit *int) ([]Task, error) {
	query := `SELECT id,skill,operation,status,progress,message,
		CASE WHEN json_valid(result_json) THEN json_extract(result_json,'$.run_key') END,
		CASE WHEN json_valid(result_json) THEN COALESCE(
			json_extract(result_json,'$.host'),json_extract(result_json,'$.account_name'),json_extract(result_json,'$.account_id')
		) END,created_at,updated_at FROM tasks ORDER BY updated_at DESC`
	arguments := []any{}
	if limit != nil {
		if *limit < 0 || *limit > 100000 {
			return nil, errors.New("limit 必须在 0 到 100000 之间")
		}
		query += ` LIMIT ?`
		arguments = append(arguments, *limit)
	}
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Task{}
	for rows.Next() {
		var task Task
		var runKey, objectLabel sql.NullString
		if err := rows.Scan(
			&task.ID, &task.Skill, &task.Operation, &task.Status, &task.Progress, &task.Message,
			&runKey, &objectLabel, &task.CreatedAt, &task.UpdatedAt,
		); err != nil {
			return nil, err
		}
		task.Result = map[string]any{}
		if runKey.Valid && strings.TrimSpace(runKey.String) != "" {
			task.Result["run_key"] = runKey.String
		}
		if objectLabel.Valid && strings.TrimSpace(objectLabel.String) != "" {
			task.Result["object_label"] = objectLabel.String
		}
		result = append(result, task)
	}
	return result, rows.Err()
}

func (s *Store) SearchLogs(ctx context.Context, search string, limit *int) ([]Task, error) {
	normalized := strings.ToLower(strings.TrimSpace(search))
	if normalized == "" {
		return s.ListLogSummaries(ctx, limit)
	}
	query := `SELECT id,skill,operation,status,progress,message,` +
		taskRunKeySQL + `,` + taskObjectSQL + `,` + taskRequestSQL + `,` + taskModelSQL + `,` + taskErrorSQL +
		`,created_at,updated_at FROM tasks INDEXED BY ix_tasks_log_search WHERE
		instr(lower(id),?)>0 OR instr(lower(skill),?)>0 OR instr(lower(operation),?)>0 OR
		instr(lower(status),?)>0 OR instr(lower(message),?)>0 OR instr(lower(COALESCE(` + taskRunKeySQL + `,'')),?)>0 OR
		instr(lower(COALESCE(` + taskObjectSQL + `,'')),?)>0 OR instr(lower(COALESCE(` + taskRequestSQL + `,'')),?)>0 OR
		instr(lower(COALESCE(` + taskModelSQL + `,'')),?)>0 OR instr(lower(COALESCE(` + taskErrorSQL + `,'')),?)>0 OR
		instr(lower(created_at),?)>0 OR instr(lower(updated_at),?)>0
		ORDER BY updated_at DESC`
	arguments := []any{normalized, normalized, normalized, normalized, normalized, normalized, normalized, normalized, normalized, normalized, normalized, normalized}
	if limit != nil {
		if *limit < 0 || *limit > 100000 {
			return nil, errors.New("limit 必须在 0 到 100000 之间")
		}
		query += ` LIMIT ?`
		arguments = append(arguments, *limit)
	}
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Task{}
	for rows.Next() {
		var task Task
		var runKey, objectLabel, requestID, model, errorText sql.NullString
		if err := rows.Scan(
			&task.ID, &task.Skill, &task.Operation, &task.Status, &task.Progress, &task.Message,
			&runKey, &objectLabel, &requestID, &model, &errorText, &task.CreatedAt, &task.UpdatedAt,
		); err != nil {
			return nil, err
		}
		task.Result = map[string]any{}
		for key, value := range map[string]sql.NullString{
			"run_key": runKey, "object_label": objectLabel, "request_id": requestID, "model": model, "error": errorText,
		} {
			if value.Valid && strings.TrimSpace(value.String) != "" {
				task.Result[key] = value.String
			}
		}
		result = append(result, task)
	}
	return result, rows.Err()
}

func (s *Store) RecoverInterrupted(ctx context.Context) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `UPDATE tasks SET status='failed',progress=100,message='进程重启导致任务中断',
		result_json='{"error":"进程重启导致任务中断","interrupted":true}',updated_at=?
		WHERE status IN ('queued','running','waiting_input')`, now)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Store) ClearLogs(ctx context.Context, before *time.Time) (int64, int64, error) {
	var protected int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE status IN ('queued','running','waiting_input')`).Scan(&protected); err != nil {
		return 0, 0, err
	}
	query := `DELETE FROM tasks WHERE status NOT IN ('queued','running','waiting_input')`
	arguments := []any{}
	if before != nil {
		query += ` AND updated_at < ?`
		arguments = append(arguments, before.UTC().Format(time.RFC3339Nano))
	}
	result, err := s.db.ExecContext(ctx, query, arguments...)
	if err != nil {
		return 0, protected, err
	}
	deleted, err := result.RowsAffected()
	return deleted, protected, err
}

type scanner interface {
	Scan(...any) error
}

func scanTask(row scanner) (Task, error) {
	var task Task
	var raw string
	if err := row.Scan(&task.ID, &task.Skill, &task.Operation, &task.Status, &task.Progress, &task.Message, &raw, &task.CreatedAt, &task.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Task{}, ErrNotFound
		}
		return Task{}, err
	}
	if err := validateTaskColumns(task); err != nil {
		return corruptTask(task, "任务持久化字段损坏"), nil
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&task.Result); err != nil || task.Result == nil {
		return corruptTask(task, "任务持久化结果损坏"), nil
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return corruptTask(task, "任务持久化结果损坏"), nil
	}
	return task, nil
}

func validateTask(task Task) error {
	if err := validateTaskColumns(task); err != nil {
		return err
	}
	if task.Result == nil {
		return errors.New("任务结果必须是对象")
	}
	return nil
}

func validateTaskColumns(task Task) error {
	if strings.TrimSpace(task.ID) == "" || strings.TrimSpace(task.Skill) == "" || strings.TrimSpace(task.Operation) == "" || strings.TrimSpace(task.Message) == "" {
		return errors.New("任务标识、技能、操作和消息不能为空")
	}
	if _, ok := validStatuses[task.Status]; !ok || task.Progress < 0 || task.Progress > 100 {
		return errors.New("任务状态或进度无效")
	}
	if _, err := time.Parse(time.RFC3339Nano, task.CreatedAt); err != nil {
		return errors.New("任务创建时间无效")
	}
	if _, err := time.Parse(time.RFC3339Nano, task.UpdatedAt); err != nil {
		return errors.New("任务更新时间无效")
	}
	return nil
}

func corruptTask(task Task, message string) Task {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := time.Parse(time.RFC3339Nano, task.CreatedAt); err != nil {
		task.CreatedAt = now
	}
	if _, err := time.Parse(time.RFC3339Nano, task.UpdatedAt); err != nil {
		task.UpdatedAt = task.CreatedAt
	}
	task.Status, task.Progress, task.Message = "failed", 100, message
	task.Result = map[string]any{"error": message, "storage_corrupt": true}
	return task
}
