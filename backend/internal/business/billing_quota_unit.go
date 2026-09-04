package business

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// ValidateNewAPIQuotaUnit records the live unit and proves that the report
// window has no known unit change. A first observation establishes the unit
// baseline; later conflicting observations still fail closed.
func (s *Store) ValidateNewAPIQuotaUnit(ctx context.Context, host string, rawUnit string, start, end time.Time) error {
	host = canonicalHost(host)
	unit, unitErr := positiveQuotaUnit(rawUnit)
	if host == "" || unitErr != nil || !end.After(start) {
		return errors.New("NewAPI quota_per_unit 校验参数无效")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := seedQuotaUnitFromUpstreamMetadata(ctx, tx, host, start); err != nil {
		return err
	}
	unitText := quotaUnitText(unit)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT INTO billing_quota_unit_observations(host,observed_at,quota_per_unit)
		VALUES(?,?,?) ON CONFLICT(host,observed_at) DO UPDATE SET quota_per_unit=excluded.quota_per_unit`, host, now, unitText); err != nil {
		return err
	}

	var baselineText string
	err = tx.QueryRowContext(ctx, `SELECT quota_per_unit FROM billing_quota_unit_observations
		WHERE host=? AND julianday(observed_at)<=julianday(?) ORDER BY julianday(observed_at) DESC LIMIT 1`,
		host, start.UTC().Format(time.RFC3339Nano)).Scan(&baselineText)
	var validationErr error
	if errors.Is(err, sql.ErrNoRows) {
		rows, queryErr := tx.QueryContext(ctx, `SELECT DISTINCT quota_per_unit
			FROM billing_quota_unit_observations WHERE host=?`, host)
		if queryErr != nil {
			return queryErr
		}
		for rows.Next() {
			var observedText string
			if scanErr := rows.Scan(&observedText); scanErr != nil {
				rows.Close()
				return scanErr
			}
			observed, parseErr := positiveQuotaUnit(observedText)
			if parseErr != nil {
				rows.Close()
				return parseErr
			}
			if observed.Cmp(unit) != 0 {
				validationErr = fmt.Errorf("NewAPI quota_per_unit 当前值 %s 与已知历史值 %s 不一致", unitText, observedText)
				break
			}
		}
		if closeErr := rows.Close(); closeErr != nil {
			return closeErr
		}
		if rowsErr := rows.Err(); rowsErr != nil {
			return rowsErr
		}
	} else if err != nil {
		return err
	} else {
		baseline, parseErr := positiveQuotaUnit(baselineText)
		if parseErr != nil {
			return parseErr
		}
		if baseline.Cmp(unit) != 0 {
			validationErr = fmt.Errorf("NewAPI quota_per_unit 当前值 %s 与报告日前基线 %s 不一致", unitText, baselineText)
		} else {
			rows, queryErr := tx.QueryContext(ctx, `SELECT quota_per_unit FROM billing_quota_unit_observations
				WHERE host=? AND julianday(observed_at)>julianday(?) AND julianday(observed_at)<=julianday(?)
				ORDER BY julianday(observed_at)`, host,
				start.UTC().Format(time.RFC3339Nano), end.UTC().Format(time.RFC3339Nano))
			if queryErr != nil {
				return queryErr
			}
			for rows.Next() {
				var observedText string
				if err := rows.Scan(&observedText); err != nil {
					rows.Close()
					return err
				}
				observed, parseErr := positiveQuotaUnit(observedText)
				if parseErr != nil {
					rows.Close()
					return parseErr
				}
				if observed.Cmp(baseline) != 0 {
					validationErr = errors.New("NewAPI quota_per_unit 在报告窗口内存在已知变更")
					break
				}
			}
			if err := rows.Close(); err != nil {
				return err
			}
			if err := rows.Err(); err != nil {
				return err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return validationErr
}

func seedQuotaUnitFromUpstreamMetadata(ctx context.Context, tx *sql.Tx, host string, start time.Time) error {
	var metadataRaw string
	var checkedAt sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT metadata_json,checked_at FROM upstreams WHERE host=?`, host).Scan(&metadataRaw, &checkedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if !checkedAt.Valid {
		return nil
	}
	observedAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(checkedAt.String))
	if err != nil || observedAt.After(start) {
		return nil
	}
	metadata, err := decodeObject(metadataRaw)
	if err != nil {
		return nil
	}
	unitText := strings.TrimSpace(stringValue(metadata["quota_per_unit"]))
	if _, err := positiveQuotaUnit(unitText); err != nil {
		return nil
	}
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO billing_quota_unit_observations(host,observed_at,quota_per_unit) VALUES(?,?,?)`,
		host, observedAt.UTC().Format(time.RFC3339Nano), unitText)
	return err
}

func positiveQuotaUnit(raw string) (*big.Rat, error) {
	value, ok := new(big.Rat).SetString(strings.TrimSpace(raw))
	if !ok || value.Sign() <= 0 {
		return nil, errors.New("已保存的 NewAPI quota_per_unit 历史值无效")
	}
	return value, nil
}

func quotaUnitText(value *big.Rat) string {
	text := strings.TrimRight(strings.TrimRight(value.FloatString(28), "0"), ".")
	if text == "" {
		return "0"
	}
	return text
}
