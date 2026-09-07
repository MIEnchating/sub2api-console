package business

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maximumAccountModelSyncItems = 1000
	maximumModelBlockPatterns    = 200
)

type AccountModelCatalog struct {
	AccountID   string   `json:"account_id"`
	AccountName string   `json:"account_name"`
	Platform    string   `json:"platform"`
	Models      []string `json:"models"`
}

type AccountModelCoverage struct {
	Model        string `json:"model"`
	AccountCount int    `json:"account_count"`
}

type AccountModelSyncAccount struct {
	AccountID   string   `json:"account_id"`
	AccountName string   `json:"account_name"`
	Platform    string   `json:"platform"`
	Models      []string `json:"models"`
	ProbeModel  string   `json:"probe_model"`
}

type AccountModelSyncPreview struct {
	AccountCount        int                       `json:"account_count"`
	AccountsWithCatalog int                       `json:"accounts_with_catalog"`
	BlockedPatterns     []string                  `json:"blocked_patterns"`
	BlockedModels       []string                  `json:"blocked_models"`
	Models              []AccountModelCoverage    `json:"models"`
	Accounts            []AccountModelSyncAccount `json:"accounts"`
	Fingerprint         string                    `json:"fingerprint"`
}

type AccountModelSyncSettings struct {
	BlockedPatterns []string `json:"blocked_patterns"`
}

func (s *Store) SaveAccountEnabledModels(ctx context.Context, accountID string, models []string) error {
	if !positiveNumericID(strings.TrimSpace(accountID)) {
		return errors.New("账号必须使用有效的稳定 ID")
	}
	normalized, err := validatedAccountModels(models, false)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var rawMetadata string
	if err := tx.QueryRowContext(ctx, `SELECT metadata_json FROM accounts WHERE id=?`, accountID).Scan(&rawMetadata); err != nil {
		return err
	}
	metadata, err := decodeJSONObject(rawMetadata)
	if err != nil {
		return errors.New("账号元数据损坏，无法保存已启用模型")
	}
	values := make([]any, len(normalized))
	for index, model := range normalized {
		values[index] = model
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	metadata["enabled_models"] = values
	metadata["enabled_models_synced_at"] = now
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE accounts SET metadata_json=?,updated_at=? WHERE id=?`, string(encoded), now, accountID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) AccountModelCatalogs(ctx context.Context, accountIDs []string) ([]AccountModelCatalog, error) {
	ids, err := validateAccountModelSyncIDs(accountIDs)
	if err != nil {
		return nil, err
	}
	result := make([]AccountModelCatalog, 0, len(ids))
	for _, accountID := range ids {
		var name, rawMetadata string
		err := s.db.QueryRowContext(ctx, `SELECT name,metadata_json FROM accounts WHERE id=?`, accountID).Scan(&name, &rawMetadata)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("账号 %s 不存在", accountID)
		}
		if err != nil {
			return nil, err
		}
		metadata, err := decodeJSONObject(rawMetadata)
		if err != nil {
			return nil, fmt.Errorf("账号 %s 元数据损坏，无法读取模型目录", accountID)
		}
		models := normalizeAccountModels(metadataStringList(metadata["known_models"]))
		result = append(result, AccountModelCatalog{
			AccountID: accountID, AccountName: name,
			Platform: strings.ToLower(strings.TrimSpace(metadataText(metadata, "platform"))), Models: models,
		})
	}
	return result, nil
}

func (s *Store) AccountModelSyncPreview(ctx context.Context, accountIDs []string) (AccountModelSyncPreview, error) {
	catalogs, err := s.AccountModelCatalogs(ctx, accountIDs)
	if err != nil {
		return AccountModelSyncPreview{}, err
	}
	blockedPatterns, err := s.AccountModelExclusionList(ctx)
	if err != nil {
		return AccountModelSyncPreview{}, err
	}
	configuredProbeModels := map[string]any{}
	policy, err := s.readPolicyDocument(ctx, s.db, "control-plane")
	if err != nil {
		return AccountModelSyncPreview{}, err
	}
	if policy != nil {
		configuredProbeModels, _ = policy["account_test_models"].(map[string]any)
	}
	coverage := map[string]*AccountModelCoverage{}
	accounts := make([]AccountModelSyncAccount, 0, len(catalogs))
	accountsWithCatalog := 0
	for _, catalog := range catalogs {
		if len(catalog.Models) > 0 {
			accountsWithCatalog++
		}
		for _, model := range catalog.Models {
			key := strings.ToLower(model)
			item := coverage[key]
			if item == nil {
				item = &AccountModelCoverage{Model: model}
				coverage[key] = item
			}
			item.AccountCount++
		}
		probeModel := ""
		if configured, configErr := normalizeAccountTestModels(configuredProbeModels[catalog.AccountID]); configErr == nil && len(configured) > 0 {
			probeModel = configured[0]
		}
		accounts = append(accounts, AccountModelSyncAccount{
			AccountID: catalog.AccountID, AccountName: catalog.AccountName,
			Platform: catalog.Platform, Models: append([]string{}, catalog.Models...), ProbeModel: probeModel,
		})
	}
	models := make([]AccountModelCoverage, 0, len(coverage))
	blockedModels := make([]string, 0)
	for _, item := range coverage {
		models = append(models, *item)
		if ModelMatchesBlockPatterns(item.Model, blockedPatterns) {
			blockedModels = append(blockedModels, item.Model)
		}
	}
	sort.Slice(models, func(left, right int) bool {
		return strings.ToLower(models[left].Model) < strings.ToLower(models[right].Model)
	})
	sort.Slice(blockedModels, func(left, right int) bool {
		return strings.ToLower(blockedModels[left]) < strings.ToLower(blockedModels[right])
	})
	fingerprint, err := accountModelCatalogFingerprint(catalogs, blockedPatterns)
	if err != nil {
		return AccountModelSyncPreview{}, err
	}
	return AccountModelSyncPreview{
		AccountCount: len(catalogs), AccountsWithCatalog: accountsWithCatalog,
		BlockedPatterns: blockedPatterns, BlockedModels: blockedModels,
		Models: models, Accounts: accounts, Fingerprint: fingerprint,
	}, nil
}

func (s *Store) AccountModelSyncSettings(ctx context.Context) (AccountModelSyncSettings, error) {
	patterns, err := s.AccountModelExclusionList(ctx)
	if err != nil {
		return AccountModelSyncSettings{}, err
	}
	return AccountModelSyncSettings{BlockedPatterns: patterns}, nil
}

func (s *Store) AccountModelExclusionList(ctx context.Context) ([]string, error) {
	document, err := s.readPolicyDocument(ctx, s.db, "control-plane")
	if err != nil {
		return nil, err
	}
	if document == nil {
		return nil, errors.New("控制面策略不存在")
	}
	section, present := document["model_sync"]
	if !present {
		return []string{}, nil
	}
	object, ok := section.(map[string]any)
	if !ok {
		return nil, errors.New("模型同步策略配置无效")
	}
	raw, present := object["blocked_patterns"]
	if !present {
		raw, present = object["excluded_models"]
	}
	if !present {
		return []string{}, nil
	}
	values, ok := raw.([]any)
	if !ok {
		return nil, errors.New("模型同步排除列表配置无效")
	}
	models := make([]string, 0, len(values))
	for _, value := range values {
		model, ok := value.(string)
		if !ok {
			return nil, errors.New("模型同步排除列表配置无效")
		}
		models = append(models, model)
	}
	return validatedModelBlockPatterns(models)
}

func (s *Store) SaveAccountModelExclusionList(ctx context.Context, models []string, actor string) error {
	normalized, err := validatedModelBlockPatterns(models)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	document, err := s.readPolicyDocument(ctx, tx, "control-plane")
	if err != nil {
		return err
	}
	if document == nil {
		return errors.New("控制面策略不存在")
	}
	values := make([]any, len(normalized))
	for index, model := range normalized {
		values[index] = model
	}
	section := map[string]any{}
	if existing, ok := document["model_sync"].(map[string]any); ok {
		for key, value := range existing {
			section[key] = value
		}
	}
	section["blocked_patterns"] = values
	delete(section, "excluded_models")
	document["model_sync"] = section
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.writePolicyDocument(ctx, tx, "control-plane", document, now); err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{"actor": strings.TrimSpace(actor), "blocked_patterns": normalized})
	if err != nil {
		return err
	}
	var minimum sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MIN(source_id) FROM runtime_events WHERE source_id < 0`).Scan(&minimum); err != nil {
		return err
	}
	sourceID := int64(-1)
	if minimum.Valid && minimum.Int64 <= -1 {
		sourceID = minimum.Int64 - 1
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO runtime_events(source_id,event_type,created_at,status,summary,payload_json)
		VALUES(?,?,?,?,?,?)`, sourceID, "account_model_policy.updated", now, "succeeded",
		fmt.Sprintf("全局屏蔽模型列表已更新：%d 条规则", len(normalized)), string(payload)); err != nil {
		return err
	}
	return tx.Commit()
}

func validatedModelBlockPatterns(patterns []string) ([]string, error) {
	if len(patterns) > maximumModelBlockPatterns {
		return nil, fmt.Errorf("全局屏蔽模型最多配置 %d 条规则", maximumModelBlockPatterns)
	}
	normalized := normalizeAccountModels(patterns)
	for _, pattern := range normalized {
		if utf8.RuneCountInString(pattern) > 256 {
			return nil, errors.New("全局屏蔽模型规则不能超过 256 个字符")
		}
	}
	return normalized, nil
}

func ModelMatchesBlockPatterns(model string, patterns []string) bool {
	value := []rune(strings.ToLower(strings.TrimSpace(model)))
	for _, rawPattern := range patterns {
		pattern := []rune(strings.ToLower(strings.TrimSpace(rawPattern)))
		if wildcardModelMatch(pattern, value) {
			return true
		}
	}
	return false
}

func wildcardModelMatch(pattern, value []rune) bool {
	previous := make([]bool, len(value)+1)
	previous[0] = true
	for _, token := range pattern {
		current := make([]bool, len(value)+1)
		if token == '*' {
			current[0] = previous[0]
		}
		for index := 1; index <= len(value); index++ {
			switch token {
			case '*':
				current[index] = previous[index] || current[index-1]
			case '?':
				current[index] = previous[index-1]
			default:
				current[index] = previous[index-1] && token == value[index-1]
			}
		}
		previous = current
	}
	return previous[len(value)]
}

func validateAccountModelSyncIDs(accountIDs []string) ([]string, error) {
	if len(accountIDs) == 0 || len(accountIDs) > maximumAccountModelSyncItems {
		return nil, fmt.Errorf("请选择 1 到 %d 个账号", maximumAccountModelSyncItems)
	}
	result := make([]string, 0, len(accountIDs))
	seen := map[string]struct{}{}
	for _, raw := range accountIDs {
		accountID := strings.TrimSpace(raw)
		if !positiveNumericID(accountID) {
			return nil, errors.New("账号必须使用有效的稳定 ID")
		}
		if _, duplicate := seen[accountID]; duplicate {
			return nil, fmt.Errorf("账号 ID %s 重复", accountID)
		}
		seen[accountID] = struct{}{}
		result = append(result, accountID)
	}
	return result, nil
}

func validatedAccountModels(models []string, allowEmpty bool) ([]string, error) {
	if len(models) > 500 {
		return nil, errors.New("模型列表不能超过 500 项")
	}
	normalized := normalizeAccountModels(models)
	if !allowEmpty && len(normalized) == 0 {
		return nil, errors.New("请至少选择一个允许模型")
	}
	for _, model := range normalized {
		if utf8.RuneCountInString(model) > 256 {
			return nil, errors.New("模型名称不能超过 256 个字符")
		}
	}
	return normalized, nil
}

func normalizeAccountModels(models []string) []string {
	result := make([]string, 0, len(models))
	seen := map[string]struct{}{}
	for _, raw := range models {
		model := strings.TrimSpace(raw)
		if model == "" {
			continue
		}
		key := strings.ToLower(model)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, model)
	}
	sort.Slice(result, func(left, right int) bool {
		return strings.ToLower(result[left]) < strings.ToLower(result[right])
	})
	return result
}

func accountModelCatalogFingerprint(catalogs []AccountModelCatalog, blockedPatterns []string) (string, error) {
	type fingerprintCatalog struct {
		AccountID string   `json:"account_id"`
		Models    []string `json:"models"`
	}
	stable := make([]fingerprintCatalog, 0, len(catalogs))
	for _, catalog := range catalogs {
		models := normalizeAccountModels(catalog.Models)
		for index := range models {
			models[index] = strings.ToLower(models[index])
		}
		stable = append(stable, fingerprintCatalog{AccountID: catalog.AccountID, Models: models})
	}
	sort.Slice(stable, func(left, right int) bool { return stable[left].AccountID < stable[right].AccountID })
	patterns := normalizeAccountModels(blockedPatterns)
	for index := range patterns {
		patterns[index] = strings.ToLower(patterns[index])
	}
	encoded, err := json.Marshal(struct {
		Catalogs        []fingerprintCatalog `json:"catalogs"`
		BlockedPatterns []string             `json:"blocked_patterns"`
	}{Catalogs: stable, BlockedPatterns: patterns})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}
