package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

type upstreamConcurrencyObservation struct {
	limit     *int64
	status    string
	checkedAt *string
}

func readUpstreamConcurrency(platform string, metadata map[string]any) upstreamConcurrencyObservation {
	result := upstreamConcurrencyObservation{status: "unknown"}
	if !strings.EqualFold(strings.TrimSpace(platform), "sub2api") {
		return result
	}
	result.checkedAt = optionalMetadataText(metadata, "concurrency_checked_at")
	if value, ok := metadata["concurrency_limit"].(json.Number); ok {
		parsed, err := strconv.ParseInt(value.String(), 10, 64)
		if err == nil && parsed >= 0 {
			result.limit = &parsed
			result.status = "known"
			if parsed == 0 {
				result.status = "unlimited"
			}
		}
	}
	if stringValue(metadata["concurrency_status"]) == "stale" && result.limit != nil {
		result.status = "stale"
	}
	return result
}

// Use the verified binding identity first, then an exact configured Host mapping.
// Names and mutable Base URLs never establish ownership of shared capacity.
const upstreamCapacityAccountQuery = `WITH bound AS (
	SELECT b.local_account_id,MIN(COALESCE(bi.upstream_id,bh.upstream_id)) AS upstream_id,
	COUNT(DISTINCT COALESCE(bi.upstream_id,bh.upstream_id)) AS identity_count
	FROM bindings b LEFT JOIN binding_identities bi ON bi.binding_id=b.id
	LEFT JOIN upstream_identity_hosts bh ON bh.host=b.upstream_host
	%s GROUP BY b.local_account_id
)
SELECT a.id,COALESCE(bound.upstream_id,h.upstream_id,''),COALESCE(bound.identity_count,0),
	a.concurrency,a.schedulable,a.target_concurrency,a.target_schedulable,m.priority,
	u.upstream_type,u.metadata_json,COALESCE(a.routing_state,'')
FROM accounts a
LEFT JOIN bound ON bound.local_account_id=a.id
LEFT JOIN upstream_identity_hosts h ON h.host=a.upstream_host
LEFT JOIN upstream_identity_hosts primary_host ON primary_host.upstream_id=COALESCE(bound.upstream_id,h.upstream_id) AND primary_host.is_primary=1
LEFT JOIN upstreams u ON u.host=primary_host.host
LEFT JOIN manual_priority_accounts m ON m.account_id=a.id
%s ORDER BY a.id`

type routingCapacityAccount struct {
	RoutingAccount
	targetConcurrency *int64
	targetSchedulable *bool
}

func (s *Store) routingCapacityInventory(ctx context.Context) (map[string]routingCapacityAccount, error) {
	return s.routingCapacityInventoryForAccount(ctx, nil)
}

func (s *Store) routingCapacityInventoryForAccount(ctx context.Context, accountID *string) (map[string]routingCapacityAccount, error) {
	return routingCapacityInventoryForAccount(ctx, s.db, accountID)
}

func routingCapacityInventoryForAccount(ctx context.Context, queryer policyQueryer, accountID *string) (map[string]routingCapacityAccount, error) {
	bindingScope, accountScope := "", ""
	arguments := []any{}
	if accountID != nil {
		bindingScope, accountScope = "WHERE b.local_account_id=?", "WHERE a.id=?"
		arguments = append(arguments, strings.TrimSpace(*accountID), strings.TrimSpace(*accountID))
	}
	rows, err := queryer.QueryContext(ctx, fmt.Sprintf(upstreamCapacityAccountQuery, bindingScope, accountScope), arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]routingCapacityAccount{}
	for rows.Next() {
		var item routingCapacityAccount
		var identities int
		var current, target, schedulable, targetSchedulable, manual sql.NullInt64
		var platform, raw sql.NullString
		if err := rows.Scan(&item.ID, &item.UpstreamID, &identities, &current, &schedulable,
			&target, &targetSchedulable, &manual, &platform, &raw, &item.EffectiveState); err != nil {
			return nil, err
		}
		if identities > 1 {
			return nil, fmt.Errorf("账号 %s 绑定了多个上游，无法核对共享并发；请先修正稳定 ID 绑定", item.ID)
		}
		metadata, err := decodeObject(raw.String)
		if raw.Valid && err != nil {
			return nil, fmt.Errorf("账号 %s 的上游配置不可读，无法核对共享并发", item.ID)
		}
		observation := readUpstreamConcurrency(platform.String, metadata)
		item.UpstreamConcurrencyLimit = observation.limit
		item.UpstreamConcurrencyStatus = observation.status
		item.UpstreamType = nullString(platform)
		item.Concurrency, item.Schedulable = nullInt(current), strictNullBool(schedulable)
		item.ManualPriority = nullInt(manual)
		if !manual.Valid {
			item.targetConcurrency, item.targetSchedulable = nullInt(target), strictNullBool(targetSchedulable)
		}
		result[item.ID] = item
	}
	return result, rows.Err()
}

// RoutingCapacityAccounts includes accounts without a group so a scoped round
// cannot accidentally spend their configured capacity.
func (s *Store) RoutingCapacityAccounts(ctx context.Context) ([]RoutingAccount, error) {
	inventory, err := s.routingCapacityInventory(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]RoutingAccount, 0, len(inventory))
	for _, item := range inventory {
		result = append(result, item.RoutingAccount)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

type upstreamConcurrencyTotals struct {
	allocated     int64
	target        int64
	unknown       bool
	targetUnknown bool
	hasTarget     bool
}

func addConfiguredConcurrency(total *int64, unknown *bool, concurrency *int64, schedulable *bool) {
	if schedulable != nil && !*schedulable {
		return
	}
	if schedulable == nil || concurrency == nil || *concurrency <= 0 || *concurrency > math.MaxInt64-*total {
		*unknown = true
		return
	}
	*total += *concurrency
}

func (s *Store) upstreamConcurrencyTotals(ctx context.Context) (map[string]upstreamConcurrencyTotals, error) {
	inventory, err := s.routingCapacityInventory(ctx)
	if err != nil {
		return nil, err
	}
	result := map[string]upstreamConcurrencyTotals{}
	for _, account := range inventory {
		if account.UpstreamID == "" {
			continue
		}
		total := result[account.UpstreamID]
		addConfiguredConcurrency(&total.allocated, &total.unknown, account.Concurrency, account.Schedulable)
		concurrency, schedulable := account.Concurrency, account.Schedulable
		if account.targetConcurrency != nil {
			concurrency, total.hasTarget = account.targetConcurrency, true
		}
		if account.targetSchedulable != nil {
			schedulable, total.hasTarget = account.targetSchedulable, true
		}
		addConfiguredConcurrency(&total.target, &total.targetUnknown, concurrency, schedulable)
		result[account.UpstreamID] = total
	}
	return result, nil
}
