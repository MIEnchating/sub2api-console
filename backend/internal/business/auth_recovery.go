package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/redact"
)

type AuthRecoveryOutcome struct {
	Host            string  `json:"host"`
	TriggerStatus   *string `json:"trigger_status,omitempty"`
	Success         bool    `json:"success"`
	Attempted       bool    `json:"attempted"`
	Transient       bool    `json:"transient"`
	Code            *string `json:"code,omitempty"`
	InteractionKind *string `json:"interaction_kind,omitempty"`
	Reason          *string `json:"reason,omitempty"`
	RefreshAttempt  *string `json:"refresh_attempt,omitempty"`
	RefreshKind     *string `json:"refresh_kind,omitempty"`
}

type AuthRecoverySummary struct {
	Hosts     int `json:"hosts"`
	Recovered int `json:"recovered"`
	Failed    int `json:"failed"`
}

func (s *Store) PersistAuthRecoveryOutcomes(ctx context.Context, values []AuthRecoveryOutcome, _ string) (AuthRecoverySummary, error) {
	values, err := normalizeAuthRecoveryOutcomes(values)
	if err != nil {
		return AuthRecoverySummary{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	payload, err := json.Marshal(map[string]any{"results": values, "updated_at": now})
	if err != nil {
		return AuthRecoverySummary{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AuthRecoverySummary{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO operational_snapshots(namespace,state_key,value_json,observed_at,updated_at,origin)
		VALUES('sub2api','auth-recovery-runtime-snapshot',?,?,?,'console') ON CONFLICT(namespace,state_key) DO UPDATE SET
		value_json=excluded.value_json,observed_at=excluded.observed_at,updated_at=excluded.updated_at,origin=excluded.origin`, string(payload), now, now); err != nil {
		return AuthRecoverySummary{}, err
	}
	result := AuthRecoverySummary{Hosts: len(values)}
	for _, item := range values {
		status := UpstreamAuthStatusInvalid
		if item.Success {
			status = UpstreamAuthStatusRecovered
			result.Recovered++
		} else {
			result.Failed++
			if item.Transient {
				status = UpstreamAuthStatusRecoveryTemporarilyFailed
			}
		}
		var metadataRaw string
		resolvedHost := item.Host
		err := tx.QueryRowContext(ctx, `SELECT metadata_json FROM upstreams WHERE host=?`, resolvedHost).Scan(&metadataRaw)
		if errors.Is(err, sql.ErrNoRows) {
			alias := "www." + resolvedHost
			if strings.HasPrefix(resolvedHost, "www.") {
				alias = strings.TrimPrefix(resolvedHost, "www.")
			}
			err = tx.QueryRowContext(ctx, `SELECT host,metadata_json FROM upstreams WHERE host=?`, alias).Scan(&resolvedHost, &metadataRaw)
		}
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return AuthRecoverySummary{}, err
		}
		metadata, err := decodeObject(metadataRaw)
		if err != nil {
			return AuthRecoverySummary{}, errors.New("上游 metadata 记录损坏")
		}
		metadata["auth_recovery_code"] = pointerValue(item.Code)
		metadata["auth_recovery_reason"] = pointerValue(item.Reason)
		metadata["auth_recovery_at"] = now
		encoded, err := json.Marshal(metadata)
		if err != nil {
			return AuthRecoverySummary{}, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE upstreams SET auth_status=?,metadata_json=?,updated_at=? WHERE host=?`, status, string(encoded), now, resolvedHost); err != nil {
			return AuthRecoverySummary{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return AuthRecoverySummary{}, err
	}
	return result, nil
}

func normalizeAuthRecoveryOutcomes(values []AuthRecoveryOutcome) ([]AuthRecoveryOutcome, error) {
	result := make([]AuthRecoveryOutcome, len(values))
	for index, raw := range values {
		item := raw
		item.Host = canonicalHost(item.Host)
		if item.Host == "" {
			return nil, errors.New("鉴权恢复结果缺少 Host")
		}
		item.Code = normalizedAuthRecoveryText(item.Code)
		item.InteractionKind = normalizedAuthRecoveryText(item.InteractionKind)
		item.RefreshKind = normalizedAuthRecoveryText(item.RefreshKind)
		item.TriggerStatus = normalizedAuthRecoveryText(item.TriggerStatus)
		item.Reason = redactedOptionalText(item.Reason, 2000)
		item.RefreshAttempt = redactedOptionalText(item.RefreshAttempt, 200)
		result[index] = item
	}
	return result, nil
}

func normalizedAuthRecoveryText(value *string) *string {
	if value == nil {
		return nil
	}
	text := strings.TrimSpace(*value)
	if len(text) > 200 {
		text = text[:200]
	}
	return &text
}

func redactedOptionalText(value *string, maximum int) *string {
	if value == nil {
		return nil
	}
	text := redact.Secrets(strings.TrimSpace(*value))
	if len(text) > maximum {
		text = text[:maximum]
	}
	return &text
}
