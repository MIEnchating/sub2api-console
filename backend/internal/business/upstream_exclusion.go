package business

import (
	"context"
	"database/sql"
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"time"
)

var ErrUpstreamExcluded = errors.New("上游已排除管控，请先取消排除后重试")

type UpstreamExclusionPreview struct {
	UpstreamDeletePreview
	UpstreamID string `json:"upstream_id"`
	Excluded   bool   `json:"excluded"`
}

const excludedAccountIDsSQL = `SELECT a.id FROM accounts a WHERE
 EXISTS (SELECT 1 FROM upstream_identity_hosts h JOIN upstream_exclusions e ON e.upstream_id=h.upstream_id WHERE h.host=a.upstream_host) OR
 EXISTS (SELECT 1 FROM bindings b LEFT JOIN binding_identities bi ON bi.binding_id=b.id LEFT JOIN upstream_identity_hosts h ON h.host=b.upstream_host
 JOIN upstream_exclusions e ON e.upstream_id=bi.upstream_id OR e.upstream_id=h.upstream_id WHERE b.local_account_id=a.id)`

func (s *Store) UpstreamExclusionPreview(ctx context.Context, host string) (UpstreamExclusionPreview, error) {
	preview, err := s.UpstreamDeletePreview(ctx, host)
	if err != nil {
		return UpstreamExclusionPreview{}, err
	}
	id, _, err := upstreamIdentityHostsForQueryer(ctx, s.db, host)
	if err != nil {
		return UpstreamExclusionPreview{}, err
	}
	excluded, err := s.UpstreamExcluded(ctx, host)
	return UpstreamExclusionPreview{UpstreamDeletePreview: preview, UpstreamID: id, Excluded: excluded}, err
}

func (s *Store) UpstreamExcluded(ctx context.Context, host string) (bool, error) {
	var excluded bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM upstream_exclusions e JOIN upstream_identity_hosts h ON h.upstream_id=e.upstream_id WHERE h.host=?)`, canonicalHost(host)).Scan(&excluded)
	return excluded, err
}

func (s *Store) AccountUpstreamExcluded(ctx context.Context, id string) (bool, error) {
	var excluded bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM (`+excludedAccountIDsSQL+`) WHERE id=?)`, id).Scan(&excluded)
	return excluded, err
}

func CheckUpstreamManaged(ctx context.Context, repository any, host string) error {
	if store, ok := repository.(interface {
		UpstreamExcluded(context.Context, string) (bool, error)
	}); ok {
		excluded, err := store.UpstreamExcluded(ctx, host)
		if err != nil {
			return err
		}
		if excluded {
			return ErrUpstreamExcluded
		}
	}
	return nil
}

func CheckAccountUpstreamManaged(ctx context.Context, repository any, id string) error {
	if store, ok := repository.(interface {
		AccountUpstreamExcluded(context.Context, string) (bool, error)
	}); ok {
		excluded, err := store.AccountUpstreamExcluded(ctx, id)
		if err != nil {
			return err
		}
		if excluded {
			return ErrUpstreamExcluded
		}
	}
	return nil
}

func (s *Store) SetUpstreamExcluded(ctx context.Context, id string, excluded bool, actor string) error {
	guarded, release, err := mutationguard.Acquire(ctx, s, mutationguard.UpstreamCatalog())
	if err != nil {
		return err
	}
	defer release()
	ctx = guarded
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM upstream_identities WHERE upstream_id=?)`, id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return errors.New("稳定上游 ID 不存在，请刷新列表")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if excluded {
		_, err = tx.ExecContext(ctx, `INSERT INTO upstream_exclusions(upstream_id,updated_at) VALUES(?,?) ON CONFLICT(upstream_id) DO UPDATE SET updated_at=excluded.updated_at`, id, now)
	} else {
		_, err = tx.ExecContext(ctx, `DELETE FROM upstream_exclusions WHERE upstream_id=?`, id)
	}
	if err != nil {
		return err
	}
	if err = closeExcludedUpstreamAlerts(ctx, tx, now); err != nil {
		return err
	}
	if err = insertRuntimeEventWithStatus(ctx, tx, "upstream.exclusion.updated", "succeeded", "上游管控排除状态已更新", map[string]any{"upstream_id": id, "excluded": excluded, "actor": actor}, now); err != nil {
		return err
	}
	return tx.Commit()
}

func closeExcludedUpstreamAlerts(ctx context.Context, tx *sql.Tx, now string) error {
	_, err := tx.ExecContext(ctx, `UPDATE alert_incidents SET status='closed',last_seen_at=?,delivery_status='上游已排除管控',last_error=NULL WHERE status IN ('firing','suppressed','recovered') AND ((object_kind='host' AND object_id IN (SELECT h.host FROM upstream_identity_hosts h JOIN upstream_exclusions e ON e.upstream_id=h.upstream_id)) OR (object_kind='account' AND object_id IN (`+excludedAccountIDsSQL+`)))`, now)
	return err
}
