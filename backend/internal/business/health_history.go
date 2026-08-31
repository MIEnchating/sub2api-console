package business

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type healthSampleSelection struct {
	id int64
}

type selectedHealthSample struct {
	id            int64
	accountID     string
	groupName     string
	result        sql.NullString
	latencyP50    sql.NullString
	latencyP95    sql.NullString
	failureReason sql.NullString
	observedAt    sql.NullString
	source        string
	payloadJSON   string
}

type healthSampleCandidate struct {
	id          int64
	accountID   string
	observedAt  sql.NullString
	source      string
	evidenceKey sql.NullString
}

func (s *Store) selectHealthSampleWindow(
	ctx context.Context,
	clauses []string,
	arguments []any,
	limit int,
	normalizeSource bool,
	coalesceObservedAt bool,
) ([]healthSampleSelection, error) {
	query := `SELECT id,account_id,observed_at,source,evidence_key
		FROM health_samples INDEXED BY ix_health_samples_account_recent` + whereSQL(clauses)
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]healthSampleSelection, 0)
	seen := map[string]struct{}{}
	currentAccount := ""
	selectedForAccount := 0
	for rows.Next() {
		var item healthSampleCandidate
		if err := rows.Scan(&item.id, &item.accountID, &item.observedAt, &item.source, &item.evidenceKey); err != nil {
			return nil, err
		}
		if item.accountID != currentAccount {
			currentAccount = item.accountID
			selectedForAccount = 0
			clear(seen)
		}
		source := item.source
		if normalizeSource {
			source = strings.ToLower(strings.ReplaceAll(source, "_", "-"))
		}
		evidence := item.evidenceKey.String
		if !item.evidenceKey.Valid || evidence == "" {
			evidence = fmt.Sprintf("row:%d", item.id)
		}
		key := source + "\x00" + evidence
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		if selectedForAccount >= limit {
			continue
		}
		result = append(result, healthSampleSelection{id: item.id})
		selectedForAccount++
	}
	return result, rows.Err()
}

func (s *Store) selectedHealthSamples(
	ctx context.Context,
	selections []healthSampleSelection,
) (map[int64]selectedHealthSample, error) {
	const batchSize = 500
	result := make(map[int64]selectedHealthSample, len(selections))
	for start := 0; start < len(selections); start += batchSize {
		end := min(start+batchSize, len(selections))
		arguments := make([]any, end-start)
		for index, selection := range selections[start:end] {
			arguments[index] = selection.id
		}
		query := `SELECT id,account_id,group_name,result,latency_p50,latency_p95,failure_reason,
			observed_at,source,payload_json FROM health_samples WHERE id IN (` +
			strings.TrimSuffix(strings.Repeat("?,", len(arguments)), ",") + `)`
		rows, err := s.db.QueryContext(ctx, query, arguments...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var item selectedHealthSample
			if err := rows.Scan(
				&item.id, &item.accountID, &item.groupName, &item.result, &item.latencyP50,
				&item.latencyP95, &item.failureReason, &item.observedAt, &item.source, &item.payloadJSON,
			); err != nil {
				rows.Close()
				return nil, err
			}
			result[item.id] = item
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return result, nil
}
