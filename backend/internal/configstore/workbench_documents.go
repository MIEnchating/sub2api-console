package configstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"strings"
)

var ErrWorkbenchVersion = errors.New("工作台资料已变化，请刷新后重试")

type WorkbenchDocumentRecord struct {
	ID       string
	Revision int64
	Payload  json.RawMessage
}

// WorkbenchDocumentPage uses a stable key cursor so cleanup does not repeatedly
// scan the newest records and starve older sessions.
func (s *Store) WorkbenchDocumentPage(ctx context.Context, prefix, after string, limit int) ([]WorkbenchDocumentRecord, error) {
	if prefix == "" || len(prefix) > 200 || len(after) > 200 || limit < 1 || limit > 501 {
		return nil, errors.New("工作台查询范围无效")
	}
	where, args := s.workbenchDocumentRange(prefix)
	args = append(args, s.workbenchKey(after), limit)
	rows, err := s.db.QueryContext(ctx, `SELECT id,revision,payload FROM account_workbench_documents WHERE `+where+` AND id>? ORDER BY id LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []WorkbenchDocumentRecord{}
	for rows.Next() {
		var record WorkbenchDocumentRecord
		if err = rows.Scan(&record.ID, &record.Revision, &record.Payload); err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

// WorkbenchDocuments scopes reads to an exact namespace prefix. LIKE is not
// used because session and task identifiers must not become wildcard queries.
func (s *Store) WorkbenchDocuments(ctx context.Context, prefix string, limit int) ([]WorkbenchDocumentRecord, error) {
	result := []WorkbenchDocumentRecord{}
	err := s.VisitWorkbenchDocuments(ctx, prefix, limit, func(record WorkbenchDocumentRecord) (bool, error) {
		result = append(result, record)
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// The id primary key uses SQLite's BINARY collation. A half-open byte range
// matches a literal prefix, including Unicode and SQL wildcard characters,
// without applying a function to each stored key. Bounds are not document IDs
// and need not themselves contain complete UTF-8 code points.
func workbenchDocumentRange(prefix string) (string, []any) {
	upper := []byte(prefix)
	for i := len(upper) - 1; i >= 0; i-- {
		if upper[i] != 0xff {
			upper[i]++
			return "id>=? AND id<?", []any{prefix, string(upper[:i+1])}
		}
	}
	// An all-0xff prefix has no finite successor in the byte ordering.
	return "id>=?", []any{prefix}
}

func (s *Store) DeleteWorkbenchDocument(ctx context.Context, key string, revision int64) error {
	s.workbenchWriteMu.Lock()
	defer s.workbenchWriteMu.Unlock()
	result, err := s.db.ExecContext(ctx, `DELETE FROM account_workbench_documents WHERE id=? AND revision=?`, s.workbenchKey(key), revision)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrWorkbenchVersion
	}
	return nil
}

// WorkbenchDocument reads only the new workbench namespace, never legacy state.
func (s *Store) WorkbenchDocument(ctx context.Context, key string) (json.RawMessage, int64, error) {
	var raw []byte
	var revision int64
	err := s.db.QueryRowContext(ctx, `SELECT payload,revision FROM account_workbench_documents WHERE id=?`, s.workbenchKey(key)).Scan(&raw, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, nil
	}
	return raw, revision, err
}

func (s *Store) WorkbenchDocumentPublic(ctx context.Context, key string) (json.RawMessage, int64, error) {
	var raw []byte
	var revision int64
	expression := `json_extract(payload,'$.public')`
	switch s.db.Driver() {
	case dbconn.Postgres:
		expression = `convert_from(payload,'UTF8')::jsonb->'public'`
	case dbconn.MySQL:
		expression = `JSON_EXTRACT(CONVERT(payload USING utf8mb4),'$.public')`
	}
	err := s.db.QueryRowContext(ctx, `SELECT `+expression+`,revision FROM account_workbench_documents WHERE id=?`, s.workbenchKey(key)).Scan(&raw, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, nil
	}
	return raw, revision, err
}

func (s *Store) SaveWorkbenchDocument(ctx context.Context, key string, revision int64, raw json.RawMessage) (int64, error) {
	s.workbenchWriteMu.Lock()
	defer s.workbenchWriteMu.Unlock()
	if key == "" || len(key) > 200 || strings.ContainsRune(key, '\x00') || revision < 0 || !json.Valid(raw) || len(raw) > 16<<20 {
		return 0, errors.New("工作台资料格式无效")
	}
	var result sql.Result
	var err error
	if revision == 0 {
		result, err = s.db.InsertIfAbsentContext(ctx, `INSERT INTO account_workbench_documents(id,revision,payload) VALUES(?,1,?) ON CONFLICT(id) DO NOTHING`, s.workbenchKey(key), []byte(raw))
	} else {
		result, err = s.db.ExecContext(ctx, `UPDATE account_workbench_documents SET revision=revision+1,payload=? WHERE id=? AND revision=?`, []byte(raw), s.workbenchKey(key), revision)
	}
	if err != nil {
		return 0, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if count != 1 {
		return 0, ErrWorkbenchVersion
	}
	return revision + 1, nil
}

// Server engines compare binary keys, preserving SQLite byte-prefix semantics.
func (s *Store) workbenchKey(key string) any {
	if s.db.Driver() == dbconn.SQLite {
		return key
	}
	return []byte(key)
}
func (s *Store) workbenchDocumentRange(prefix string) (string, []any) {
	where, args := workbenchDocumentRange(prefix)
	for i := range args {
		args[i] = s.workbenchKey(args[i].(string))
	}
	return where, args
}
