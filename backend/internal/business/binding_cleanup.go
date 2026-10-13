package business

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

const MissingBindingCleanupReason = "上游绑定已确认失效"

// ConfirmedMissingBindings only authorizes accounts with exactly one stable
// binding. An ambiguous or partially identified binding is never destructive evidence.
func (s *Store) ConfirmedMissingBindings(ctx context.Context, accountID *string) (map[string]string, error) {
	query := `SELECT b.local_account_id,b.id,bi.upstream_id,bi.upstream_key_id,COALESCE(bi.upstream_group_id,''),
 COALESCE(k.parent_entity_id,''),COALESCE(k.confirmed_missing_at,''),COALESCE(g.confirmed_missing_at,'')
 FROM bindings b JOIN binding_identities bi ON bi.binding_id=b.id
 JOIN upstream_identity_hosts h ON h.host=b.upstream_host AND h.upstream_id=bi.upstream_id
 JOIN upstreams u ON u.host=b.upstream_host
 LEFT JOIN upstream_catalog_entities k ON k.upstream_id=bi.upstream_id AND k.entity_kind='key' AND k.entity_id=bi.upstream_key_id
 LEFT JOIN upstream_catalog_entities g ON g.upstream_id=bi.upstream_id AND g.entity_kind='group'
 AND g.entity_id=COALESCE(NULLIF(TRIM(k.parent_entity_id),''),bi.upstream_group_id)
 WHERE b.upstream_key_id=bi.upstream_key_id AND COALESCE(b.upstream_group_id,'')=COALESCE(bi.upstream_group_id,'')
 AND u.enabled=1 AND u.auth_status IN ('已鉴权','已恢复','valid','authenticated','healthy','ok')
 AND json_extract(u.metadata_json,'$.catalog_status')='已同步'
 AND (SELECT COUNT(*) FROM bindings sibling WHERE sibling.local_account_id=b.local_account_id)=1
 AND ((k.lifecycle_state='missing' AND k.missing_observations>=2 AND k.confirmed_missing_at IS NOT NULL)
 OR (g.lifecycle_state='missing' AND g.missing_observations>=2 AND g.confirmed_missing_at IS NOT NULL))`
	args := []any{}
	if accountID != nil {
		query += ` AND b.local_account_id=?`
		args = append(args, *accountID)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]string{}
	for rows.Next() {
		var id string
		var bindingID int64
		var upstreamID, keyID, groupID, parent, keyMissing, groupMissing string
		if err := rows.Scan(&id, &bindingID, &upstreamID, &keyID, &groupID, &parent, &keyMissing, &groupMissing); err != nil {
			return nil, err
		}
		raw, err := json.Marshal([]any{bindingID, upstreamID, keyID, groupID, parent, keyMissing, groupMissing})
		if err != nil {
			return nil, err
		}
		digest := sha256.Sum256(raw)
		result[id] = hex.EncodeToString(digest[:])
	}
	return result, rows.Err()
}
