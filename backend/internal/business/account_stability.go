package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountquality"
	"github.com/MIEnchating/sub2api-console/backend/internal/usagequality"
)

func (s *Store) loadAccountStability(ctx context.Context, accounts map[string]*accountProjection) error {
	now := time.Now().UTC()
	for _, account := range accounts {
		stats := accountquality.New(now)
		account.Stability = &stats
	}
	if len(accounts) == 0 {
		return nil
	}
	args := []any{now.Add(-24 * time.Hour).Format(healthSampleTimeLayout), now.Add(-30 * 24 * time.Hour).Format(healthSampleTimeLayout), now.Format(healthSampleTimeLayout)}
	ids := make([]string, 0, len(accounts))
	for id := range accounts {
		ids = append(ids, "?")
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT account_id,outcome,COUNT(*),SUM(CASE WHEN observed_at>=? THEN 1 ELSE 0 END) FROM account_stability_samples
 WHERE observed_at>=? AND observed_at<=? AND account_id IN (`+strings.Join(ids, ",")+`) GROUP BY account_id,outcome`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, outcome string
		var longCount, shortCount int
		if err := rows.Scan(&id, &outcome, &longCount, &shortCount); err != nil {
			return err
		}
		addStabilityCount(&accounts[id].Stability.Short, outcome, shortCount)
		addStabilityCount(&accounts[id].Stability.Long, outcome, longCount)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, account := range accounts {
		account.Stability.Calculate(false)
	}
	return nil
}

func addStabilityCount(window *accountquality.Window, outcome string, count int) {
	window.Samples += count
	switch outcome {
	case "passed":
		window.Passed += count
	case "failed":
		window.Failed += count
	default:
		window.Inconclusive += count
	}
}

func persistStabilitySample(ctx context.Context, tx *sql.Tx, accountID, source, key, at, outcome string, usageKnown bool) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO account_stability_samples(account_id,source,evidence_key,observed_at,outcome,usage_known)
 VALUES(?,?,?,?,?,?) ON CONFLICT(account_id,source,evidence_key) DO UPDATE SET outcome=excluded.outcome,usage_known=excluded.usage_known
 WHERE excluded.observed_at>=account_stability_samples.observed_at
 AND (excluded.usage_known=1 OR account_stability_samples.usage_known=0)`, accountID, source, key, at, outcome, usageKnown)
	return err
}

func stabilityUsageKnown(raw string) bool {
	var payload map[string]any
	if json.Unmarshal([]byte(raw), &payload) != nil {
		return false
	}
	if value, ok := payload["token_usage"]; ok {
		payload, _ = value.(map[string]any)
	}
	_, known := usagequality.Normalize(payload)
	return known
}

func stabilityOutcome(result, reason, source, raw string) string {
	switch strings.ToLower(strings.TrimSpace(result)) {
	case "通过", "passed", "pass", "success", "succeeded", "healthy", "ok":
		if strings.EqualFold(source, "traffic") {
			var payload map[string]any
			if json.Unmarshal([]byte(raw), &payload) == nil {
				usage := payload
				if value, ok := payload["token_usage"]; ok {
					usage, _ = value.(map[string]any)
				}
				if usagequality.Empty(usage) {
					return "failed"
				}
			}
		}
		return "passed"
	case "失败", "failed", "error", "timeout", "超时", "probe failed", "unhealthy":
		return "failed"
	case "", "跳过", "skipped", "cancelled":
		return "inconclusive"
	default:
		if strings.TrimSpace(reason) != "" {
			return "failed"
		}
		return "inconclusive"
	}
}
