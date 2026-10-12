package business

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	jsonpatch "github.com/evanphx/json-patch/v5"
)

func trafficObject(raw []byte) (map[string]json.RawMessage, error) {
	if !json.Valid(raw) {
		return nil, errors.New("流量证据 JSON 已损坏")
	}
	fields := map[string]json.RawMessage{}
	if bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) {
		if err := json.Unmarshal(raw, &fields); err != nil {
			return nil, err
		}
	}
	return fields, nil
}

func trafficJSONPresent(raw json.RawMessage) bool {
	return len(raw) > 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

// The caller holds the account evidence lock. Only the matching request is
// transferred to Go; retained history is still selected and pruned in SQL.
func persistServerTrafficSample(ctx context.Context, tx *dbconn.Tx, sample TrafficSample, payload []byte) (int, error) {
	var id int64
	var priorP50, priorP95, priorP99 sql.NullString
	var priorRaw string
	err := tx.QueryRowContext(ctx, `SELECT id,latency_p50,latency_p95,latency_p99,payload_json FROM health_samples WHERE source='traffic' AND evidence_key=? AND account_id=? AND group_name=? FOR UPDATE`, sample.EvidenceKey, sample.AccountID, sample.GroupName).Scan(&id, &priorP50, &priorP95, &priorP99, &priorRaw)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.ExecContext(ctx, `INSERT INTO health_samples(account_id,group_name,result,latency_p50,latency_p95,latency_p99,sample_count,attempts,failure_reason,observed_at,source,evidence_key,payload_json) VALUES(?,?,?,?,?,?,?,?,?,?,'traffic',?,?)`, sample.AccountID, sample.GroupName, sample.Result, sample.LatencyP50, sample.LatencyP95, sample.LatencyP99, sample.SampleCount, sample.Attempts, sample.FailureReason, sample.ObservedAt, sample.EvidenceKey, string(payload))
		return 1, err
	}
	if err != nil {
		return 0, err
	}
	prior, err := trafficObject([]byte(priorRaw))
	if err != nil {
		return 0, err
	}
	incoming, err := trafficObject(payload)
	if err != nil {
		return 0, err
	}
	oldDuration := bytes.Equal(prior["latency_metric"], []byte(`"request_duration"`))
	newDuration := bytes.Equal(incoming["latency_metric"], []byte(`"request_duration"`))
	usageObject := bytes.HasPrefix(bytes.TrimSpace(incoming["token_usage"]), []byte("{"))
	latencyEnrichment := sample.LatencyP95 != nil && (!priorP95.Valid || (!oldDuration && newDuration) || (!trafficJSONPresent(prior["first_token_ms"]) && trafficJSONPresent(incoming["first_token_ms"])))
	usageEnrichment := usageObject && (!trafficJSONPresent(prior["token_usage"]) || !jsonpatch.Equal(prior["token_usage"], incoming["token_usage"]))
	if !latencyEnrichment && !usageEnrichment {
		return 0, nil
	}
	base, patch := []byte(priorRaw), payload
	if oldDuration && !newDuration {
		base, patch = payload, []byte(priorRaw)
	}
	merged, err := jsonpatch.MergePatch(base, patch)
	if err != nil {
		return 0, err
	}
	if usageObject {
		usagePatch, err := json.Marshal(map[string]json.RawMessage{"token_usage": incoming["token_usage"]})
		if err != nil {
			return 0, err
		}
		merged, err = jsonpatch.MergePatch(merged, usagePatch)
		if err != nil {
			return 0, err
		}
	}
	chooseLatency := func(incoming *string, prior sql.NullString) any {
		if incoming == nil || (oldDuration && !newDuration) {
			return prior
		}
		return *incoming
	}
	_, err = tx.ExecContext(ctx, `UPDATE health_samples SET latency_p50=?,latency_p95=?,latency_p99=?,payload_json=? WHERE id=?`, chooseLatency(sample.LatencyP50, priorP50), chooseLatency(sample.LatencyP95, priorP95), chooseLatency(sample.LatencyP99, priorP99), string(merged), id)
	return 1, err
}

func persistServerTrafficUsage(ctx context.Context, tx *dbconn.Tx, sample TrafficSample, payload []byte, isError int, firstToken *string) error {
	var id int64
	var priorRaw string
	var priorFirst sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT id,payload_json,first_token_ms FROM usage_records WHERE request_id=? AND account_id=? AND group_name=? AND observed_at=? FOR UPDATE`, sample.EvidenceKey, sample.AccountID, sample.GroupName, sample.ObservedAt).Scan(&id, &priorRaw, &priorFirst)
	if errors.Is(err, sql.ErrNoRows) {
		observed, parseErr := time.Parse(time.RFC3339Nano, sample.ObservedAt)
		if parseErr != nil {
			return parseErr
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO usage_records(request_id,account_id,account_name,group_name,is_error,error_reason,first_token_ms,observed_at,source,payload_json,observed_key) VALUES(?,?,(SELECT name FROM accounts WHERE id=?),?,?,?,?,?,'traffic',?,?)`, sample.EvidenceKey, sample.AccountID, sample.AccountID, sample.GroupName, isError, sample.FailureReason, firstToken, sample.ObservedAt, string(payload), routingRecoveryTimeKey(observed))
		return err
	}
	if err != nil {
		return err
	}
	merged, err := jsonpatch.MergePatch([]byte(priorRaw), payload)
	if err != nil {
		return err
	}
	var latency any = priorFirst
	if firstToken != nil {
		latency = *firstToken
	}
	_, err = tx.ExecContext(ctx, `UPDATE usage_records SET account_name=(SELECT name FROM accounts WHERE id=?),is_error=?,error_reason=?,first_token_ms=?,payload_json=? WHERE id=?`, sample.AccountID, isError, sample.FailureReason, latency, string(merged), id)
	return err
}
