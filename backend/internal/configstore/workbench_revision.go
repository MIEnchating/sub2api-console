package configstore

import (
	"context"
	"database/sql"
	"errors"
)

// WorkbenchDocumentRevision reads the current storage version without exposing
// private document contents. Missing documents have revision zero, as in
// WorkbenchDocument. Callers remain responsible for content validation.
func (s *Store) WorkbenchDocumentRevision(ctx context.Context, key string) (int64, error) {
	var revision int64
	err := s.db.QueryRowContext(ctx, `SELECT revision FROM account_workbench_documents WHERE id=?`, key).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return revision, err
}
