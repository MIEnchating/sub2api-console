package business

import "context"

// Manual controls are confirmed independently of routing calculations. Read the
// authoritative scope so alerts need not wait for the next inspection to finish.
func (s *Store) manualFuseAlertFindings(ctx context.Context, enabled bool) ([]alertFinding, map[string]struct{}, error) {
	policy, err := s.readPolicyDocument(ctx, s.db, "control-plane")
	if err != nil {
		return nil, nil, err
	}
	scope, _ := policy["scope"].(map[string]any)
	fused := controlAccountIDs(scope["manual_fused_account_ids"])
	findings := []alertFinding{}
	if !enabled || len(fused) == 0 {
		return findings, fused, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT a.id,COALESCE(rd.group_name,MIN(ag.group_name),'')
		FROM accounts a LEFT JOIN account_groups ag ON ag.account_id=a.id
		LEFT JOIN routing_decisions rd ON rd.account_id=a.id GROUP BY a.id ORDER BY a.id`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var accountID, group string
		if err := rows.Scan(&accountID, &group); err != nil {
			return nil, nil, err
		}
		if _, manual := fused[accountID]; manual {
			findings = append(findings, alertFinding{
				"console:routing:breaker:" + accountID + ":" + group,
				"account.routing_breaker", "account", accountID, "MANUAL_FUSE",
			})
		}
	}
	return findings, fused, rows.Err()
}
