package business

import (
	"context"
	"database/sql"
	"strconv"
)

// Catalog options omit runtime statistics and evidence histories.
type GroupOption struct {
	ID             *string  `json:"id"`
	Name           string   `json:"name"`
	Platform       *string  `json:"platform"`
	Platforms      []string `json:"platforms"`
	RateMultiplier *string  `json:"rate_multiplier"`
}

type AccountOption struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Groups       []string `json:"groups"`
	Platform     *string  `json:"platform"`
	AccountType  *string  `json:"account_type"`
	UpstreamType *string  `json:"upstream_type"`
}

type UpstreamOption struct {
	UpstreamID   string `json:"upstream_id"`
	Host         string `json:"host"`
	Name         string `json:"name"`
	UpstreamType string `json:"upstream_type"`
	AuthStatus   string `json:"auth_status"`
	AccountCount int64  `json:"account_count"`
}

type GroupConfiguration struct {
	ID                  *string              `json:"id"`
	Name                string               `json:"name"`
	StrategySource      string               `json:"strategy_source"`
	ParticipationStatus string               `json:"participation_status"`
	Override            *GroupPolicyOverride `json:"override"`
}

func (s *Store) GroupConfigurations(ctx context.Context) ([]GroupConfiguration, error) {
	control, err := s.readPolicyDocument(ctx, s.db, "control-plane")
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT remote_id,name,strategy,strategy_source FROM local_groups ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []GroupConfiguration{}
	for rows.Next() {
		var item GroupConfiguration
		var id, strategy, source sql.NullString
		if err := rows.Scan(&id, &item.Name, &strategy, &source); err != nil {
			return nil, err
		}
		item.ID = nullString(id)
		_, item.StrategySource = groupStrategy(control, item.ID, nullString(strategy), nullString(source))
		item.ParticipationStatus, _ = groupParticipation(control, item.ID)
		item.Override = groupOverride(control, item.ID)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) GroupOptions(ctx context.Context) ([]GroupOption, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT remote_id,name,platform,rate_multiplier FROM local_groups ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []GroupOption{}
	for rows.Next() {
		var item GroupOption
		var id, platform, rate sql.NullString
		if err := rows.Scan(&id, &item.Name, &platform, &rate); err != nil {
			return nil, err
		}
		item.ID, item.Platform, item.RateMultiplier = nullString(id), nullString(platform), nullString(rate)
		item.Platforms = []string{}
		if item.Platform != nil {
			item.Platforms = append(item.Platforms, *item.Platform)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) AccountOptions(ctx context.Context) ([]AccountOption, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT a.id,a.name,ag.group_name,a.metadata_json,COALESCE(u.upstream_type,a.upstream_type) FROM accounts a
 LEFT JOIN account_groups ag ON ag.account_id=a.id
 LEFT JOIN (SELECT b.local_account_id,MIN(COALESCE(h.host,b.upstream_host)) AS host FROM bindings b
 JOIN binding_identities bi ON bi.binding_id=b.id
 LEFT JOIN upstream_identity_hosts h ON h.upstream_id=bi.upstream_id AND h.is_primary=1
 WHERE TRIM(b.upstream_host)<>'' GROUP BY b.local_account_id) ownership ON ownership.local_account_id=a.id
 LEFT JOIN upstreams u ON u.host=COALESCE(ownership.host,a.upstream_host)
 ORDER BY a.id,ag.group_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []AccountOption{}
	for rows.Next() {
		var id, name, raw string
		var group, upstreamType sql.NullString
		if err := rows.Scan(&id, &name, &group, &raw, &upstreamType); err != nil {
			return nil, err
		}
		if len(result) == 0 || result[len(result)-1].ID != id {
			metadata, _ := decodeObject(raw)
			item := AccountOption{ID: id, Name: name, Groups: []string{}, UpstreamType: nullString(upstreamType), Platform: accountMetadataValueText(metadata, "platform"), AccountType: accountMetadataValueText(metadata, "account_type", "type")}
			if item.AccountType == nil {
				item.AccountType = item.UpstreamType
			}
			result = append(result, item)
		}
		if group.Valid {
			result[len(result)-1].Groups = append(result[len(result)-1].Groups, group.String)
		}
	}
	return result, rows.Err()
}

func (s *Store) UpstreamOptions(ctx context.Context) ([]UpstreamOption, error) {
	names, err := s.derivedUpstreamNames(ctx, nil)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT h.upstream_id,u.host,u.base_url,u.upstream_type,u.auth_status,u.raw_balance,u.checked_at,u.metadata_json,u.balance,(SELECT COUNT(DISTINCT b.local_account_id) FROM binding_identities bi JOIN bindings b ON b.id=bi.binding_id WHERE bi.upstream_id=h.upstream_id) FROM upstreams u JOIN upstream_identity_hosts h ON h.host=u.host WHERE h.is_primary=1 ORDER BY u.host`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []UpstreamOption{}
	for rows.Next() {
		var item UpstreamOption
		var baseURL, raw string
		var balance, checked sql.NullString
		var legacyBalance sql.NullFloat64
		if err := rows.Scan(&item.UpstreamID, &item.Host, &baseURL, &item.UpstreamType, &item.AuthStatus, &balance, &checked, &raw, &legacyBalance, &item.AccountCount); err != nil {
			return nil, err
		}
		metadata, err := decodeObject(raw)
		item.Name = upstreamDisplayName(metadata, baseURL, names[item.Host])
		if err != nil {
			item.AuthStatus = UpstreamAuthStatusConfigurationError
		} else {
			rawBalance := nullString(balance)
			if rawBalance == nil && legacyBalance.Valid {
				rawBalance = normalizeDecimal(strconv.FormatFloat(legacyBalance.Float64, 'f', -1, 64))
			}
			item.AuthStatus = effectiveAuthStatus(item.AuthStatus, rawBalance, nullString(checked), metadata)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// GroupMembershipOptions preserves managed membership counts without health projections.
type GroupMembershipOption struct {
	ID           *string `json:"id"`
	Name         string  `json:"name"`
	AccountCount int64   `json:"account_count"`
}

func (s *Store) GroupMembershipOptions(ctx context.Context) ([]GroupMembershipOption, error) {
	control, err := s.readPolicyDocument(ctx, s.db, "control-plane")
	if err != nil {
		return nil, err
	}
	groups, err := s.GroupOptions(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT ag.group_name,a.id,a.metadata_json,COALESCE(u.upstream_type,a.upstream_type)
 FROM account_groups ag JOIN accounts a ON a.id=ag.account_id
 LEFT JOIN (SELECT b.local_account_id, MIN(COALESCE(h.host,b.upstream_host)) AS host
 FROM bindings b JOIN binding_identities bi ON bi.binding_id=b.id
 LEFT JOIN upstream_identity_hosts h ON h.upstream_id=bi.upstream_id AND h.is_primary=1
 WHERE TRIM(b.upstream_host)<>'' GROUP BY b.local_account_id) ownership ON ownership.local_account_id=a.id
 LEFT JOIN upstreams u ON u.host=COALESCE(ownership.host,a.upstream_host) WHERE a.id NOT IN (`+excludedAccountIDsSQL+`)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int64{}
	for rows.Next() {
		var name string
		var account accountProjection
		var upstreamType sql.NullString
		if err := rows.Scan(&name, &account.ID, &account.metadataRaw, &upstreamType); err != nil {
			return nil, err
		}
		account.UpstreamType = nullString(upstreamType)
		if groupAccountMetadataManaged(account, control) {
			counts[name]++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]GroupMembershipOption, 0, len(groups))
	for _, group := range groups {
		result = append(result, GroupMembershipOption{ID: group.ID, Name: group.Name, AccountCount: counts[group.Name]})
	}
	return result, nil
}
