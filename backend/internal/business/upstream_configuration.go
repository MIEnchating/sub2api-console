package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type UpstreamConfigurationWrite struct {
	Host         string
	Name         *string
	BaseURL      string
	UpstreamType string
	AuthMode     string
	RechargeRate string
}

type UpstreamConfigurationWriteResult struct {
	UpstreamID        string
	Host              string
	Name              string
	RechargeRate      string
	RawBalance        *string
	Balance           *string
	ConvertedGroups   int
	UnavailableGroups int
}

func (s *Store) UpstreamExists(ctx context.Context, host string) (bool, error) {
	var marker int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM upstreams WHERE host=?`, canonicalHost(host)).Scan(&marker)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) CreateUpstreamConfiguration(ctx context.Context, value UpstreamConfigurationWrite) (UpstreamConfigurationWriteResult, error) {
	value.Host = canonicalHost(value.Host)
	name, baseURL, platform, authMode, recharge, err := normalizeUpstreamConfiguration(value)
	if err != nil {
		return UpstreamConfigurationWriteResult{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	metadata, err := json.Marshal(map[string]any{
		"site_name": name, "auth_verified_at": now, "balance_status": "未读取", "catalog_status": "未同步",
	})
	if err != nil {
		return UpstreamConfigurationWriteResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return UpstreamConfigurationWriteResult{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO upstreams(
		host,base_url,upstream_type,auth_mode,enabled,auth_status,metadata_json,updated_at
	) VALUES(?,?,?,?,1,'已鉴权',?,?)`, value.Host, baseURL, platform, authMode, string(metadata), now); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return UpstreamConfigurationWriteResult{}, errors.New("上游 Host 已存在")
		}
		return UpstreamConfigurationWriteResult{}, err
	}
	upstreamID, err := s.createUpstreamIdentityTx(ctx, tx, value.Host, now)
	if err != nil {
		return UpstreamConfigurationWriteResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO recharge_rates(host,recharge_rate,note,updated_at) VALUES(?,?,?,?)
		ON CONFLICT(host) DO UPDATE SET recharge_rate=excluded.recharge_rate,note=excluded.note,updated_at=excluded.updated_at`,
		value.Host, recharge, "console-upstream-create", now); err != nil {
		return UpstreamConfigurationWriteResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return UpstreamConfigurationWriteResult{}, err
	}
	return UpstreamConfigurationWriteResult{UpstreamID: upstreamID, Host: value.Host, Name: name, RechargeRate: recharge}, nil
}

func (s *Store) UpdateUpstreamConfiguration(ctx context.Context, value UpstreamConfigurationWrite) (UpstreamConfigurationWriteResult, error) {
	value.Host = canonicalHost(value.Host)
	upstreamID, identityErr := s.upstreamIdentityID(ctx, value.Host)
	if identityErr != nil {
		return UpstreamConfigurationWriteResult{}, identityErr
	}
	name, baseURL, platform, authMode, recharge, err := normalizeUpstreamConfiguration(value)
	if err != nil {
		return UpstreamConfigurationWriteResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return UpstreamConfigurationWriteResult{}, err
	}
	defer tx.Rollback()
	var rawBalance, legacyBalance sql.NullString
	var metadataRaw string
	err = tx.QueryRowContext(ctx, `SELECT raw_balance,CAST(balance AS TEXT),metadata_json FROM upstreams WHERE host=?`, value.Host).Scan(
		&rawBalance, &legacyBalance, &metadataRaw,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return UpstreamConfigurationWriteResult{}, sql.ErrNoRows
	}
	if err != nil {
		return UpstreamConfigurationWriteResult{}, err
	}
	metadata, err := decodeObject(metadataRaw)
	if err != nil {
		return UpstreamConfigurationWriteResult{}, errors.New("上游 metadata 记录损坏")
	}
	if value.Name != nil {
		metadata["site_name"] = name
	} else {
		name = upstreamDisplayName(metadata, baseURL, "")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	metadata["auth_verified_at"] = now
	delete(metadata, "auth_error")
	metadataEncoded, err := json.Marshal(metadata)
	if err != nil {
		return UpstreamConfigurationWriteResult{}, err
	}
	var normalizedRaw *string
	if rawBalance.Valid {
		normalizedRaw = normalizeDecimal(rawBalance.String)
	} else if legacyBalance.Valid {
		normalizedRaw = normalizeDecimal(legacyBalance.String)
	}
	if normalizedRaw != nil && *normalizedRaw == "" {
		normalizedRaw = nil
	}
	mappedBalance := divideDecimalPointers(normalizedRaw, &recharge)
	rows, err := tx.QueryContext(ctx, `SELECT group_id,raw_rate FROM upstream_groups WHERE host=?`, value.Host)
	if err != nil {
		return UpstreamConfigurationWriteResult{}, err
	}
	defer rows.Close()
	type groupRate struct {
		id  string
		raw sql.NullString
	}
	groupRates := []groupRate{}
	for rows.Next() {
		var item groupRate
		if err := rows.Scan(&item.id, &item.raw); err != nil {
			return UpstreamConfigurationWriteResult{}, err
		}
		groupRates = append(groupRates, item)
	}
	if err := rows.Err(); err != nil {
		return UpstreamConfigurationWriteResult{}, err
	}
	if err := rows.Close(); err != nil {
		return UpstreamConfigurationWriteResult{}, err
	}
	result := UpstreamConfigurationWriteResult{
		UpstreamID: upstreamID, Host: value.Host, Name: name, RechargeRate: recharge, RawBalance: normalizedRaw, Balance: mappedBalance,
	}
	for _, group := range groupRates {
		var effective *string
		if group.raw.Valid {
			effective = divideMultiplierPointers(normalizePositiveDecimal(group.raw.String), &recharge)
		}
		if effective == nil {
			result.UnavailableGroups++
		} else {
			result.ConvertedGroups++
		}
		if _, err := tx.ExecContext(ctx, `UPDATE upstream_groups SET effective_rate=?,rate_source='manual-mapping',updated_at=?
			WHERE host=? AND group_id=?`, effective, now, value.Host, group.id); err != nil {
			return UpstreamConfigurationWriteResult{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO recharge_rates(host,recharge_rate,note,updated_at) VALUES(?,?,?,?)
		ON CONFLICT(host) DO UPDATE SET recharge_rate=excluded.recharge_rate,note=excluded.note,updated_at=excluded.updated_at`,
		value.Host, recharge, "console-upstream-edit", now); err != nil {
		return UpstreamConfigurationWriteResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE upstreams SET base_url=?,upstream_type=?,auth_mode=?,auth_status='已鉴权',
		raw_balance=?,mapped_balance=?,metadata_json=?,updated_at=? WHERE host=?`,
		baseURL, platform, authMode, normalizedRaw, mappedBalance, string(metadataEncoded), now, value.Host); err != nil {
		return UpstreamConfigurationWriteResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return UpstreamConfigurationWriteResult{}, err
	}
	return result, nil
}

func (s *Store) UpdateUpstreamClassification(ctx context.Context, host, upstreamType, authMode string) error {
	host = canonicalHost(host)
	upstreamType = strings.ToLower(strings.TrimSpace(upstreamType))
	authMode = strings.TrimSpace(authMode)
	if host == "" || upstreamType == "" || authMode == "" {
		return errors.New("上游 Host、平台和鉴权方式不能为空")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var metadataRaw string
	if err := tx.QueryRowContext(ctx, `SELECT metadata_json FROM upstreams WHERE host=?`, host).Scan(&metadataRaw); err != nil {
		return err
	}
	metadata, err := decodeObject(metadataRaw)
	if err != nil {
		return errors.New("上游 metadata 记录损坏")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	metadata["auth_verified_at"] = now
	delete(metadata, "auth_error")
	metadataEncoded, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE upstreams SET upstream_type=?,auth_mode=?,auth_status='已鉴权',metadata_json=?,updated_at=? WHERE host=?`,
		upstreamType, authMode, string(metadataEncoded), now, host)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}

func normalizeUpstreamConfiguration(value UpstreamConfigurationWrite) (string, string, string, string, string, error) {
	if value.Host == "" {
		return "", "", "", "", "", errors.New("上游配置必须包含 Host")
	}
	name := ""
	if value.Name != nil {
		name = strings.TrimSpace(*value.Name)
		if name == "" || len([]rune(name)) > 100 {
			return "", "", "", "", "", errors.New("上游名称长度必须在 1 到 100 之间")
		}
	}
	baseURL := strings.TrimRight(strings.TrimSpace(value.BaseURL), "/")
	platform := strings.ToLower(strings.TrimSpace(value.UpstreamType))
	authMode := strings.TrimSpace(value.AuthMode)
	if baseURL == "" || platform == "" || authMode == "" {
		return "", "", "", "", "", errors.New("上游地址、平台和鉴权方式不能为空")
	}
	recharge := normalizePositiveDecimal(value.RechargeRate)
	if recharge == nil {
		return "", "", "", "", "", errors.New("倍率必须是有限正数")
	}
	return name, baseURL, platform, authMode, *recharge, nil
}

func (s *Store) DeleteUpstreamConfiguration(ctx context.Context, host string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM upstreams WHERE host=?`, canonicalHost(host))
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count > 0, err
}
