package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type EvidenceTarget struct {
	AccountID      string
	GroupName      string
	GroupID        *string
	AccountType    string
	Platform       string
	EffectiveState string
	DecisionState  *string
	TrafficAt      *time.Time
	TrafficFetchAt *time.Time
	ProbeAt        *time.Time
}

type TrafficSample struct {
	AccountID     string
	GroupName     string
	Result        string
	LatencyP50    *string
	LatencyP95    *string
	LatencyP99    *string
	SampleCount   int
	Attempts      int
	FailureReason *string
	ObservedAt    string
	EvidenceKey   string
	Payload       map[string]any
}

func (s *Store) EvidenceTargets(ctx context.Context, accountID, groupName *string) ([]EvidenceTarget, error) {
	clauses := []string{}
	arguments := []any{}
	if accountID != nil {
		clauses = append(clauses, "a.id=?")
		arguments = append(arguments, strings.TrimSpace(*accountID))
	}
	if groupName != nil {
		clauses = append(clauses, "ag.group_name=?")
		arguments = append(arguments, strings.TrimSpace(*groupName))
	}
	query := `SELECT a.id,ag.group_name,ag.group_id,a.upstream_type,a.metadata_json,COALESCE(a.routing_state,''),
		(SELECT COALESCE(NULLIF(TRIM(rd.routing_state),''),NULLIF(TRIM(rd.role),'')) FROM routing_decisions rd
		 WHERE rd.account_id=a.id
		 AND (decision_epoch.updated_at IS NULL OR julianday(rd.updated_at)>=julianday(decision_epoch.updated_at))
		 ORDER BY julianday(rd.updated_at) DESC,rd.group_name LIMIT 1),
		(SELECT hs.observed_at FROM health_samples hs
		 WHERE hs.account_id=a.id AND LOWER(REPLACE(hs.source,'_','-'))='traffic'
		 ORDER BY hs.observed_at DESC,hs.id DESC LIMIT 1),
		(SELECT st.updated_at FROM app_state st WHERE st.key='evidence-traffic-fetch:' || a.id),
		(SELECT hs.observed_at FROM health_samples hs
		 WHERE hs.account_id=a.id AND LOWER(REPLACE(hs.source,'_','-'))='active-probe'
		 ORDER BY hs.observed_at DESC,hs.id DESC LIMIT 1)
		FROM accounts a JOIN account_groups ag ON ag.account_id=a.id
		LEFT JOIN app_state decision_epoch ON decision_epoch.key='routing-decision-epoch'`
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY CASE WHEN a.id GLOB '[0-9]*' THEN CAST(a.id AS INTEGER) ELSE 0 END,a.id,ag.group_name"
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []EvidenceTarget{}
	for rows.Next() {
		var item EvidenceTarget
		var groupID, upstreamType, decisionState, trafficAt, trafficFetchAt, probeAt sql.NullString
		var metadataRaw string
		if err := rows.Scan(&item.AccountID, &item.GroupName, &groupID, &upstreamType, &metadataRaw, &item.EffectiveState, &decisionState, &trafficAt, &trafficFetchAt, &probeAt); err != nil {
			return nil, err
		}
		metadata := map[string]any{}
		if err := json.Unmarshal([]byte(metadataRaw), &metadata); err != nil {
			return nil, fmt.Errorf("账号 %s metadata 配置无效", item.AccountID)
		}
		item.AccountType = strings.ToLower(strings.TrimSpace(evidenceMetadataText(metadata, "type", "account_type")))
		if item.AccountType == "" {
			item.AccountType = "apikey"
		}
		item.Platform = strings.ToLower(strings.TrimSpace(evidenceMetadataText(metadata, "platform")))
		if item.Platform == "" && upstreamType.Valid {
			item.Platform = strings.ToLower(strings.TrimSpace(upstreamType.String))
		}
		item.GroupID = nullString(groupID)
		item.DecisionState = nullString(decisionState)
		item.TrafficAt = parsedEvidenceTime(trafficAt)
		item.TrafficFetchAt = parsedEvidenceTime(trafficFetchAt)
		item.ProbeAt = parsedEvidenceTime(probeAt)
		result = append(result, item)
	}
	return result, rows.Err()
}

func evidenceMetadataText(metadata map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := metadata[key].(string); ok {
			return value
		}
	}
	return ""
}

func (s *Store) PersistTrafficFetches(ctx context.Context, accountIDs []string, observedAt time.Time) error {
	if len(accountIDs) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stamp := observedAt.UTC().Format(time.RFC3339Nano)
	for _, accountID := range accountIDs {
		accountID = strings.TrimSpace(accountID)
		if accountID == "" {
			return errors.New("真实流量拉取记录缺少账号 ID")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO app_state(key,value_json,updated_at) VALUES(?,?,?)
			ON CONFLICT(key) DO UPDATE SET value_json=excluded.value_json,updated_at=excluded.updated_at`,
			"evidence-traffic-fetch:"+accountID, `{}`, stamp); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) PersistTrafficSamples(ctx context.Context, samples []TrafficSample) (int, error) {
	if len(samples) == 0 {
		return 0, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	inserted := 0
	accountIDs := map[string]struct{}{}
	for _, sample := range samples {
		if strings.TrimSpace(sample.AccountID) == "" || strings.TrimSpace(sample.GroupName) == "" || strings.TrimSpace(sample.EvidenceKey) == "" {
			return 0, errors.New("流量样本缺少账号、分组或请求 ID")
		}
		if _, err := time.Parse(time.RFC3339Nano, sample.ObservedAt); err != nil {
			return 0, errors.New("流量样本时间无效")
		}
		payload, err := json.Marshal(sample.Payload)
		if err != nil {
			return 0, fmt.Errorf("流量样本无法严格 JSON 序列化：%w", err)
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO health_samples(
			account_id,group_name,result,latency_p50,latency_p95,latency_p99,sample_count,attempts,
			failure_reason,observed_at,source,evidence_key,payload_json
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(source,evidence_key,account_id,group_name) DO UPDATE SET
			latency_p50=excluded.latency_p50,latency_p95=excluded.latency_p95,latency_p99=excluded.latency_p99,
			payload_json=excluded.payload_json
		WHERE health_samples.latency_p95 IS NULL AND excluded.latency_p95 IS NOT NULL`, sample.AccountID, sample.GroupName, sample.Result,
			sample.LatencyP50, sample.LatencyP95, sample.LatencyP99, sample.SampleCount, sample.Attempts,
			sample.FailureReason, sample.ObservedAt, "traffic", sample.EvidenceKey, string(payload))
		if err != nil {
			return 0, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		inserted += int(count)
		accountIDs[sample.AccountID] = struct{}{}
	}
	if err := pruneHealthSamples(ctx, tx, accountIDs); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return inserted, nil
}

const retainedHealthSamplesPerAccount = 200

func pruneHealthSamples(ctx context.Context, tx *sql.Tx, accountIDs map[string]struct{}) error {
	for accountID := range accountIDs {
		if _, err := tx.ExecContext(ctx, `WITH deduplicated AS (
			SELECT id,observed_at,
				ROW_NUMBER() OVER(PARTITION BY LOWER(REPLACE(source,'_','-')),
					COALESCE(NULLIF(evidence_key,''),'row:'||id)
					ORDER BY COALESCE(observed_at,'') DESC,id DESC) duplicate_rank
			FROM health_samples WHERE account_id=?
		), retained AS (
			SELECT id FROM deduplicated WHERE duplicate_rank=1
			ORDER BY COALESCE(observed_at,'') DESC,id DESC LIMIT ?
		)
		DELETE FROM health_samples WHERE account_id=? AND id NOT IN (SELECT id FROM retained)`,
			accountID, retainedHealthSamplesPerAccount, accountID); err != nil {
			return err
		}
	}
	return nil
}

func parsedEvidenceTime(value sql.NullString) *time.Time {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value.String)
	if err != nil {
		return nil
	}
	parsed = parsed.UTC()
	return &parsed
}
