package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// RoutingRecoveryTraffic retains the complete hold interval and its preceding
// observation even when high traffic has rotated the bounded health cache.
func (s *Store) RoutingRecoveryTraffic(ctx context.Context, accountID string, since, before time.Time) ([]RoutingSample, error) {
	query := `WITH recent AS (
		SELECT id,request_id,group_name,is_error,error_reason,first_token_ms,observed_at,payload_json
		FROM usage_records WHERE LOWER(REPLACE(source,'_','-'))='traffic' AND account_id=?
		AND COALESCE(observed_at,'')>=? AND COALESCE(observed_at,'')<=?
	), boundary AS (
		SELECT id,request_id,group_name,is_error,error_reason,first_token_ms,observed_at,payload_json
		FROM usage_records WHERE LOWER(REPLACE(source,'_','-'))='traffic' AND account_id=?
		AND COALESCE(observed_at,'')<? ORDER BY COALESCE(observed_at,'') DESC,id DESC LIMIT 1
	)
	SELECT * FROM recent UNION ALL SELECT * FROM boundary ORDER BY observed_at DESC,id DESC`
	rows, err := s.db.QueryContext(ctx, query, accountID, since.UTC().Format(healthSampleTimeLayout), before.UTC().Format(healthSampleTimeLayout), accountID, since.UTC().Format(healthSampleTimeLayout))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []RoutingSample{}
	seen := map[string]struct{}{}
	for rows.Next() {
		var id int64
		var requestID, raw string
		var group, reason, firstToken, observed sql.NullString
		var failed sql.NullInt64
		if err := rows.Scan(&id, &requestID, &group, &failed, &reason, &firstToken, &observed, &raw); err != nil {
			return nil, err
		}
		if _, exists := seen[requestID]; exists {
			continue
		}
		seen[requestID] = struct{}{}
		item := RoutingSample{AccountID: accountID, GroupName: group.String, Source: "traffic", Result: "通过", FailureReason: reason.String, ObservedAt: observed.String}
		if !failed.Valid || failed.Int64 != 0 {
			item.Result = "失败"
		}
		if err := json.Unmarshal([]byte(raw), &item.Payload); err != nil || item.Payload == nil {
			return nil, fmt.Errorf("账号 %s 的恢复流量样本损坏", accountID)
		}
		item.Payload["request_id"] = requestID
		if item.Payload["first_token_ms"] == nil && firstToken.Valid {
			item.Payload["first_token_ms"] = firstToken.String
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
