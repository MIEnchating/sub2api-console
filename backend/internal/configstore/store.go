package configstore

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"
	"golang.org/x/crypto/pbkdf2"
	_ "modernc.org/sqlite"
)

const passwordRounds = 310_000

type Store struct {
	db *sql.DB
}

type PublicStatus struct {
	Initialized         bool     `json:"initialized"`
	TargetConfigured    bool     `json:"target_configured"`
	Username            *string  `json:"username"`
	ConfigurationErrors []string `json:"configuration_errors"`
}

type RuntimeSettings struct {
	Keys                             []string
	AdminBaseURL                     *string
	RequestTimeoutSeconds            int
	AccountDefaultConcurrency        int64
	AccountDefaultPriority           int64
	RequestTimeoutConfigurationError string
}

type AccountDefaultsSettings struct {
	Concurrency int64 `json:"concurrency"`
	Priority    int64 `json:"priority"`
}

type NotificationStatus struct {
	Configured             bool     `json:"configured"`
	AppID                  string   `json:"app_id"`
	ClientSecretConfigured bool     `json:"client_secret_configured"`
	HomeChannel            string   `json:"home_channel"`
	ChannelType            string   `json:"channel_type"`
	DestinationConfigured  bool     `json:"destination_configured"`
	ConfigurationErrors    []string `json:"configuration_errors"`
}

type NotificationSettings struct {
	AppID           string
	ClientSecret    string
	HomeChannel     string
	HomeChannelType string
}

type TargetSettings struct {
	BaseURL        string
	AdminKey       string
	TimeoutSeconds int
}

type LogCleanupSettings struct {
	Enabled       bool    `json:"enabled"`
	RetentionDays int     `json:"retention_days"`
	LastRunAt     *string `json:"last_run_at"`
}

func Open(path string) (*Store, error) {
	if err := sqliteutil.Prepare(path); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	store := &Store{db: db}
	if err := store.ensureSchema(context.Background()); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	if err := sqliteutil.Secure(path); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) ensureSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS auth_records (
			host TEXT PRIMARY KEY, base_url TEXT NOT NULL, upstream_type TEXT NOT NULL,
			auth_mode TEXT NOT NULL, access_token TEXT, refresh_token TEXT, admin_key TEXT,
			user_id TEXT, headers_json TEXT NOT NULL DEFAULT '{}', cookies_json TEXT NOT NULL DEFAULT '{}',
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS vault_entries (
			entry TEXT PRIMARY KEY, username TEXT, password TEXT,
			hosts_json TEXT NOT NULL DEFAULT '[]', headers_json TEXT NOT NULL DEFAULT '{}',
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS upstream_key_secrets (
			host TEXT NOT NULL, key_id TEXT NOT NULL, group_id TEXT NOT NULL,
			secret TEXT NOT NULL, updated_at TEXT NOT NULL,
			PRIMARY KEY(host,key_id,group_id)
		)`,
		`CREATE TABLE IF NOT EXISTS console_sessions (
			token_hash TEXT PRIMARY KEY, username TEXT NOT NULL,
			expires_at TEXT NOT NULL, created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS ix_console_sessions_expires_at ON console_sessions(expires_at)`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM settings WHERE key='runtime.probes_enabled'`); err != nil {
		return err
	}
	return nil
}

func (s *Store) PublicStatus(ctx context.Context) (PublicStatus, error) {
	values, err := s.settings(ctx)
	if err != nil {
		return PublicStatus{}, err
	}
	errorsFound := make([]string, 0)
	username, usernamePresent := values["console.username"]
	passwordHash, hashPresent := values["console.password_hash"]
	if usernamePresent && strings.TrimSpace(username) == "" {
		errorsFound = append(errorsFound, "console.username")
	}
	if hashPresent && !validPasswordHashShape(passwordHash) {
		errorsFound = append(errorsFound, "console.password_hash")
	}
	if usernamePresent != hashPresent {
		errorsFound = append(errorsFound, "console.credentials")
	}
	if targetError := targetConfigurationError(values); targetError != "" {
		errorsFound = append(errorsFound, targetError)
	}
	initialized := strings.TrimSpace(username) != "" && validPasswordHashShape(passwordHash)
	var publicUsername *string
	if usernamePresent {
		copy := username
		publicUsername = &copy
	}
	return PublicStatus{
		Initialized:         initialized,
		TargetConfigured:    validTarget(values),
		Username:            publicUsername,
		ConfigurationErrors: unique(errorsFound),
	}, nil
}

func (s *Store) IsInitialized(ctx context.Context) (bool, error) {
	status, err := s.PublicStatus(ctx)
	return status.Initialized, err
}

func (s *Store) RuntimeSettings(ctx context.Context) (RuntimeSettings, error) {
	values, err := s.settings(ctx)
	if err != nil {
		return RuntimeSettings{}, err
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var adminBaseURL *string
	if raw := strings.TrimSpace(values["target.base_url"]); raw != "" {
		if normalized, validationErr := ValidateBaseURL(raw); validationErr == nil {
			adminBaseURL = &normalized
		}
	}
	timeout := 30
	timeoutError := ""
	if raw, present := values["target.timeout_seconds"]; present {
		parsed, validationErr := validateRequestTimeout(raw)
		if validationErr != nil {
			timeoutError = "target.timeout_seconds"
		} else {
			timeout = parsed
		}
	}
	accountDefaults, accountDefaultsErr := accountDefaultsFromValues(values)
	if accountDefaultsErr != nil {
		return RuntimeSettings{}, accountDefaultsErr
	}
	return RuntimeSettings{
		Keys:                             keys,
		AdminBaseURL:                     adminBaseURL,
		RequestTimeoutSeconds:            timeout,
		AccountDefaultConcurrency:        accountDefaults.Concurrency,
		AccountDefaultPriority:           accountDefaults.Priority,
		RequestTimeoutConfigurationError: timeoutError,
	}, nil
}

func (s *Store) AccountDefaults(ctx context.Context) (AccountDefaultsSettings, error) {
	values, err := s.settings(ctx)
	if err != nil {
		return AccountDefaultsSettings{}, err
	}
	return accountDefaultsFromValues(values)
}

func (s *Store) ConfigureAccountDefaults(ctx context.Context, concurrency, priority int64) (AccountDefaultsSettings, error) {
	settings := AccountDefaultsSettings{Concurrency: concurrency, Priority: priority}
	if err := validateAccountDefaults(settings); err != nil {
		return AccountDefaultsSettings{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AccountDefaultsSettings{}, err
	}
	defer tx.Rollback()
	for key, value := range map[string]int64{
		"accounts.default_concurrency": concurrency,
		"accounts.default_priority":    priority,
	} {
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO settings(key,value) VALUES(?,?)`, key, strconv.FormatInt(value, 10)); err != nil {
			return AccountDefaultsSettings{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return AccountDefaultsSettings{}, err
	}
	return settings, nil
}

func accountDefaultsFromValues(values map[string]string) (AccountDefaultsSettings, error) {
	result := AccountDefaultsSettings{Concurrency: 10, Priority: 1}
	for key, target := range map[string]*int64{
		"accounts.default_concurrency": &result.Concurrency,
		"accounts.default_priority":    &result.Priority,
	} {
		raw, present := values[key]
		if !present {
			continue
		}
		parsed, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil {
			return AccountDefaultsSettings{}, fmt.Errorf("%s 配置无效", key)
		}
		*target = parsed
	}
	if err := validateAccountDefaults(result); err != nil {
		return AccountDefaultsSettings{}, err
	}
	return result, nil
}

func validateAccountDefaults(settings AccountDefaultsSettings) error {
	if settings.Concurrency < 1 || settings.Concurrency > 10_000_000 {
		return errors.New("账号默认并发必须是 1 到 10000000 之间的整数")
	}
	if settings.Priority < 1 || settings.Priority > 10_000_000 {
		return errors.New("账号默认优先级必须是 1 到 10000000 之间的整数")
	}
	return nil
}

func (s *Store) LogCleanupSettings(ctx context.Context) (LogCleanupSettings, error) {
	values, err := s.settings(ctx)
	if err != nil {
		return LogCleanupSettings{}, err
	}
	result := LogCleanupSettings{RetentionDays: 30}
	if raw, present := values["logs.cleanup_enabled"]; present {
		parsed, parseErr := strconv.ParseBool(strings.TrimSpace(raw))
		if parseErr != nil {
			return LogCleanupSettings{}, errors.New("logs.cleanup_enabled 配置无效")
		}
		result.Enabled = parsed
	}
	if raw, present := values["logs.retention_days"]; present {
		parsed, parseErr := strconv.Atoi(strings.TrimSpace(raw))
		if parseErr != nil || parsed < 1 || parsed > 3650 {
			return LogCleanupSettings{}, errors.New("logs.retention_days 必须在 1 到 3650 之间")
		}
		result.RetentionDays = parsed
	}
	if raw := strings.TrimSpace(values["logs.cleanup_last_run_at"]); raw != "" {
		parsed, parseErr := time.Parse(time.RFC3339Nano, raw)
		if parseErr != nil {
			return LogCleanupSettings{}, errors.New("logs.cleanup_last_run_at 配置无效")
		}
		normalized := parsed.UTC().Format(time.RFC3339Nano)
		result.LastRunAt = &normalized
	}
	return result, nil
}

func (s *Store) ConfigureLogCleanup(ctx context.Context, enabled bool, retentionDays int) (LogCleanupSettings, error) {
	if retentionDays < 1 || retentionDays > 3650 {
		return LogCleanupSettings{}, errors.New("日志保留天数必须在 1 到 3650 之间")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return LogCleanupSettings{}, err
	}
	defer tx.Rollback()
	for _, update := range [][2]string{
		{"logs.cleanup_enabled", strconv.FormatBool(enabled)},
		{"logs.retention_days", strconv.Itoa(retentionDays)},
	} {
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO settings(key,value) VALUES(?,?)`, update[0], update[1]); err != nil {
			return LogCleanupSettings{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return LogCleanupSettings{}, err
	}
	return s.LogCleanupSettings(ctx)
}

func (s *Store) MarkLogCleanupRun(ctx context.Context, completedAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR REPLACE INTO settings(key,value) VALUES('logs.cleanup_last_run_at',?)`,
		completedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func MaskUsername(username *string) *string {
	if username == nil || *username == "" {
		return nil
	}
	characters := []rune(*username)
	masked := "***"
	if len(characters) > 2 {
		masked = string(characters[:2]) + "***"
	}
	return &masked
}

func (s *Store) Initialize(ctx context.Context, username string, password string, baseURL string, adminKey string) error {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	adminKey = strings.TrimSpace(adminKey)
	if len(username) < 2 || len(username) > 80 {
		return errors.New("控制台账号长度必须为 2 到 80 个字符")
	}
	if len(password) < 10 || len(password) > 256 {
		return errors.New("控制台密码长度必须为 10 到 256 个字符")
	}
	if len(adminKey) > 4096 {
		return errors.New("管理密钥无效")
	}
	normalizedURL := ""
	var err error
	if strings.TrimSpace(baseURL) != "" {
		normalizedURL, err = ValidateBaseURL(baseURL)
		if err != nil {
			return err
		}
	}
	if (normalizedURL != "") != (adminKey != "") {
		return errors.New("管理地址和管理密钥必须同时填写")
	}
	passwordHash, err := hashPassword(password)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	values, err := settingsFrom(ctx, tx)
	if err != nil {
		return err
	}
	if _, found := values["console.username"]; found {
		return errors.New("控制台已经初始化，不能重复覆盖配置")
	}
	if _, found := values["console.password_hash"]; found {
		return errors.New("控制台已有损坏的初始化记录，不能覆盖；请先修复现有配置")
	}
	if normalizedURL == "" && !validTarget(values) {
		return errors.New("未找到 Sub2API 管理目标，请填写 Admin Base URL 和 Admin Key")
	}
	updates := map[string]string{
		"console.username":      username,
		"console.password_hash": passwordHash,
	}
	if normalizedURL != "" {
		updates["target.base_url"] = normalizedURL
		updates["target.admin_key"] = adminKey
	}
	for key, value := range updates {
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO settings(key,value) VALUES(?,?)`, key, value); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Authenticate(ctx context.Context, username string, password string) (bool, error) {
	values, err := s.settings(ctx)
	if err != nil {
		return false, err
	}
	storedUsername := values["console.username"]
	storedHash := values["console.password_hash"]
	usernameMatches := hmac.Equal([]byte(strings.TrimSpace(username)), []byte(storedUsername))
	passwordMatches := verifyPassword(password, storedHash)
	return usernameMatches && passwordMatches, nil
}

func (s *Store) CreateSession(ctx context.Context, username string, ttl time.Duration, now time.Time) (string, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return "", errors.New("会话用户名不能为空")
	}
	if ttl < time.Second || ttl > 31*24*time.Hour {
		return "", errors.New("会话有效期必须在 1 秒到 31 天之间")
	}
	issuedAt := now.UTC().Truncate(time.Second)
	expiresAt := issuedAt.Add(ttl)
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM console_sessions WHERE expires_at<=?`, formatTime(issuedAt)); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO console_sessions(token_hash,username,expires_at,created_at) VALUES(?,?,?,?)`,
		hashSessionToken(token), username, formatTime(expiresAt), formatTime(issuedAt),
	); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return token, nil
}

func (s *Store) SessionUser(ctx context.Context, token string, now time.Time) (*string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, nil
	}
	var username string
	var expiresAtRaw string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT username,expires_at FROM console_sessions WHERE token_hash=?`,
		hashSessionToken(token),
	).Scan(&username, &expiresAtRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	expiresAt, parseErr := parseTime(expiresAtRaw)
	username = strings.TrimSpace(username)
	if parseErr != nil || username == "" || !expiresAt.After(now.UTC()) {
		_, deleteErr := s.db.ExecContext(ctx, `DELETE FROM console_sessions WHERE token_hash=?`, hashSessionToken(token))
		return nil, deleteErr
	}
	return &username, nil
}

func (s *Store) RevokeSession(ctx context.Context, token string) error {
	if strings.TrimSpace(token) == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM console_sessions WHERE token_hash=?`, hashSessionToken(token))
	return err
}

func (s *Store) UpdateCredentials(ctx context.Context, currentPassword string, username string, newPassword *string) (string, error) {
	username = strings.TrimSpace(username)
	if len(username) < 2 || len(username) > 80 {
		return "", errors.New("控制台账号长度必须为 2 到 80 个字符")
	}
	if newPassword != nil && (len(*newPassword) < 10 || len(*newPassword) > 256) {
		return "", errors.New("控制台密码长度必须为 10 到 256 个字符")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var storedUsername string
	var storedHash string
	rows, err := tx.QueryContext(ctx, `SELECT key,value FROM settings WHERE key IN ('console.username','console.password_hash')`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return "", err
		}
		if key == "console.username" {
			storedUsername = value
		} else {
			storedHash = value
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if err := rows.Close(); err != nil {
		return "", err
	}
	if strings.TrimSpace(storedUsername) == "" || !validPasswordHashShape(storedHash) {
		return "", errors.New("控制台账号配置无效")
	}
	if !verifyPassword(currentPassword, storedHash) {
		return "", errors.New("当前密码错误")
	}
	if newPassword != nil && verifyPassword(*newPassword, storedHash) {
		return "", errors.New("新密码不能与当前密码相同")
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO settings(key,value) VALUES('console.username',?)`, username); err != nil {
		return "", err
	}
	if newPassword != nil {
		hash, err := hashPassword(*newPassword)
		if err != nil {
			return "", err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO settings(key,value) VALUES('console.password_hash',?)`, hash); err != nil {
			return "", err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM console_sessions`); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return username, nil
}

func (s *Store) ConfigureTarget(ctx context.Context, baseURL string, adminKey string, timeoutSeconds int) error {
	normalizedURL, err := ValidateBaseURL(baseURL)
	if err != nil {
		return err
	}
	if _, err := validateRequestTimeout(strconv.Itoa(timeoutSeconds)); err != nil {
		return err
	}
	values, err := s.settings(ctx)
	if err != nil {
		return err
	}
	normalizedKey := strings.TrimSpace(adminKey)
	if normalizedKey == "" && validTarget(values) {
		normalizedKey = values["target.admin_key"]
	}
	if normalizedKey == "" || len(normalizedKey) > 4096 {
		return errors.New("管理密钥无效")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	updates := [][2]string{
		{"target.base_url", normalizedURL},
		{"target.admin_key", normalizedKey},
		{"target.timeout_seconds", strconv.Itoa(timeoutSeconds)},
	}
	for _, update := range updates {
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO settings(key,value) VALUES(?,?)`, update[0], update[1]); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) TargetSettings(ctx context.Context) (TargetSettings, error) {
	values, err := s.settings(ctx)
	if err != nil {
		return TargetSettings{}, err
	}
	if errorText := targetConfigurationError(values); errorText != "" {
		return TargetSettings{}, errors.New(errorText)
	}
	if !validTarget(values) {
		return TargetSettings{}, errors.New("管理地址和管理密钥配置不完整")
	}
	timeout := 30
	if raw, present := values["target.timeout_seconds"]; present {
		timeout, err = validateRequestTimeout(raw)
		if err != nil {
			return TargetSettings{}, err
		}
	}
	return TargetSettings{
		BaseURL:  strings.TrimRight(strings.TrimSpace(values["target.base_url"]), "/"),
		AdminKey: strings.TrimSpace(values["target.admin_key"]), TimeoutSeconds: timeout,
	}, nil
}

func (s *Store) NotificationPublicStatus(ctx context.Context) (NotificationStatus, error) {
	values, err := s.settings(ctx)
	if err != nil {
		return NotificationStatus{}, err
	}
	channelType := "c2c"
	if raw, present := values["qqbot.home_channel_type"]; present {
		channelType = raw
	}
	errorsFound := make([]string, 0, 1)
	if channelType != "c2c" && channelType != "group" && channelType != "channel" {
		errorsFound = append(errorsFound, "qqbot.home_channel_type")
	}
	appID := strings.TrimSpace(values["qqbot.app_id"])
	homeChannel := strings.TrimSpace(values["qqbot.home_channel"])
	clientSecretConfigured := strings.TrimSpace(values["qqbot.client_secret"]) != ""
	destinationConfigured := homeChannel != ""
	configured := appID != "" && clientSecretConfigured &&
		destinationConfigured && len(errorsFound) == 0
	return NotificationStatus{
		Configured:             configured,
		AppID:                  appID,
		ClientSecretConfigured: clientSecretConfigured,
		HomeChannel:            homeChannel,
		ChannelType:            channelType,
		DestinationConfigured:  destinationConfigured,
		ConfigurationErrors:    errorsFound,
	}, nil
}

func ValidateNotificationSettings(appID string, clientSecret string, homeChannel string, homeChannelType string) (NotificationSettings, error) {
	settings := NotificationSettings{
		AppID:           strings.TrimSpace(appID),
		ClientSecret:    strings.TrimSpace(clientSecret),
		HomeChannel:     strings.TrimSpace(homeChannel),
		HomeChannelType: strings.ToLower(strings.TrimSpace(homeChannelType)),
	}
	if settings.AppID == "" || settings.ClientSecret == "" || settings.HomeChannel == "" {
		return NotificationSettings{}, errors.New("QQBot App ID、Client Secret 和目标必须同时填写")
	}
	if settings.HomeChannelType != "c2c" && settings.HomeChannelType != "group" && settings.HomeChannelType != "channel" {
		return NotificationSettings{}, errors.New("QQBot 目标类型只能是 c2c、group 或 channel")
	}
	if len(settings.AppID) > 4096 || len(settings.ClientSecret) > 4096 || len(settings.HomeChannel) > 4096 || len(settings.HomeChannelType) > 4096 {
		return NotificationSettings{}, errors.New("QQBot 配置字段过长")
	}
	return settings, nil
}

func (s *Store) ConfigureNotifications(ctx context.Context, appID string, clientSecret string, homeChannel string, homeChannelType string) error {
	if strings.TrimSpace(clientSecret) == "" {
		current, err := s.NotificationSettings(ctx)
		if err != nil {
			return err
		}
		clientSecret = current.ClientSecret
	}
	settings, err := ValidateNotificationSettings(appID, clientSecret, homeChannel, homeChannelType)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	updates := [][2]string{
		{"qqbot.app_id", settings.AppID},
		{"qqbot.client_secret", settings.ClientSecret},
		{"qqbot.home_channel", settings.HomeChannel},
		{"qqbot.home_channel_type", settings.HomeChannelType},
	}
	for _, update := range updates {
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO settings(key,value) VALUES(?,?)`, update[0], update[1]); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) NotificationSettings(ctx context.Context) (NotificationSettings, error) {
	values, err := s.settings(ctx)
	if err != nil {
		return NotificationSettings{}, err
	}
	return NotificationSettings{
		AppID:           strings.TrimSpace(values["qqbot.app_id"]),
		ClientSecret:    strings.TrimSpace(values["qqbot.client_secret"]),
		HomeChannel:     strings.TrimSpace(values["qqbot.home_channel"]),
		HomeChannelType: strings.ToLower(strings.TrimSpace(values["qqbot.home_channel_type"])),
	}, nil
}

func (s *Store) settings(ctx context.Context) (map[string]string, error) {
	return settingsFrom(ctx, s.db)
}

func settingsFrom(ctx context.Context, queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}) (map[string]string, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT key,value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		values[key] = value
	}
	return values, rows.Err()
}

func ValidateBaseURL(raw string) (string, error) {
	normalized := strings.TrimRight(strings.TrimSpace(raw), "/")
	parsed, err := url.Parse(normalized)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return "", errors.New("管理地址必须是完整的 http 或 https URL")
	}
	return normalized, nil
}

func validateRequestTimeout(raw string) (int, error) {
	normalized := strings.TrimSpace(raw)
	parsed, err := strconv.Atoi(normalized)
	if err != nil || strconv.Itoa(parsed) != normalized || parsed < 1 || parsed > 120 {
		return 0, errors.New("请求超时必须在 1 到 120 秒之间")
	}
	return parsed, nil
}

func validTarget(values map[string]string) bool {
	baseURL := strings.TrimSpace(values["target.base_url"])
	adminKey := strings.TrimSpace(values["target.admin_key"])
	if baseURL == "" || adminKey == "" {
		return false
	}
	_, err := ValidateBaseURL(baseURL)
	return err == nil
}

func targetConfigurationError(values map[string]string) string {
	_, urlPresent := values["target.base_url"]
	_, keyPresent := values["target.admin_key"]
	if !urlPresent && !keyPresent {
		return ""
	}
	if strings.TrimSpace(values["target.base_url"]) == "" || strings.TrimSpace(values["target.admin_key"]) == "" {
		return "Admin Base URL 和 Admin Key 配置不完整"
	}
	if _, err := ValidateBaseURL(values["target.base_url"]); err != nil {
		return "Admin Base URL 配置无效"
	}
	return ""
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	digest := pbkdf2.Key([]byte(password), salt, passwordRounds, 32, sha256.New)
	return fmt.Sprintf(
		"pbkdf2_sha256$%d$%s$%s",
		passwordRounds,
		base64.URLEncoding.EncodeToString(salt),
		base64.URLEncoding.EncodeToString(digest),
	), nil
}

func verifyPassword(password string, encoded string) bool {
	algorithm, rounds, salt, expected, ok := decodePasswordHash(encoded)
	if !ok || algorithm != "pbkdf2_sha256" {
		return false
	}
	actual := pbkdf2.Key([]byte(password), salt, rounds, len(expected), sha256.New)
	return hmac.Equal(actual, expected)
}

func validPasswordHashShape(encoded string) bool {
	algorithm, rounds, salt, digest, ok := decodePasswordHash(encoded)
	return ok && algorithm == "pbkdf2_sha256" && rounds == passwordRounds && len(salt) == 16 && len(digest) == 32
}

func decodePasswordHash(encoded string) (string, int, []byte, []byte, bool) {
	parts := strings.SplitN(encoded, "$", 4)
	if len(parts) != 4 {
		return "", 0, nil, nil, false
	}
	rounds, err := strconv.Atoi(parts[1])
	if err != nil || rounds <= 0 {
		return "", 0, nil, nil, false
	}
	salt, err := base64.URLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", 0, nil, nil, false
	}
	digest, err := base64.URLEncoding.DecodeString(parts[3])
	if err != nil {
		return "", 0, nil, nil, false
	}
	return parts[0], rounds, salt, digest, true
}

func hashSessionToken(token string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(digest[:])
}

func sqliteDSN(path string) string {
	return "file:" + path + "?_txlock=immediate&_pragma=busy_timeout%285000%29&_pragma=journal_mode%28WAL%29"
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339)
}

func parseTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
}

func unique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, found := seen[value]; found {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
