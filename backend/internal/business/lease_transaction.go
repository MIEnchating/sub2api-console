package business

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"slices"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
)

// Lock identities separately from data rows, including rows not yet created.
// Hash collisions only serialize unrelated resources; they never share data.
func (s *Store) beginResourceTx(ctx context.Context, names []string) (*dbconn.Tx, error) {
	options := &sql.TxOptions{}
	if s.db.Driver() != dbconn.SQLite {
		options.Isolation = sql.LevelReadCommitted
	}
	tx, err := s.db.BeginTx(ctx, options)
	if err != nil {
		return nil, err
	}
	if tx.Driver() == dbconn.SQLite {
		return tx, nil
	}
	keys := make([]string, 0, len(names))
	for _, name := range names {
		digest := sha256.Sum256([]byte(name))
		keys = append(keys, hex.EncodeToString(digest[:]))
	}
	slices.Sort(keys)
	for _, key := range slices.Compact(keys) {
		if _, err := tx.ExecContext(ctx, tx.Dialect(`INSERT INTO business_resource_locks(id) VALUES(?) ON CONFLICT(id) DO UPDATE SET id=excluded.id`, `INSERT INTO business_resource_locks(id) VALUES(?) ON DUPLICATE KEY UPDATE id=VALUES(id)`), key); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}
	return tx, nil
}

func (s *Store) beginPolicyTx(ctx context.Context, key string) (*dbconn.Tx, error) {
	resources := []string{"policy/" + key}
	if key == "control-plane" {
		resources = append(resources, "setting/config")
	}
	return s.beginResourceTx(ctx, resources)
}

// Parse RFC3339Nano in Go rather than rounding lease deadlines through SQL
// date functions. The resource locks fence replacement for the entire renewal.
func (s *Store) renewLeaseNames(ctx context.Context, owner string, names []string, now time.Time, ttl time.Duration) (bool, error) {
	tx, err := s.beginResourceTx(ctx, names)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	for start := 0; start < len(names); start += mutationLeaseSQLChunk {
		chunk := names[start:min(start+mutationLeaseSQLChunk, len(names))]
		placeholders, args := mutationLeaseArguments(chunk)
		rows, err := tx.QueryContext(ctx, `SELECT lease_name,owner_id,expires_at FROM scheduler_leases WHERE lease_name IN (`+placeholders+`)`, args...)
		if err != nil {
			return false, err
		}
		count := 0
		valid := true
		for rows.Next() {
			var name, currentOwner, deadline string
			if err := rows.Scan(&name, &currentOwner, &deadline); err != nil {
				rows.Close()
				return false, err
			}
			expires, err := time.Parse(time.RFC3339Nano, deadline)
			if err != nil {
				rows.Close()
				return false, fmt.Errorf("租约 %s 的到期时间无效", name)
			}
			valid = valid && currentOwner == owner && expires.After(now)
			count++
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return false, err
		}
		if err := rows.Close(); err != nil {
			return false, err
		}
		if !valid || count != len(chunk) {
			return false, nil
		}
	}
	for start := 0; start < len(names); start += mutationLeaseSQLChunk {
		chunk := names[start:min(start+mutationLeaseSQLChunk, len(names))]
		placeholders, resources := mutationLeaseArguments(chunk)
		args := append([]any{now.UTC().Format(time.RFC3339Nano), now.UTC().Add(ttl).Format(time.RFC3339Nano), owner}, resources...)
		result, err := tx.ExecContext(ctx, `UPDATE scheduler_leases SET renewed_at=?,expires_at=? WHERE owner_id=? AND lease_name IN (`+placeholders+`)`, args...)
		if err != nil {
			return false, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return false, err
		}
		if count != int64(len(chunk)) {
			return false, nil
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
