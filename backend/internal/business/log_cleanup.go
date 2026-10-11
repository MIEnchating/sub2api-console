package business

import (
	"context"
	"strconv"
	"time"
)

const logCleanupBatchSize = 2000

func (s *Store) ClearLogRecords(ctx context.Context, before *time.Time) (LogCleanupResult, error) {
	return s.ClearLogRecordsWithProgress(ctx, before, nil)
}

// Each statement commits at most 2000 rows, releasing the writer between batches.
// Progress contains committed counts, including when a later batch fails.
func (s *Store) ClearLogRecordsWithProgress(ctx context.Context, before *time.Time, report func(LogCleanupResult) error) (LogCleanupResult, error) {
	result := LogCleanupResult{}
	deletions := []struct {
		table, column, extra string
		count                *int64
	}{
		{"run_records", "updated_at", " AND COALESCE(status,'') NOT IN ('queued','running','waiting_input')", &result.Runs},
		{"runtime_events", "created_at", "", &result.Events},
		{"operation_audit", "created_at", "", &result.Changes},
	}
	for _, deletion := range deletions {
		where := " WHERE 1=1" + deletion.extra
		args := []any{}
		if before != nil {
			// A broad indexed upper bound excludes recent history without relying
			// on fractional width or UTC offsets for the actual retention decision.
			// 48h also covers the boundary offsets accepted by Go's RFC3339 parser.
			upper := before.UTC().Add(48 * time.Hour)
			if upper.Year() >= 0 && upper.Year() <= 9999 {
				where += " AND " + deletion.column + " < ?"
				args = append(args, upper.Format(time.RFC3339))
			}
			where += " AND console_log_before(" + deletion.column + ",?,?)"
			args = append(args, before.Unix(), before.Nanosecond())
		}
		cursor := ""
		for {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			batchWhere := where
			batchArgs := append([]any(nil), args...)
			if cursor != "" {
				// Equal timestamps can span batches. Keep the boundary inclusive;
				// already deleted rows are gone, and all equal dates have the same age.
				batchWhere += " AND " + deletion.column + " >= ?"
				batchArgs = append(batchArgs, cursor)
			}
			selection := "SELECT rowid," + deletion.column + " AS stamp FROM " + deletion.table + batchWhere + " ORDER BY " + deletion.column + " LIMIT " + strconv.Itoa(logCleanupBatchSize)
			count, next, err := s.clearLogRecordBatch(ctx, deletion.table, selection, batchArgs, before != nil)
			if err != nil {
				return result, err
			}
			cursor = next
			*deletion.count += count
			if count > 0 && report != nil {
				if err := report(result); err != nil {
					return result, err
				}
			}
			if count < logCleanupBatchSize {
				break
			}
		}
	}
	return result, nil
}

func (s *Store) clearLogRecordBatch(ctx context.Context, table, selection string, args []any, retainDates bool) (int64, string, error) {
	if !retainDates {
		query := "DELETE FROM " + table + " WHERE rowid IN (SELECT rowid FROM (" + selection + "))"
		execution, err := s.db.ExecContext(ctx, query, args...)
		if err != nil {
			return 0, "", err
		}
		count, err := execution.RowsAffected()
		return count, "", err
	}
	// Keep only a bounded batch of rowids and timestamps inside SQLite. The
	// cursor and progress become visible only after the deletion commits.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, "", err
	}
	defer tx.Rollback()
	for _, statement := range []string{
		"CREATE TEMP TABLE IF NOT EXISTS business_log_cleanup_batch(id INTEGER NOT NULL,stamp TEXT NOT NULL)",
		"DELETE FROM business_log_cleanup_batch",
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return 0, "", err
		}
	}
	selected, err := tx.ExecContext(ctx, "INSERT INTO business_log_cleanup_batch(id,stamp) "+selection, args...)
	if err != nil {
		return 0, "", err
	}
	count, err := selected.RowsAffected()
	if err != nil || count == 0 {
		return 0, "", err
	}
	next := ""
	if count == logCleanupBatchSize {
		// Selection is time-ordered; implicit rowids preserve its insertion order
		// without sorting by source ID or scanning the batch for its maximum date.
		if err := tx.QueryRowContext(ctx, "SELECT stamp FROM business_log_cleanup_batch ORDER BY rowid DESC LIMIT 1").Scan(&next); err != nil {
			return 0, "", err
		}
	}
	execution, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE rowid IN (SELECT id FROM business_log_cleanup_batch)")
	if err != nil {
		return 0, "", err
	}
	count, err = execution.RowsAffected()
	if err != nil {
		return 0, "", err
	}
	if err := tx.Commit(); err != nil {
		return 0, "", err
	}
	return count, next, nil
}
