package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"strings"
	"time"
)

// RoutingRecoveryTraffic retains the complete hold interval and its preceding
// observation even when high traffic has rotated the bounded health cache.
func (s *Store) RoutingRecoveryTraffic(ctx context.Context, accountID string, since, before time.Time) ([]RoutingSample, error) {
	query := `WITH recent AS (
		SELECT id,request_id,console_recovery_time_v1(observed_at) observed_time
		FROM usage_records WHERE LOWER(REPLACE(source,'_','-'))='traffic' AND account_id=?
		AND console_recovery_time_v1(observed_at)>=? AND console_recovery_time_v1(observed_at)<=?
	), boundary AS (
		SELECT id,request_id,console_recovery_time_v1(observed_at) observed_time
		FROM usage_records WHERE LOWER(REPLACE(source,'_','-'))='traffic' AND account_id=?
		AND console_recovery_time_v1(observed_at)<?
		ORDER BY console_recovery_time_v1(observed_at) DESC,id DESC LIMIT 1
	), ranked AS (
		SELECT id,observed_time,ROW_NUMBER() OVER (
			PARTITION BY request_id ORDER BY observed_time DESC,id DESC
		) request_rank FROM (SELECT * FROM recent UNION ALL SELECT * FROM boundary) window_rows
	)
	SELECT u.id,u.request_id,u.group_name,u.is_error,u.error_reason,u.first_token_ms,u.observed_at,u.payload_json
	FROM ranked r JOIN usage_records u ON u.id=r.id
	WHERE r.request_rank=1 ORDER BY r.observed_time DESC,r.id DESC`
	if s.db.Driver() != dbconn.SQLite {
		query = strings.ReplaceAll(query, "console_recovery_time_v1(observed_at)", "observed_key")
	}
	start, end := routingRecoveryTimeKey(since), routingRecoveryTimeKey(before)
	rows, err := s.db.QueryContext(ctx, query, accountID, start, end, accountID, start)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []RoutingSample{}
	for rows.Next() {
		var id int64
		var requestID, raw string
		var group, reason, firstToken, observed sql.NullString
		var failed sql.NullInt64
		if err := rows.Scan(&id, &requestID, &group, &failed, &reason, &firstToken, &observed, &raw); err != nil {
			return nil, err
		}
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
