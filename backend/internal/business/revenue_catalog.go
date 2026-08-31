package business

import (
	"context"
	"sort"
	"strings"
)

// RevenueCatalog is the Console-owned identity map used for billing
// reconciliation. Remote management and upstream APIs supply amounts only.
type RevenueCatalog struct {
	Accounts []RevenueAccount
}

type RevenueAccount struct {
	ID       string
	Name     string
	Groups   []string
	Bindings []RevenueBinding
}

type RevenueBinding struct {
	AuthHost        string
	UpstreamHost    string
	UpstreamType    string
	UpstreamKeyID   string
	UpstreamKeyName string
	RechargeRate    string
}

func (s *Store) RevenueCatalog(ctx context.Context) (RevenueCatalog, error) {
	result := RevenueCatalog{Accounts: []RevenueAccount{}}
	rows, err := s.db.QueryContext(ctx, `SELECT id,name FROM accounts
		ORDER BY CASE WHEN id GLOB '[0-9]*' THEN CAST(id AS INTEGER) ELSE 0 END,id`)
	if err != nil {
		return RevenueCatalog{}, err
	}
	byID := map[string]int{}
	for rows.Next() {
		var account RevenueAccount
		if err := rows.Scan(&account.ID, &account.Name); err != nil {
			rows.Close()
			return RevenueCatalog{}, err
		}
		if !positiveNumericID(account.ID) {
			continue
		}
		account.Groups, account.Bindings = []string{}, []RevenueBinding{}
		byID[account.ID] = len(result.Accounts)
		result.Accounts = append(result.Accounts, account)
	}
	if err := rows.Close(); err != nil {
		return RevenueCatalog{}, err
	}
	if err := rows.Err(); err != nil {
		return RevenueCatalog{}, err
	}

	groupRows, err := s.db.QueryContext(ctx, `SELECT account_id,group_name FROM account_groups ORDER BY account_id,group_name`)
	if err != nil {
		return RevenueCatalog{}, err
	}
	for groupRows.Next() {
		var accountID, groupName string
		if err := groupRows.Scan(&accountID, &groupName); err != nil {
			groupRows.Close()
			return RevenueCatalog{}, err
		}
		if index, found := byID[accountID]; found && strings.TrimSpace(groupName) != "" {
			result.Accounts[index].Groups = append(result.Accounts[index].Groups, strings.TrimSpace(groupName))
		}
	}
	if err := groupRows.Close(); err != nil {
		return RevenueCatalog{}, err
	}
	if err := groupRows.Err(); err != nil {
		return RevenueCatalog{}, err
	}

	bindingRows, err := s.db.QueryContext(ctx, `SELECT b.local_account_id,b.upstream_host,
		COALESCE(NULLIF(TRIM(b.source_auth_host),''),b.upstream_host),
		COALESCE(u.upstream_type,''),b.upstream_key_id,b.upstream_key_name,
		COALESCE(r.recharge_rate,r2.recharge_rate,'1')
		FROM bindings b
		LEFT JOIN upstreams u ON u.host=b.upstream_host
		LEFT JOIN recharge_rates r ON r.host=COALESCE(NULLIF(TRIM(b.source_auth_host),''),b.upstream_host)
		LEFT JOIN recharge_rates r2 ON r2.host=b.upstream_host
		WHERE COALESCE(b.status,'')<>'missing'
		ORDER BY b.local_account_id,b.upstream_host,b.upstream_key_id,b.id`)
	if err != nil {
		return RevenueCatalog{}, err
	}
	for bindingRows.Next() {
		var accountID string
		var binding RevenueBinding
		if err := bindingRows.Scan(&accountID, &binding.UpstreamHost, &binding.AuthHost,
			&binding.UpstreamType, &binding.UpstreamKeyID, &binding.UpstreamKeyName, &binding.RechargeRate); err != nil {
			bindingRows.Close()
			return RevenueCatalog{}, err
		}
		if index, found := byID[accountID]; found {
			result.Accounts[index].Bindings = append(result.Accounts[index].Bindings, binding)
		}
	}
	if err := bindingRows.Close(); err != nil {
		return RevenueCatalog{}, err
	}
	if err := bindingRows.Err(); err != nil {
		return RevenueCatalog{}, err
	}
	for index := range result.Accounts {
		result.Accounts[index].Groups = uniqueRevenueGroups(result.Accounts[index].Groups)
	}
	return result, nil
}

func uniqueRevenueGroups(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, found := seen[value]; found {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
