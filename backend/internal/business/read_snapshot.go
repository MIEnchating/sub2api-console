package business

import (
	"context"
	"database/sql"
	"time"
)

type readSnapshotKey struct {
	database *database
}

type readSnapshot struct {
	tx         *sql.Tx
	capturedAt time.Time
}

func (db *database) readSnapshot(ctx context.Context) *sql.Tx {
	snapshot, _ := ctx.Value(readSnapshotKey{database: db}).(readSnapshot)
	return snapshot.tx
}

// ReadSnapshotTime identifies when this store's current read snapshot was
// acquired, rather than when a later calculation within it happened to finish.
func (s *Store) ReadSnapshotTime(ctx context.Context) (time.Time, bool) {
	snapshot, found := ctx.Value(readSnapshotKey{database: s.db}).(readSnapshot)
	return snapshot.capturedAt, found
}

func (db *database) captureReadSnapshot(ctx context.Context, tx *sql.Tx) (readSnapshot, error) {
	db.snapshotLock.Lock()
	defer db.snapshotLock.Unlock()
	// BEGIN is deferred: the first real table read pins SQLite's WAL snapshot.
	// Keep pinning and timestamp assignment together so competing snapshots
	// retain their acquisition order without serializing the callback's work.
	var hasState bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app_state)`).Scan(&hasState); err != nil {
		return readSnapshot{}, err
	}
	return readSnapshot{tx: tx, capturedAt: time.Now().UTC()}, nil
}

// WithReadSnapshot keeps this store's queries in one read-only transaction for
// the callback. Nested calls reuse the transaction; other stores stay isolated.
// The callback must finish reading before it returns and must not retain ctx.
func (s *Store) WithReadSnapshot(ctx context.Context, read func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.db.readSnapshot(ctx) != nil {
		return preferContextError(ctx, read(ctx))
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return preferContextError(ctx, err)
	}
	defer tx.Rollback()
	view, err := s.db.captureReadSnapshot(ctx, tx)
	if err != nil {
		return preferContextError(ctx, err)
	}
	snapshot := context.WithValue(ctx, readSnapshotKey{database: s.db}, view)
	return preferContextError(ctx, read(snapshot))
}
