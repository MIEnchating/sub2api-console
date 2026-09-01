package business

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type PricingCatalog struct {
	Accounts []PricingAccount
	Groups   []PricingGroup
}

type PricingAccount struct {
	ID             string
	Name           string
	Platform       string
	Multiplier     *string
	GroupIDs       []string
	GroupsValid    bool
	ManualPriority bool
}

type PricingGroup struct {
	ID             string
	Name           string
	Platform       string
	RateMultiplier *string
}

type PricingSyncResult struct {
	Accounts   int   `json:"accounts"`
	GroupLinks int   `json:"group_links"`
	EventID    int64 `json:"event_id"`
}

func (s *Store) PricingCatalog(ctx context.Context) (PricingCatalog, error) {
	catalog := PricingCatalog{Accounts: []PricingAccount{}, Groups: []PricingGroup{}}
	groupRows, err := s.db.QueryContext(ctx, `SELECT name,remote_id,platform,rate_multiplier
		FROM local_groups ORDER BY CASE WHEN remote_id GLOB '[0-9]*' THEN CAST(remote_id AS INTEGER) ELSE 0 END,remote_id,name`)
	if err != nil {
		return PricingCatalog{}, err
	}
	for groupRows.Next() {
		var name string
		var id, platform, rate sql.NullString
		if err := groupRows.Scan(&name, &id, &platform, &rate); err != nil {
			groupRows.Close()
			return PricingCatalog{}, err
		}
		if !id.Valid || !positiveNumericID(id.String) {
			continue
		}
		catalog.Groups = append(catalog.Groups, PricingGroup{
			ID: id.String, Name: name, Platform: strings.ToLower(strings.TrimSpace(platform.String)), RateMultiplier: nullString(rate),
		})
	}
	if err := groupRows.Close(); err != nil {
		return PricingCatalog{}, err
	}
	if err := groupRows.Err(); err != nil {
		return PricingCatalog{}, err
	}

	accountRows, err := s.db.QueryContext(ctx, `SELECT a.id,a.name,a.multiplier,a.metadata_json,
		EXISTS(SELECT 1 FROM manual_priority_accounts m WHERE m.account_id=a.id) FROM accounts a
		ORDER BY CASE WHEN a.id GLOB '[0-9]*' THEN CAST(a.id AS INTEGER) ELSE 0 END,a.id`)
	if err != nil {
		return PricingCatalog{}, err
	}
	for accountRows.Next() {
		var item PricingAccount
		var multiplier sql.NullString
		var metadata string
		if err := accountRows.Scan(&item.ID, &item.Name, &multiplier, &metadata, &item.ManualPriority); err != nil {
			accountRows.Close()
			return PricingCatalog{}, err
		}
		if !positiveNumericID(item.ID) {
			continue
		}
		item.Multiplier = nullString(multiplier)
		if platform := accountMetadataText(metadata, "platform"); platform != nil {
			item.Platform = strings.ToLower(strings.TrimSpace(*platform))
		}
		item.GroupIDs = []string{}
		item.GroupsValid = true
		catalog.Accounts = append(catalog.Accounts, item)
	}
	if err := accountRows.Close(); err != nil {
		return PricingCatalog{}, err
	}
	if err := accountRows.Err(); err != nil {
		return PricingCatalog{}, err
	}
	byID := make(map[string]int, len(catalog.Accounts))
	for index := range catalog.Accounts {
		byID[catalog.Accounts[index].ID] = index
	}

	membershipRows, err := s.db.QueryContext(ctx, `SELECT ag.account_id,ag.group_id,lg.remote_id
		FROM account_groups ag LEFT JOIN local_groups lg ON lg.name=ag.group_name
		ORDER BY ag.account_id,ag.group_name`)
	if err != nil {
		return PricingCatalog{}, err
	}
	seen := map[string]map[string]struct{}{}
	for membershipRows.Next() {
		var accountID string
		var membershipID, catalogID sql.NullString
		if err := membershipRows.Scan(&accountID, &membershipID, &catalogID); err != nil {
			membershipRows.Close()
			return PricingCatalog{}, err
		}
		accountIndex, found := byID[accountID]
		if !found {
			continue
		}
		account := &catalog.Accounts[accountIndex]
		groupID := ""
		if membershipID.Valid {
			groupID = strings.TrimSpace(membershipID.String)
		} else if catalogID.Valid {
			groupID = strings.TrimSpace(catalogID.String)
		}
		if !positiveNumericID(groupID) {
			account.GroupsValid = false
			continue
		}
		if seen[accountID] == nil {
			seen[accountID] = map[string]struct{}{}
		}
		if _, duplicate := seen[accountID][groupID]; duplicate {
			continue
		}
		seen[accountID][groupID] = struct{}{}
		account.GroupIDs = append(account.GroupIDs, groupID)
	}
	if err := membershipRows.Close(); err != nil {
		return PricingCatalog{}, err
	}
	if err := membershipRows.Err(); err != nil {
		return PricingCatalog{}, err
	}
	for index := range catalog.Accounts {
		sort.Slice(catalog.Accounts[index].GroupIDs, func(left, right int) bool {
			return stableNumericLess(catalog.Accounts[index].GroupIDs[left], catalog.Accounts[index].GroupIDs[right])
		})
	}
	return catalog, nil
}

func (s *Store) SyncPricingAccountGroups(ctx context.Context, changes map[string][]string, actor string) (PricingSyncResult, error) {
	if len(changes) == 0 {
		return PricingSyncResult{}, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PricingSyncResult{}, err
	}
	defer tx.Rollback()
	groupNames, err := pricingGroupNames(ctx, tx)
	if err != nil {
		return PricingSyncResult{}, err
	}
	accountIDs := make([]string, 0, len(changes))
	for accountID := range changes {
		if !positiveNumericID(accountID) {
			return PricingSyncResult{}, errors.New("价格管理本地同步包含无效账号 ID")
		}
		accountIDs = append(accountIDs, accountID)
	}
	sort.Slice(accountIDs, func(left, right int) bool { return stableNumericLess(accountIDs[left], accountIDs[right]) })
	now := time.Now().UTC().Format(time.RFC3339Nano)
	groupLinks := 0
	for _, accountID := range accountIDs {
		var multiplier sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT multiplier FROM accounts WHERE id=?`, accountID).Scan(&multiplier); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return PricingSyncResult{}, fmt.Errorf("价格管理写回账号 %s 已不存在", accountID)
			}
			return PricingSyncResult{}, err
		}
		groupIDs := uniqueStableIDs(changes[accountID])
		if groupIDs == nil {
			return PricingSyncResult{}, fmt.Errorf("价格管理写回账号 %s 包含无效分组 ID", accountID)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM account_groups WHERE account_id=?`, accountID); err != nil {
			return PricingSyncResult{}, err
		}
		for _, groupID := range groupIDs {
			name, found := groupNames[groupID]
			if !found {
				return PricingSyncResult{}, fmt.Errorf("价格管理写回分组 %s 不在 Console 本地目录中", groupID)
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO account_groups(account_id,group_name,group_id,group_rate)
				VALUES(?,?,?,?)`, accountID, name, groupID, nullablePricingString(multiplier)); err != nil {
				return PricingSyncResult{}, err
			}
			groupLinks++
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE local_groups SET account_count=(
		SELECT COUNT(*) FROM account_groups WHERE group_name=local_groups.name
	),updated_at=?`, now); err != nil {
		return PricingSyncResult{}, err
	}
	payload := map[string]any{
		"actor": strings.TrimSpace(actor), "accounts": len(accountIDs), "group_links": groupLinks, "remote_write": true,
	}
	if err := insertRuntimeEventWithStatus(ctx, tx, "pricing.groups.synced", "succeeded",
		fmt.Sprintf("价格分组本地状态已同步：账号 %d，成员关系 %d", len(accountIDs), groupLinks), payload, now); err != nil {
		return PricingSyncResult{}, err
	}
	var eventID int64
	if err := tx.QueryRowContext(ctx, `SELECT MIN(source_id) FROM runtime_events WHERE source_id < 0`).Scan(&eventID); err != nil {
		return PricingSyncResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return PricingSyncResult{}, err
	}
	return PricingSyncResult{Accounts: len(accountIDs), GroupLinks: groupLinks, EventID: eventID}, nil
}

func pricingGroupNames(ctx context.Context, queryer policyQueryer) (map[string]string, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT remote_id,name FROM local_groups WHERE remote_id IS NOT NULL
		UNION ALL SELECT group_id,group_name FROM account_groups WHERE group_id IS NOT NULL ORDER BY 1,2`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]string{}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		id, name = strings.TrimSpace(id), strings.TrimSpace(name)
		if !positiveNumericID(id) || name == "" {
			continue
		}
		if previous, found := result[id]; found && previous != name {
			return nil, fmt.Errorf("Console 本地分组 ID %s 对应多个名称", id)
		}
		result[id] = name
	}
	return result, rows.Err()
}

func uniqueStableIDs(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if !positiveNumericID(value) {
			return nil
		}
		if _, found := seen[value]; found {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool { return stableNumericLess(result[left], result[right]) })
	return result
}

func stableNumericLess(left, right string) bool {
	if len(left) != len(right) {
		return len(left) < len(right)
	}
	return left < right
}

func nullablePricingString(value sql.NullString) any {
	if value.Valid {
		return value.String
	}
	return nil
}
