package configstore

import (
	"context"
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
)

// VisitWorkbenchDocuments visits at most limit records in reverse insertion
// order within an exact namespace. Returning false stops the visit.
// The visitor must only inspect the record: it must not call the store or
// perform network requests while the query holds a database connection.
func (s *Store) VisitWorkbenchDocuments(ctx context.Context, prefix string, limit int, visit func(WorkbenchDocumentRecord) (bool, error)) error {
	if prefix == "" || len(prefix) > 200 || limit < 1 || limit > 501 {
		return errors.New("工作台查询范围无效")
	}
	where, args := s.workbenchDocumentRange(prefix)
	args = append(args, limit)
	// Sort narrow rowids before loading private JSON. Stream only the rows the
	// caller inspects, keeping one record at a time rather than the entire list.
	query := `SELECT id,revision,payload FROM account_workbench_documents WHERE rowid IN (
		SELECT rowid FROM account_workbench_documents WHERE ` + where + ` ORDER BY rowid DESC LIMIT ?
 ) ORDER BY rowid DESC`
	if s.db.Driver() != dbconn.SQLite {
		query = `SELECT id,revision,payload FROM account_workbench_documents WHERE ` + where + ` ORDER BY insertion_order DESC LIMIT ?`
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var record WorkbenchDocumentRecord
		if err := rows.Scan(&record.ID, &record.Revision, &record.Payload); err != nil {
			return err
		}
		more, err := visit(record)
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !more {
			return rows.Close()
		}
	}
	return rows.Err()
}
