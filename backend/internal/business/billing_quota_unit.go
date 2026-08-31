package business

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// ValidateNewAPIQuotaUnit records the live unit and proves that the report
// window has a known, unchanged baseline. The observation is committed even
// when the older window cannot yet be validated, so later completed days can
// be reconciled exactly.
func (s *Store) ValidateNewAPIQuotaUnit(ctx context.Context, host string, unit float64, start, end time.Time) error {
	host = canonicalHost(host)
	if host == "" || math.IsNaN(unit) || math.IsInf(unit, 0) || unit <= 0 || !end.After(start) {
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
	unitText := strconv.FormatFloat(unit, 'g', -1, 64)
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
		validationErr = errors.New("NewAPI quota_per_unit 尚无报告日前历史基线，本次仅建立观察，无法精确核对")
	} else if err != nil {
		return err
	} else {
		baseline, parseErr := positiveQuotaUnit(baselineText)
		if parseErr != nil {
			return parseErr
		}
		if baseline != unit {
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
				if observed != baseline {
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
	if errors.Is(err, sql.ErrNoRows) || !checkedAt.Valid {
		return nil
	}
	if err != nil {
		return err
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

func positiveQuotaUnit(raw string) (float64, error) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return 0, errors.New("已保存的 NewAPI quota_per_unit 历史值无效")
	}
	return value, nil
}
