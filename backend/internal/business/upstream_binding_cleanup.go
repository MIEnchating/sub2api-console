package business

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
)

var ErrBindingCleanupConflict = errors.New("绑定或账号状态已变化，请刷新列表后重新确认")
var ErrBindingCleanupInvalid = errors.New("清理失效绑定缺少有效的稳定 ID，请刷新列表")

type UpstreamBindingCleanup struct {
	UpstreamID      string `json:"upstream_id" binding:"required,max=128"`
	AccountID       string `json:"account_id" binding:"required,max=128"`
	UpstreamKeyID   string `json:"upstream_key_id" binding:"required,max=128"`
	UpstreamGroupID string `json:"upstream_group_id" binding:"required,max=128"`
}

// CleanupUpstreamBinding removes exactly one orphaned binding, never remote
// accounts, keys, credentials or any other binding for the same account.
func (s *Store) CleanupUpstreamBinding(ctx context.Context, host string, bindingID int64, expected UpstreamBindingCleanup, actor string) error {
	host = canonicalHost(host)
	if host == "" || bindingID <= 0 || !positiveNumericID(expected.AccountID) || strings.TrimSpace(expected.UpstreamID) == "" || strings.TrimSpace(expected.UpstreamKeyID) == "" || strings.TrimSpace(expected.UpstreamGroupID) == "" {
		return ErrBindingCleanupInvalid
	}
	ctx, release, err := mutationguard.Acquire(ctx, s, mutationguard.Account(expected.AccountID), mutationguard.Upstream(host))
	if err != nil {
		return err
	}
	defer release()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var actual UpstreamBindingCleanup
	var keyID, groupID, status string
	var accountExists, hostMatches bool
	err = tx.QueryRowContext(ctx, `SELECT b.local_account_id,bi.upstream_id,b.upstream_key_id,COALESCE(b.upstream_group_id,''),bi.upstream_key_id,COALESCE(bi.upstream_group_id,''),COALESCE(b.status,''),
 EXISTS(SELECT 1 FROM accounts WHERE id=b.local_account_id),
 EXISTS(SELECT 1 FROM upstream_identity_hosts WHERE host=? AND upstream_id=bi.upstream_id)
 FROM bindings b JOIN binding_identities bi ON bi.binding_id=b.id WHERE b.id=?`, host, bindingID).Scan(&actual.AccountID, &actual.UpstreamID, &actual.UpstreamKeyID, &actual.UpstreamGroupID, &keyID, &groupID, &status, &accountExists, &hostMatches)
	if err != nil {
		return err
	}
	if actual != expected || !hostMatches || keyID != actual.UpstreamKeyID || groupID != actual.UpstreamGroupID || (accountExists && status != "missing") {
		return ErrBindingCleanupConflict
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM bindings WHERE id=?`, bindingID); err != nil {
		return err
	}
	if err := insertAccountOperation(ctx, tx, AccountOperation{
		OperationID:   fmt.Sprintf("binding-cleanup/%d/%d", bindingID, time.Now().UnixNano()),
		OperationType: "upstream.binding_cleanup", State: "succeeded", Phase: "local_cleanup", Actor: actor,
		ObjectID: actual.AccountID, GroupNames: []string{},
		Before: map[string]any{"binding_id": bindingID, "binding": actual},
		After:  map[string]any{"binding_id": bindingID, "removed": true},
	}); err != nil {
		return err
	}
	return tx.Commit()
}
