package configstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
	rows, err := s.db.QueryContext(ctx, `SELECT id,revision,payload FROM account_workbench_documents WHERE substr(id,1,?)=? AND id>? ORDER BY id LIMIT ?`, len(prefix), prefix, after, limit)
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
	if prefix == "" || len(prefix) > 200 || limit < 1 || limit > 501 {
		return nil, errors.New("工作台查询范围无效")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,revision,payload FROM account_workbench_documents WHERE substr(id,1,?)=? ORDER BY rowid DESC LIMIT ?`, len(prefix), prefix, limit)
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

func (s *Store) DeleteWorkbenchDocument(ctx context.Context, key string, revision int64) error {
	s.workbenchWriteMu.Lock()
	defer s.workbenchWriteMu.Unlock()
	result, err := s.db.ExecContext(ctx, `DELETE FROM account_workbench_documents WHERE id=? AND revision=?`, key, revision)
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
	err := s.db.QueryRowContext(ctx, `SELECT payload,revision FROM account_workbench_documents WHERE id=?`, key).Scan(&raw, &revision)
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
		result, err = s.db.ExecContext(ctx, `INSERT INTO account_workbench_documents(id,revision,payload) VALUES(?,1,?) ON CONFLICT(id) DO NOTHING`, key, []byte(raw))
	} else {
		result, err = s.db.ExecContext(ctx, `UPDATE account_workbench_documents SET revision=revision+1,payload=? WHERE id=? AND revision=?`, []byte(raw), key, revision)
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
