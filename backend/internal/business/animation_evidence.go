package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// AnimationEvidence is the latest per-task verdict used by group-local routing.
// It is stored per current account/group membership so a multi-group account can
// receive different policy treatment in each group.
type AnimationEvidence struct {
	AccountID   string
	GroupName   string
	GroupID     *string
	TaskID      string
	Mode        string
	Verdict     string
	FailureKind string
	Error       string
	ObservedAt  string
}

func (s *Store) PersistAnimationEvidence(ctx context.Context, values []AnimationEvidence) error {
	if len(values) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, value := range values {
		if strings.TrimSpace(value.AccountID) == "" || strings.TrimSpace(value.TaskID) == "" || strings.TrimSpace(value.Mode) == "" {
			return errors.New("动画证据缺少账号、任务或检测类型")
		}
		observed := strings.TrimSpace(value.ObservedAt)
		if observed == "" {
			observed = time.Now().UTC().Format(healthSampleTimeLayout)
		} else if parsed, parseErr := time.Parse(time.RFC3339Nano, observed); parseErr == nil {
			observed = parsed.UTC().Format(healthSampleTimeLayout)
		} else {
			return errors.New("动画证据时间无效")
		}
		rows, queryErr := tx.QueryContext(ctx, `SELECT ag.group_name,ag.group_id FROM account_groups ag WHERE ag.account_id=?`, value.AccountID)
		if queryErr != nil {
			return queryErr
		}
		memberships := []struct {
			name string
			id   sql.NullString
		}{}
		for rows.Next() {
			var item struct {
				name string
				id   sql.NullString
			}
			if scanErr := rows.Scan(&item.name, &item.id); scanErr != nil {
				rows.Close()
				return scanErr
			}
			memberships = append(memberships, item)
		}
		if closeErr := rows.Close(); closeErr != nil {
			return closeErr
		}
		payload, marshalErr := json.Marshal(map[string]any{"task_id": value.TaskID, "mode": value.Mode, "verdict": value.Verdict, "failure_kind": value.FailureKind, "error": value.Error})
		if marshalErr != nil {
			return marshalErr
		}
		for _, membership := range memberships {
			if _, execErr := tx.ExecContext(ctx, `INSERT INTO health_samples(account_id,group_name,result,failure_reason,observed_at,source,evidence_key,payload_json)
				VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(source,evidence_key,account_id,group_name) DO UPDATE SET
				result=excluded.result,failure_reason=excluded.failure_reason,observed_at=excluded.observed_at,payload_json=excluded.payload_json`,
				value.AccountID, membership.name, value.Verdict, value.Error, observed, "animation", value.TaskID+":"+value.Mode, string(payload)); execErr != nil {
				return fmt.Errorf("保存动画证据失败：%w", execErr)
			}
		}
	}
	return tx.Commit()
}

func (s *Store) RoutingAnimationEvidence(ctx context.Context, accountID, groupName *string) ([]AnimationEvidence, error) {
	clauses := []string{"source='animation'"}
	args := []any{}
	if accountID != nil {
		clauses = append(clauses, "account_id=?")
		args = append(args, strings.TrimSpace(*accountID))
	}
	if groupName != nil {
		clauses = append(clauses, "group_name=?")
		args = append(args, strings.TrimSpace(*groupName))
	}
	query := `SELECT account_id,group_name,evidence_key,failure_reason,observed_at,payload_json FROM health_samples WHERE ` + strings.Join(clauses, " AND ") + ` ORDER BY CASE WHEN json_extract(payload_json,'$.mode')='precheck' THEN 0 ELSE 1 END, observed_at DESC,id DESC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []AnimationEvidence{}
	seen := map[string]bool{}
	for rows.Next() {
		var item AnimationEvidence
		var key, reason, observed, raw string
		if err := rows.Scan(&item.AccountID, &item.GroupName, &key, &reason, &observed, &raw); err != nil {
			return nil, err
		}
		identity := item.AccountID + "\x00" + item.GroupName
		if seen[identity] {
			continue
		}
		seen[identity] = true
		item.TaskID, item.Mode = key, "animation"
		var payload map[string]any
		if json.Unmarshal([]byte(raw), &payload) == nil {
			item.TaskID = stringValue(payload["task_id"])
			item.Mode = stringValue(payload["mode"])
			item.Verdict = stringValue(payload["verdict"])
			item.FailureKind = stringValue(payload["failure_kind"])
			item.Error = stringValue(payload["error"])
		}
		if strings.Contains(item.TaskID, ":") {
			parts := strings.SplitN(item.TaskID, ":", 2)
			item.TaskID, item.Mode = parts[0], parts[1]
		}
		if item.Error == "" {
			item.Error = reason
		}
		item.ObservedAt = observed
		result = append(result, item)
	}
	return result, rows.Err()
}
