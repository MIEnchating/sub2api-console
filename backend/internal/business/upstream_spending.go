package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
)

// UpstreamSpending is the running balance-drop total, in the same recharge-
// adjusted units as Balance. It is not an upstream billing statement.
type UpstreamSpending struct {
	Total     string `json:"total"`
	StartedAt string `json:"started_at"`
	UpdatedAt string `json:"updated_at"`
}

type upstreamSpendingState struct {
	UpstreamSpending
	Baseline      string `json:"baseline"`
	ProfileUserID string `json:"profile_user_id"`
	QuotaPerUnit  string `json:"quota_per_unit"`
}

func readUpstreamSpending(metadata map[string]any) (*upstreamSpendingState, error) {
	value, exists := metadata["balance_spending"]
	if !exists {
		return nil, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var state upstreamSpendingState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, errors.New("上游累计消费状态不可读")
	}
	total, totalOK := new(big.Rat).SetString(state.Total)
	_, baselineOK := new(big.Rat).SetString(state.Baseline)
	if !totalOK || total.Sign() < 0 || !baselineOK || state.StartedAt == "" || state.UpdatedAt == "" {
		return nil, errors.New("上游累计消费状态无效")
	}
	return &state, nil
}

// Keep one shared observation across aliases, in the existing upstream state.
// All writes share the balance transaction, so retries cannot count a drop twice.
func applyUpstreamSpending(ctx context.Context, tx *sql.Tx, host string, metadata map[string]any, balance *UpstreamBalanceObservation, recharge, now string) error {
	if balance.RawBalance == nil || balance.Status != "已读取" {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT u.host,u.metadata_json FROM upstreams u
 JOIN upstream_identity_hosts h ON h.host=u.host
 WHERE h.upstream_id=(SELECT upstream_id FROM upstream_identity_hosts WHERE host=?)`, host)
	if err != nil {
		return err
	}
	aliases := map[string]map[string]any{}
	var state *upstreamSpendingState
	for rows.Next() {
		var alias, raw string
		if err := rows.Scan(&alias, &raw); err != nil {
			rows.Close()
			return err
		}
		data, err := decodeObject(raw)
		if err != nil {
			rows.Close()
			return err
		}
		aliases[alias] = data
		candidate, err := readUpstreamSpending(data)
		if err != nil {
			rows.Close()
			return err
		}
		if candidate != nil && (state == nil || candidate.UpdatedAt > state.UpdatedAt) {
			state = candidate
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	quota := ""
	if balance.QuotaPerUnit != nil {
		quota = strings.TrimSpace(*balance.QuotaPerUnit)
	}
	user := strings.TrimSpace(balance.ProfileUserID)
	if state == nil {
		state = &upstreamSpendingState{UpstreamSpending: UpstreamSpending{Total: "0", StartedAt: now}}
	} else if state.ProfileUserID == user && state.QuotaPerUnit == quota {
		before, _ := new(big.Rat).SetString(state.Baseline)
		after, _ := new(big.Rat).SetString(*balance.RawBalance)
		delta := new(big.Rat).Sub(before, after)
		if delta.Sign() > 0 {
			rate, _ := new(big.Rat).SetString(recharge)
			total, _ := new(big.Rat).SetString(state.Total)
			state.Total = decimalRatText(total.Add(total, delta.Quo(delta, rate)))
		}
	}
	state.Baseline, state.ProfileUserID, state.QuotaPerUnit = *balance.RawBalance, user, quota
	state.UpdatedAt = now
	metadata["balance_spending"] = state
	for alias, data := range aliases {
		if alias == host {
			continue
		}
		data["balance_spending"] = state
		encoded, err := json.Marshal(data)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE upstreams SET metadata_json=? WHERE host=?`, string(encoded), alias); err != nil {
			return err
		}
	}
	return nil
}
