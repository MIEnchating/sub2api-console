package business

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// Cost traffic is an observation against current synchronized prices, not a
// reconstruction of historical billing. Only actual traffic in its own group
// contributes; active probes and another membership cannot supply evidence.
func (s *Store) costTrafficAlertFindings(ctx context.Context) ([]alertFinding, map[string]string, error) {
	now := time.Now().UTC()
	ignoredAccounts, err := s.costWallIgnoredAccounts(ctx, s.db)
	if err != nil {
		return nil, nil, err
	}
	rows, err := s.db.QueryContext(ctx, `WITH traffic AS (
 SELECT account_id,json_extract(payload_json,'$.request_group_id') AS group_id,COUNT(DISTINCT request_id) AS requests,MAX(observed_at) AS latest
 FROM usage_records WHERE LOWER(source)='traffic' AND observed_at>=? AND observed_at<=? AND TRIM(request_id)<>''
 GROUP BY account_id,json_extract(payload_json,'$.request_group_id')
 ) SELECT a.id,ag.group_name,a.multiplier,lg.rate_multiplier,COALESCE(t.requests,0),t.latest
 FROM accounts a JOIN account_groups ag ON ag.account_id=a.id
 LEFT JOIN local_groups lg ON lg.remote_id=ag.group_id AND lg.name=ag.group_name AND TRIM(ag.group_id)<>''
 LEFT JOIN traffic t ON t.account_id=a.id AND t.group_id=ag.group_id
 ORDER BY a.id,ag.group_name`, now.Add(-5*time.Minute).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	findings := []alertFinding{}
	notEvaluated := map[string]string{}
	known := map[string]bool{}
	for rows.Next() {
		var id, group string
		var rateText, groupRateText, latest sql.NullString
		var count int
		if err := rows.Scan(&id, &group, &rateText, &groupRateText, &count, &latest); err != nil {
			return nil, nil, err
		}
		key := "console:cost-traffic:" + id + ":" + group
		known[key] = true
		if containsControlID(ignoredAccounts, id) {
			continue
		}
		rate, rateOK := new(big.Rat).SetString(strings.TrimSpace(rateText.String))
		groupRate, groupOK := new(big.Rat).SetString(strings.TrimSpace(groupRateText.String))
		if !rateOK || !groupOK || rate.Sign() < 0 || groupRate.Sign() < 0 {
			notEvaluated[key] = "倍率或分组绑定无法核对，未判定成本流量恢复"
			continue
		}
		if count == 0 || rate.Cmp(groupRate) < 0 {
			continue
		}
		observed, err := time.Parse(time.RFC3339Nano, latest.String)
		if err != nil {
			notEvaluated[key] = "调用时间无法核对，未判定成本流量恢复"
			continue
		}
		code, comparison := "COST_TRAFFIC_BREAK_EVEN", "="
		if rate.Cmp(groupRate) > 0 {
			code, comparison = "COST_TRAFFIC_LOSS", ">"
		}
		reason := fmt.Sprintf("当前账号倍率 %s %s 分组倍率 %s；最近 5 分钟已采集 %d 次实际请求；最近调用 %s（北京时间）；请核对账号成本及分组定价", rateText.String, comparison, groupRateText.String, count, observed.In(time.FixedZone("CST", 8*60*60)).Format("2006-01-02 15:04:05"))
		findings = append(findings, alertFinding{key, "account.cost_traffic", "account", id, alertCause(code, reason)})
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, nil, err
	}
	active, err := s.db.QueryContext(ctx, `SELECT incident_key FROM alert_incidents WHERE event_type='account.cost_traffic' AND status IN ('firing','suppressed')`)
	if err != nil {
		return nil, nil, err
	}
	defer active.Close()
	for active.Next() {
		var key string
		if err := active.Scan(&key); err != nil {
			return nil, nil, err
		}
		if !known[key] {
			notEvaluated[key] = "账号或分组绑定已变化，未判定成本流量恢复"
		}
	}
	return findings, notEvaluated, active.Err()
}

func (s *Store) costWallIgnoredAccounts(ctx context.Context, queryer policyQueryer) (map[string]struct{}, error) {
	control, err := s.readPolicyDocument(ctx, queryer, "control-plane")
	if err != nil {
		return nil, err
	}
	scope, _ := control["scope"].(map[string]any)
	return controlAccountIDs(scope["ignore_cost_wall_account_ids"]), nil
}

func closeAccountCostTrafficAlerts(ctx context.Context, tx *sql.Tx, accountID, now string) error {
	_, err := tx.ExecContext(ctx, `UPDATE alert_incidents SET status='closed',last_seen_at=?,delivery_status='账号已开启无视成本墙',last_error=NULL
		WHERE event_type='account.cost_traffic' AND object_kind='account' AND object_id=? AND status IN ('firing','suppressed','recovered')`, now, accountID)
	return err
}
