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

type ProbeCandidate struct {
	AccountID    string
	GroupName    string
	GroupID      *string
	UpstreamType *string
	KnownModels  []string
	Metadata     map[string]any
	MetadataErr  error
}

type ProbeSample struct {
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
	StatusCode    *int
	RequestModel  string
	ActualModel   string
}

func (s *Store) ControlPolicy(ctx context.Context) (map[string]any, error) {
	document, err := s.readPolicyDocument(ctx, s.db, "control-plane")
	if err != nil {
		return nil, err
	}
	if document == nil {
		return nil, errors.New("控制面策略不存在")
	}
	return document, nil
}

func (s *Store) ProbeCandidates(ctx context.Context, accountID, groupName *string) ([]ProbeCandidate, error) {
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
	clauses = append(clauses, "NOT EXISTS (SELECT 1 FROM manual_priority_accounts m WHERE m.account_id=a.id)")
	query := `SELECT a.id,ag.group_name,ag.group_id,a.upstream_type,a.metadata_json
		FROM accounts a JOIN account_groups ag ON ag.account_id=a.id`
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY CAST(a.id AS INTEGER),a.id,ag.group_name"
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []ProbeCandidate{}
	for rows.Next() {
		var item ProbeCandidate
		var groupID, upstreamType sql.NullString
		var rawMetadata string
		if err := rows.Scan(&item.AccountID, &item.GroupName, &groupID, &upstreamType, &rawMetadata); err != nil {
			return nil, err
		}
		item.GroupID = nullString(groupID)
		item.UpstreamType = nullString(upstreamType)
		item.Metadata, item.MetadataErr = decodeJSONObject(rawMetadata)
		if item.MetadataErr == nil {
			item.KnownModels = metadataStringList(item.Metadata["known_models"])
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) PersistProbeSamples(ctx context.Context, samples []ProbeSample) (int, error) {
	if len(samples) == 0 {
		return 0, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	accountIDs := map[string]struct{}{}
	for _, sample := range samples {
		if strings.TrimSpace(sample.AccountID) == "" || strings.TrimSpace(sample.GroupName) == "" {
			return 0, errors.New("探测样本缺少账号或分组")
		}
		if _, err := time.Parse(time.RFC3339Nano, sample.ObservedAt); err != nil {
			return 0, errors.New("探测样本时间无效")
		}
		payload, err := json.Marshal(map[string]any{
			"status_code": sample.StatusCode, "request_model": sample.RequestModel, "actual_model": sample.ActualModel,
			"latency_metric": "first_token", "latency_source": "account_test.first_content", "latency_unit": "ms",
		})
		if err != nil {
			return 0, fmt.Errorf("探测样本无法序列化：%w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO health_samples(
			account_id,group_name,result,latency_p50,latency_p95,latency_p99,sample_count,attempts,
			failure_reason,observed_at,source,evidence_key,payload_json
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(source,evidence_key,account_id,group_name) DO UPDATE SET
			result=excluded.result,latency_p50=excluded.latency_p50,latency_p95=excluded.latency_p95,
			latency_p99=excluded.latency_p99,sample_count=excluded.sample_count,attempts=excluded.attempts,
			failure_reason=excluded.failure_reason,payload_json=excluded.payload_json`,
			sample.AccountID, sample.GroupName, sample.Result, sample.LatencyP50, sample.LatencyP95,
			sample.LatencyP99, sample.SampleCount, sample.Attempts, sample.FailureReason, sample.ObservedAt,
			"active-probe", sample.ObservedAt, string(payload)); err != nil {
			return 0, err
		}
		if sample.RequestModel != "" && sample.ActualModel != "" && sample.RequestModel != sample.ActualModel {
			if err := insertRuntimeEventWithStatus(ctx, tx, "probe_model_rewritten", "warning",
				fmt.Sprintf("账号 %s 指定探测模型 %q，但 Sub2API 实际使用了 %q", sample.AccountID, sample.RequestModel, sample.ActualModel),
				map[string]any{"account_id": sample.AccountID, "group_name": sample.GroupName, "requested_model": sample.RequestModel, "actual_model": sample.ActualModel},
				sample.ObservedAt); err != nil {
				return 0, err
			}
		}
		if sample.Result != "通过" {
			reason := pointerValue(sample.FailureReason)
			if strings.TrimSpace(reason) == "" {
				reason = sample.Result
			}
			if err := insertRuntimeEventWithStatus(ctx, tx, "probe.failed", "warning",
				fmt.Sprintf("账号 %s 主动探测失败：%s", sample.AccountID, reason),
				map[string]any{
					"account_id": sample.AccountID, "group_name": sample.GroupName,
					"result": sample.Result, "reason": reason, "status_code": sample.StatusCode,
					"request_model": sample.RequestModel, "actual_model": sample.ActualModel,
				}, sample.ObservedAt); err != nil {
				return 0, err
			}
		}
		accountIDs[sample.AccountID] = struct{}{}
	}
	if err := pruneHealthSamples(ctx, tx, accountIDs); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(samples), nil
}

func metadataStringList(raw any) []string {
	values, ok := raw.([]any)
	if !ok {
		return nil
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, rawValue := range values {
		value, ok := rawValue.(string)
		value = strings.TrimSpace(value)
		if !ok || value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}
