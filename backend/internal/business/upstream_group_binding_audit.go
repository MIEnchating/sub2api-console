package business

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
)

type UpstreamGroupBindingAuditAccount struct {
	ID   string  `json:"id"`
	Name *string `json:"name"`
}

type UpstreamGroupBindingAuditItem struct {
	UpstreamID   string                             `json:"upstream_id"`
	Host         string                             `json:"host"`
	GroupID      *string                            `json:"group_id"`
	GroupName    string                             `json:"group_name"`
	AccountCount int                                `json:"account_count"`
	Accounts     []UpstreamGroupBindingAuditAccount `json:"accounts"`
	Status       string                             `json:"status"`
	Reason       *string                            `json:"reason"`
}

type UpstreamGroupBindingAudit struct {
	Items         []UpstreamGroupBindingAuditItem `json:"items"`
	TotalBindings int                             `json:"total_bindings"`
	Present       int                             `json:"present"`
	Missing       int                             `json:"missing"`
	Unknown       int                             `json:"unknown"`
}

type upstreamGroupBindingAuditRow struct {
	bindingID   int64
	upstreamID  string
	host        string
	groupID     sql.NullString
	groupName   sql.NullString
	accountID   string
	accountName sql.NullString
	lifecycle   sql.NullString
}

func (s *Store) UpstreamGroupBindingAudit(ctx context.Context) (UpstreamGroupBindingAudit, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT b.id,bi.upstream_id,COALESCE(h.host,b.upstream_host),
		COALESCE(NULLIF(TRIM(k.parent_entity_id),''),NULLIF(TRIM(bi.upstream_group_id),''),NULLIF(TRIM(b.upstream_group_id),'')),
		COALESCE(NULLIF(TRIM(g.name),''),NULLIF(TRIM(b.upstream_group),'')),
		b.local_account_id,a.name,g.lifecycle_state
		FROM bindings b
		JOIN binding_identities bi ON bi.binding_id=b.id
		LEFT JOIN upstream_identity_hosts h ON h.upstream_id=bi.upstream_id AND h.is_primary=1
		LEFT JOIN upstream_catalog_entities k ON k.upstream_id=bi.upstream_id AND k.entity_kind='key'
			AND k.entity_id=bi.upstream_key_id
		LEFT JOIN upstream_catalog_entities g ON g.upstream_id=bi.upstream_id AND g.entity_kind='group'
			AND g.entity_id=COALESCE(NULLIF(TRIM(k.parent_entity_id),''),NULLIF(TRIM(bi.upstream_group_id),''),NULLIF(TRIM(b.upstream_group_id),''))
		LEFT JOIN accounts a ON a.id=b.local_account_id
		ORDER BY 3,2,5,6,1`)
	if err != nil {
		return UpstreamGroupBindingAudit{}, err
	}
	defer rows.Close()

	result := UpstreamGroupBindingAudit{Items: []UpstreamGroupBindingAuditItem{}}
	itemIndexes := map[string]int{}
	for rows.Next() {
		var row upstreamGroupBindingAuditRow
		if err := rows.Scan(
			&row.bindingID,
			&row.upstreamID,
			&row.host,
			&row.groupID,
			&row.groupName,
			&row.accountID,
			&row.accountName,
			&row.lifecycle,
		); err != nil {
			return UpstreamGroupBindingAudit{}, err
		}

		groupID := strings.TrimSpace(row.groupID.String)
		itemKey := row.upstreamID + "\x00" + groupID
		if groupID == "" {
			itemKey += "\x00" + strings.TrimSpace(row.groupName.String)
			if strings.TrimSpace(row.groupName.String) == "" {
				itemKey += "\x00" + strconv.FormatInt(row.bindingID, 10)
			}
		}
		itemIndex, found := itemIndexes[itemKey]
		if !found {
			status, reason := groupBindingAuditStatus(groupID, row.lifecycle)
			groupName := strings.TrimSpace(row.groupName.String)
			if groupName == "" {
				if groupID != "" {
					groupName = groupID
				} else {
					groupName = "未记录分组"
				}
			}
			item := UpstreamGroupBindingAuditItem{
				UpstreamID: row.upstreamID,
				Host:       row.host,
				GroupID:    nullString(row.groupID),
				GroupName:  groupName,
				Accounts:   []UpstreamGroupBindingAuditAccount{},
				Status:     status,
				Reason:     reason,
			}
			result.Items = append(result.Items, item)
			itemIndex = len(result.Items) - 1
			itemIndexes[itemKey] = itemIndex
			switch status {
			case "present":
				result.Present++
			case "missing":
				result.Missing++
			default:
				result.Unknown++
			}
		}

		item := &result.Items[itemIndex]
		item.Accounts = append(item.Accounts, UpstreamGroupBindingAuditAccount{
			ID:   row.accountID,
			Name: nullString(row.accountName),
		})
		item.AccountCount = len(item.Accounts)
		result.TotalBindings++
	}
	if err := rows.Err(); err != nil {
		return UpstreamGroupBindingAudit{}, err
	}
	return result, nil
}

func groupBindingAuditStatus(groupID string, lifecycle sql.NullString) (string, *string) {
	if strings.TrimSpace(groupID) == "" {
		return "unknown", stringPointer("绑定记录没有稳定的上游分组 ID")
	}
	if !lifecycle.Valid || strings.TrimSpace(lifecycle.String) == "" {
		return "unknown", stringPointer("当前目录中没有这个分组的核对记录")
	}
	switch strings.TrimSpace(lifecycle.String) {
	case "active":
		return "present", nil
	case "missing", "retired":
		return "missing", stringPointer("当前上游目录中已确认不存在")
	default:
		return "unknown", stringPointer("分组暂未确认是否仍然存在")
	}
}
