package business

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"time"
)

// BalanceRecoveryState stores intent separately from management snapshots.
// No credential or upstream error body is persisted here.
type BalanceRecoveryState struct {
	AccountID          string
	BindingFingerprint string
	TargetFingerprint  string
	RemoteFingerprint  string
	Schedulable        bool
	Depleted           bool
	Started            bool
}

type BalanceRecoveryAccount struct {
	AccountID          string
	BindingFingerprint string
	BalanceKnown       bool
	Depleted           bool
	RoutingState       string
	State              *BalanceRecoveryState
}

func (s *Store) BalanceRecoveryAccounts(ctx context.Context, host string) ([]BalanceRecoveryAccount, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT a.id,b.id,bi.upstream_id,bi.upstream_key_id,COALESCE(bi.upstream_group_id,''),
 COALESCE(json_extract(a.metadata_json,'$.base_url'),''),COALESCE(a.routing_state,''),u.base_url,u.raw_balance,u.metadata_json,r.state_json
 FROM accounts a JOIN bindings b ON b.local_account_id=a.id
 JOIN binding_identities bi ON bi.binding_id=b.id
 JOIN upstream_identity_hosts h ON h.host=? AND h.upstream_id=bi.upstream_id
 JOIN upstream_identity_hosts bh ON bh.host=b.upstream_host AND bh.upstream_id=bi.upstream_id
 JOIN upstreams u ON u.host=h.host AND u.enabled=1
 LEFT JOIN balance_recovery_states r ON r.account_id=a.id
 WHERE (SELECT COUNT(*) FROM bindings sibling WHERE sibling.local_account_id=a.id)=1
 AND b.upstream_key_id=bi.upstream_key_id AND COALESCE(b.upstream_group_id,'')=COALESCE(bi.upstream_group_id,'')
 AND TRIM(bi.upstream_key_id)<>'' ORDER BY a.id`, canonicalHost(host))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []BalanceRecoveryAccount{}
	for rows.Next() {
		var item BalanceRecoveryAccount
		var bindingID int64
		var upstreamID, keyID, groupID, baseURL, upstreamBaseURL, metadataRaw string
		var balance, saved sql.NullString
		if err := rows.Scan(&item.AccountID, &bindingID, &upstreamID, &keyID, &groupID, &baseURL, &item.RoutingState, &upstreamBaseURL, &balance, &metadataRaw, &saved); err != nil {
			return nil, err
		}
		metadata, err := decodeObject(metadataRaw)
		if err != nil {
			return nil, err
		}
		amount, valid := new(big.Rat).SetString(balance.String)
		checked, timeErr := time.Parse(time.RFC3339Nano, stringValue(metadata["balance_checked_at"]))
		item.BalanceKnown = balance.Valid && valid && timeErr == nil && time.Since(checked) >= 0 && time.Since(checked) < 5*time.Minute && stringValue(metadata["balance_error"]) == ""
		if item.BalanceKnown {
			item.Depleted = amount.Sign() <= 0 || metadata["balance_hard_closed"] == true
		}
		raw, err := json.Marshal([]any{bindingID, upstreamID, keyID, groupID, baseURL, upstreamBaseURL, metadata["concurrency_user_id"]})
		if err != nil {
			return nil, err
		}
		digest := sha256.Sum256(raw)
		item.BindingFingerprint = hex.EncodeToString(digest[:])
		if saved.Valid {
			var state BalanceRecoveryState
			if err := json.Unmarshal([]byte(saved.String), &state); err != nil {
				return nil, err
			}
			item.State = &state
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) SaveBalanceRecoveryState(ctx context.Context, state BalanceRecoveryState) error {
	if !positiveNumericID(state.AccountID) || !validRoutingTargetFingerprint(state.TargetFingerprint) || !validRoutingTargetFingerprint(state.BindingFingerprint) {
		return errors.New("余额恢复记录缺少稳定账号或绑定指纹")
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO balance_recovery_states(account_id,state_json) VALUES(?,?) ON CONFLICT(account_id) DO UPDATE SET state_json=excluded.state_json`, state.AccountID, string(raw))
	return err
}

func (s *Store) DeleteBalanceRecoveryState(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM balance_recovery_states WHERE account_id=?`, id)
	return err
}

func (s *Store) CommitBalanceRecovery(ctx context.Context, state BalanceRecoveryState, operation AccountOperation) error {
	return s.commitAccountMutation(ctx, state.AccountID, operation, func(tx *sql.Tx, now string) error {
		var raw string
		if err := tx.QueryRowContext(ctx, `SELECT metadata_json FROM accounts WHERE id=?`, state.AccountID).Scan(&raw); err != nil {
			return err
		}
		metadata, err := decodeObject(raw)
		if err != nil {
			return err
		}
		delete(metadata, "error_message")
		delete(metadata, "last_error")
		metadata["status"] = "active"
		encoded, err := json.Marshal(metadata)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE accounts SET schedulable=?,metadata_json=?,updated_at=? WHERE id=?`, state.Schedulable, string(encoded), now, state.AccountID); err != nil {
			return err
		}
		state.Depleted, state.Started = false, false
		encoded, err = json.Marshal(state)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE balance_recovery_states SET state_json=? WHERE account_id=?`, string(encoded), state.AccountID)
		return err
	})
}
