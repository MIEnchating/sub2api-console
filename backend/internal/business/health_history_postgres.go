package business

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Fetch bounded indexed pages in one round trip for all accounts. Continue only
// accounts whose duplicate group evidence has not filled their unique window.
func (s *Store) selectPostgresHealthWindows(ctx context.Context, ids []string, clauses []string, arguments []any, limit int, normalizeSource bool) ([]healthSampleSelection, error) {
	if len(ids) == 0 || limit < 1 {
		return []healthSampleSelection{}, nil
	}
	pageSize := min(max(limit, 32), 500)
	pending := append([]string(nil), ids...)
	selected := make(map[string][]healthSampleSelection, len(ids))
	seen := make(map[string]map[string]struct{}, len(ids))
	for offset := 0; len(pending) > 0; offset += pageSize {
		raw, err := json.Marshal(pending)
		if err != nil {
			return nil, err
		}
		filters := append(append([]string{}, clauses...), "account_id=scope.value")
		query := `SELECT page.id,page.account_id,page.observed_at,page.source,page.evidence_key
   FROM json_array_elements_text(?::json) AS scope(value)
   CROSS JOIN LATERAL (SELECT id,account_id,observed_at,source,evidence_key FROM health_samples` + whereSQL(filters) + `
   ORDER BY observed_at DESC,id DESC LIMIT ? OFFSET ?) page
   ORDER BY page.account_id,page.observed_at DESC,page.id DESC`
		args := append([]any{string(raw)}, arguments...)
		args = append(args, pageSize, offset)
		rows, err := s.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		counts := make(map[string]int, len(pending))
		for rows.Next() {
			var item healthSampleCandidate
			if err := rows.Scan(&item.id, &item.accountID, &item.observedAt, &item.source, &item.evidenceKey); err != nil {
				rows.Close()
				return nil, err
			}
			counts[item.accountID]++
			if len(selected[item.accountID]) >= limit {
				continue
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
			if seen[item.accountID] == nil {
				seen[item.accountID] = map[string]struct{}{}
			}
			if _, exists := seen[item.accountID][key]; exists {
				continue
			}
			seen[item.accountID][key] = struct{}{}
			selected[item.accountID] = append(selected[item.accountID], healthSampleSelection{id: item.id, evidenceKey: evidence})
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
		next := make([]string, 0, len(pending))
		for _, id := range pending {
			if counts[id] == pageSize && len(selected[id]) < limit {
				next = append(next, id)
			}
		}
		pending = next
	}
	result := make([]healthSampleSelection, 0, len(ids)*limit)
	for _, id := range ids {
		result = append(result, selected[id]...)
	}
	return result, nil
}
