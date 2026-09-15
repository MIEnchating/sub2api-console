package configstore

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

type executionCleanup struct {
	key       string
	expected  string
	expiresAt string
	updated   string
}

// PurgeExpiredWorkbenchExecutions scans without a write transaction. Each
// bounded page is then conditionally applied in a short local write transaction.
func (s *Store) PurgeExpiredWorkbenchExecutions(ctx context.Context) error {
	after := ""
	for {
		plans, cursor, count, err := s.executionCleanupPage(ctx, after, s.workbenchExecutionNow())
		if err != nil {
			return err
		}
		if len(plans) > 0 {
			if err := s.applyExecutionCleanup(ctx, plans); err != nil {
				return err
			}
		}
		if count < 64 {
			return nil
		}
		after = cursor
	}
}

func (s *Store) executionCleanupPage(ctx context.Context, after string, now time.Time) ([]executionCleanup, string, int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key,value FROM settings WHERE (key GLOB 'account_workbench.execution.*' OR key GLOB 'account_workbench.execution_claims.*') AND key>? ORDER BY key LIMIT 64`, after)
	if err != nil {
		return nil, "", 0, err
	}
	defer func() { _ = rows.Close() }()
	plans := []executionCleanup{}
	cursor, count := after, 0
	for rows.Next() {
		var key, raw string
		if err := rows.Scan(&key, &raw); err != nil {
			return nil, "", 0, err
		}
		cursor, count = key, count+1
		if id, execution := strings.CutPrefix(key, workbenchExecutionPrefix); execution {
			var record WorkbenchExecution
			if json.Unmarshal([]byte(raw), &record) != nil || record.ID != id || record.Revision < 1 || !validExecutionLifetime(record) {
				plans = append(plans, executionCleanup{key: key, expected: raw})
			} else if !executionUnexpired(record, now) {
				plans = append(plans, executionCleanup{key: key, expiresAt: record.ExpiresAt})
			}
			continue
		}
		id := strings.TrimPrefix(key, workbenchExecutionClaimsPrefix)
		claims, err := parseExecutionClaims(raw, id)
		if err != nil {
			plans = append(plans, executionCleanup{key: key, expected: raw})
			continue
		}
		live := make([]workbenchExecutionClaim, 0, len(claims.Claims))
		for _, claim := range claims.Claims {
			expires, _ := time.Parse(workbenchExecutionTimeLayout, claim.ExpiresAt)
			if now.Before(expires) {
				live = append(live, claim)
			}
		}
		if len(live) == len(claims.Claims) {
			continue
		}
		plan := executionCleanup{key: key, expected: raw}
		if len(live) > 0 {
			claims.Claims = live
			encoded, err := json.Marshal(claims)
			if err != nil {
				return nil, "", 0, err
			}
			plan.updated = string(encoded)
		}
		plans = append(plans, plan)
	}
	if err := rows.Err(); err != nil {
		return nil, "", 0, err
	}
	return plans, cursor, count, nil
}

func (s *Store) applyExecutionCleanup(ctx context.Context, plans []executionCleanup) error {
	s.workbenchExecutionWriteMu.Lock()
	defer s.workbenchExecutionWriteMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, plan := range plans {
		var err error
		switch {
		case plan.updated != "":
			_, err = tx.ExecContext(ctx, `UPDATE settings SET value=? WHERE key=? AND value=?`, plan.updated, plan.key, plan.expected)
		case plan.expiresAt != "":
			_, err = tx.ExecContext(ctx, `DELETE FROM settings WHERE key=? AND CASE WHEN json_valid(value) THEN json_extract(value,'$.expires_at') END=?`, plan.key, plan.expiresAt)
		default:
			_, err = tx.ExecContext(ctx, `DELETE FROM settings WHERE key=? AND value=?`, plan.key, plan.expected)
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
