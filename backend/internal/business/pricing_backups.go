package business

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type PricingBackupAccount struct {
	AccountID   string   `json:"account_id"`
	AccountName string   `json:"account_name"`
	GroupIDs    []string `json:"group_ids"`
	GroupNames  []string `json:"group_names"`
}

type PricingBackup struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Actor        string                 `json:"actor"`
	AccountCount int                    `json:"account_count"`
	CreatedAt    string                 `json:"created_at"`
	Accounts     []PricingBackupAccount `json:"accounts,omitempty"`
}

func (s *Store) CreatePricingBackup(ctx context.Context, name, actor string) (PricingBackup, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 80 {
		return PricingBackup{}, errors.New("备份名称必须为 1 到 80 个字符")
	}
	id, err := pricingBackupID()
	if err != nil {
		return PricingBackup{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PricingBackup{}, err
	}
	defer tx.Rollback()
	accounts, err := pricingBackupAccounts(ctx, tx)
	if err != nil {
		return PricingBackup{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT INTO pricing_backups(id,name,actor,account_count,created_at) VALUES(?,?,?,?,?)`,
		id, name, strings.TrimSpace(actor), len(accounts), now); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return PricingBackup{}, fmt.Errorf("备份名称 %q 已存在", name)
		}
		return PricingBackup{}, err
	}
	for _, account := range accounts {
		groupIDs, _ := json.Marshal(account.GroupIDs)
		groupNames, _ := json.Marshal(account.GroupNames)
		if _, err := tx.ExecContext(ctx, `INSERT INTO pricing_backup_accounts(
			backup_id,account_id,account_name,group_ids_json,group_names_json) VALUES(?,?,?,?,?)`,
			id, account.AccountID, account.AccountName, string(groupIDs), string(groupNames)); err != nil {
			return PricingBackup{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return PricingBackup{}, err
	}
	return PricingBackup{ID: id, Name: name, Actor: strings.TrimSpace(actor), AccountCount: len(accounts), CreatedAt: now, Accounts: accounts}, nil
}

func (s *Store) PricingBackups(ctx context.Context) ([]PricingBackup, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,actor,account_count,created_at FROM pricing_backups ORDER BY created_at DESC,id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []PricingBackup{}
	for rows.Next() {
		var item PricingBackup
		if err := rows.Scan(&item.ID, &item.Name, &item.Actor, &item.AccountCount, &item.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) PricingBackup(ctx context.Context, id string) (PricingBackup, error) {
	id = strings.TrimSpace(id)
	var result PricingBackup
	err := s.db.QueryRowContext(ctx, `SELECT id,name,actor,account_count,created_at FROM pricing_backups WHERE id=?`, id).
		Scan(&result.ID, &result.Name, &result.Actor, &result.AccountCount, &result.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return PricingBackup{}, errors.New("价格分组备份不存在")
	}
	if err != nil {
		return PricingBackup{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT account_id,account_name,group_ids_json,group_names_json
		FROM pricing_backup_accounts WHERE backup_id=? ORDER BY CAST(account_id AS INTEGER),account_id`, id)
	if err != nil {
		return PricingBackup{}, err
	}
	defer rows.Close()
	result.Accounts = []PricingBackupAccount{}
	for rows.Next() {
		var item PricingBackupAccount
		var groupIDsRaw, groupNamesRaw string
		if err := rows.Scan(&item.AccountID, &item.AccountName, &groupIDsRaw, &groupNamesRaw); err != nil {
			return PricingBackup{}, err
		}
		if err := json.Unmarshal([]byte(groupIDsRaw), &item.GroupIDs); err != nil {
			return PricingBackup{}, errors.New("价格分组备份中的分组 ID 已损坏")
		}
		if err := json.Unmarshal([]byte(groupNamesRaw), &item.GroupNames); err != nil {
			return PricingBackup{}, errors.New("价格分组备份中的分组名称已损坏")
		}
		result.Accounts = append(result.Accounts, item)
	}
	return result, rows.Err()
}

func pricingBackupAccounts(ctx context.Context, queryer policyQueryer) ([]PricingBackupAccount, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT a.id,a.name,COALESCE(ag.group_id,lg.remote_id),ag.group_name FROM accounts a
		LEFT JOIN account_groups ag ON ag.account_id=a.id LEFT JOIN local_groups lg ON lg.name=ag.group_name
		WHERE a.id GLOB '[0-9]*' AND CAST(a.id AS INTEGER)>0
		ORDER BY CAST(a.id AS INTEGER),a.id,CAST(COALESCE(ag.group_id,lg.remote_id) AS INTEGER),COALESCE(ag.group_id,lg.remote_id),ag.group_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []PricingBackupAccount{}
	byID := map[string]int{}
	for rows.Next() {
		var accountID, accountName string
		var groupID, groupName sql.NullString
		if err := rows.Scan(&accountID, &accountName, &groupID, &groupName); err != nil {
			return nil, err
		}
		index, found := byID[accountID]
		if !found {
			index = len(result)
			byID[accountID] = index
			result = append(result, PricingBackupAccount{AccountID: accountID, AccountName: accountName, GroupIDs: []string{}, GroupNames: []string{}})
		}
		if groupID.Valid && positiveNumericID(strings.TrimSpace(groupID.String)) {
			result[index].GroupIDs = append(result[index].GroupIDs, strings.TrimSpace(groupID.String))
			result[index].GroupNames = append(result[index].GroupNames, strings.TrimSpace(groupName.String))
		}
	}
	for index := range result {
		paired := make([]struct{ id, name string }, len(result[index].GroupIDs))
		for pairIndex := range paired {
			paired[pairIndex].id = result[index].GroupIDs[pairIndex]
			paired[pairIndex].name = result[index].GroupNames[pairIndex]
		}
		sort.Slice(paired, func(left, right int) bool { return stableNumericLess(paired[left].id, paired[right].id) })
		for pairIndex := range paired {
			result[index].GroupIDs[pairIndex] = paired[pairIndex].id
			result[index].GroupNames[pairIndex] = paired[pairIndex].name
		}
	}
	return result, rows.Err()
}

func pricingBackupID() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
