package configstore

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

type VaultEntry struct {
	ID       string
	Entry    string
	Username *string
	Password *string
	Hosts    []string
	Headers  map[string]string
}

type VaultEntrySummary struct {
	ID              string   `json:"id"`
	Entry           string   `json:"entry"`
	Hosts           []string `json:"hosts"`
	HasUsername     bool     `json:"has_username"`
	HasPassword     bool     `json:"has_password"`
	UsernameIsEmail bool     `json:"username_is_email"`
	HeaderNames     []string `json:"header_names"`
}

func (s *Store) VaultEntry(ctx context.Context, entry string) (*VaultEntry, error) {
	return readVaultEntry(ctx, s.db, entry)
}

func readVaultEntry(ctx context.Context, reader privateRowReader, entry string) (*VaultEntry, error) {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return nil, errors.New("凭据 ID 不能为空")
	}
	var result VaultEntry
	var username, password sql.NullString
	var rawHosts, rawHeaders string
	err := reader.QueryRowContext(ctx, `SELECT id,entry,username,password,hosts_json,headers_json
		FROM vault_entries WHERE id=?`, entry).Scan(
		&result.ID, &result.Entry, &username, &password, &rawHosts, &rawHeaders,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result.Username, result.Password = nullableText(username), nullableText(password)
	result.Hosts, err = decodeStringList(rawHosts)
	if err != nil {
		return nil, fmt.Errorf("凭据 %s 的 Host 记录损坏", entry)
	}
	result.Headers, err = decodeStringMap(rawHeaders)
	if err != nil {
		return nil, fmt.Errorf("凭据 %s 的 headers 记录损坏", entry)
	}
	return &result, nil
}

var ErrVaultEntryExists = errors.New("凭据名称已存在，请使用其他名称")
var ErrVaultEntryMissing = errors.New("原凭据已不存在，请刷新密码箱后重试")

func (s *Store) SaveVaultEntry(ctx context.Context, entry VaultEntry, present map[string]bool) error {
	return s.saveVaultEntry(ctx, false, entry, present)
}

// NewVaultEntryID creates an opaque identity independent from the display name.
func NewVaultEntryID() string { return "vault_" + rand.Text() }

// UpdateVaultEntry never recreates a deleted credential, even if its name is reused.
func (s *Store) UpdateVaultEntry(ctx context.Context, id string, entry VaultEntry, present map[string]bool) error {
	entry.ID = strings.TrimSpace(id)
	if entry.ID == "" {
		return errors.New("凭据 ID 不能为空")
	}
	return s.saveVaultEntry(ctx, true, entry, present)
}

func (s *Store) saveVaultEntry(ctx context.Context, requireExisting bool, entry VaultEntry, present map[string]bool) error {
	entry.Entry = strings.TrimSpace(entry.Entry)
	if entry.Entry == "" || textLength(entry.Entry) > 255 {
		return errors.New("凭据名称长度必须在 1 到 255 之间")
	}
	if entry.ID == "" {
		entry.ID = NewVaultEntryID()
	}
	if textLength(entry.ID) > 64 {
		return errors.New("凭据 ID 无效")
	}
	tx, err := s.beginSettingsTx(ctx, "private.credentials")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	current, err := readVaultEntry(ctx, tx, entry.ID)
	if err != nil {
		return err
	}
	if requireExisting && current == nil {
		return ErrVaultEntryMissing
	}
	var sameNameID string
	err = tx.QueryRowContext(ctx, `SELECT id FROM vault_entries WHERE entry=?`, entry.Entry).Scan(&sameNameID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && sameNameID != entry.ID {
		return ErrVaultEntryExists
	}
	if current != nil && present != nil {
		if !present["username"] {
			entry.Username = current.Username
		}
		if !present["password"] {
			entry.Password = current.Password
		}
		if !present["hosts"] {
			entry.Hosts = current.Hosts
		}
		if !present["headers"] {
			entry.Headers = current.Headers
		}
	}
	if pointerLength(entry.Username) > 65536 || pointerLength(entry.Password) > 65536 {
		return errors.New("密码箱字段过长")
	}
	hosts := make([]string, 0, len(entry.Hosts))
	seen := map[string]struct{}{}
	for _, raw := range entry.Hosts {
		host := CanonicalHost(raw)
		if host == "" {
			return errors.New("密码箱 Host 不能为空")
		}
		if _, found := seen[host]; !found {
			seen[host] = struct{}{}
			hosts = append(hosts, host)
		}
	}
	if len(hosts) > 100 {
		return errors.New("密码箱 Host 数量不能超过 100")
	}
	sort.Strings(hosts)
	headers, err := normalizedHeaders(entry.Headers)
	if err != nil {
		return err
	}
	rawHosts, _ := json.Marshal(hosts)
	rawHeaders, _ := json.Marshal(headers)
	if current == nil {
		_, err = tx.ExecContext(ctx, `INSERT INTO vault_entries(id,entry,username,password,hosts_json,headers_json,updated_at) VALUES(?,?,?,?,?,?,?)`, entry.ID, entry.Entry, entry.Username, entry.Password, string(rawHosts), string(rawHeaders), time.Now().UTC().Format(time.RFC3339Nano))
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE vault_entries SET entry=?,username=?,password=?,hosts_json=?,headers_json=?,updated_at=? WHERE id=?`, entry.Entry, entry.Username, entry.Password, string(rawHosts), string(rawHeaders), time.Now().UTC().Format(time.RFC3339Nano), entry.ID)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeleteVaultEntry(ctx context.Context, entry string) (bool, error) {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return false, errors.New("凭据 ID 不能为空")
	}
	tx, err := s.beginSettingsTx(ctx, "private.credentials")
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `DELETE FROM vault_entries WHERE id=?`, entry)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return count > 0, tx.Commit()
}

func (s *Store) VaultEntryIndex(ctx context.Context) ([]VaultEntrySummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,entry,username,password,hosts_json,headers_json FROM vault_entries ORDER BY entry`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []VaultEntrySummary{}
	for rows.Next() {
		var item VaultEntrySummary
		var username, password sql.NullString
		var rawHosts, rawHeaders string
		if err := rows.Scan(&item.ID, &item.Entry, &username, &password, &rawHosts, &rawHeaders); err != nil {
			return nil, err
		}
		item.Hosts, err = decodeStringList(rawHosts)
		if err != nil {
			return nil, fmt.Errorf("凭据 %s 的 Host 记录损坏", item.Entry)
		}
		headers, err := decodeStringMap(rawHeaders)
		if err != nil {
			return nil, fmt.Errorf("凭据 %s 的 headers 记录损坏", item.Entry)
		}
		item.HasUsername, item.HasPassword = nonempty(username), nonempty(password)
		item.UsernameIsEmail = IsEmailUsername(nullableText(username))
		item.HeaderNames = sortedMapKeys(headers)
		result = append(result, item)
	}
	return result, rows.Err()
}

func IsEmailUsername(value *string) bool {
	if value == nil {
		return false
	}
	text := strings.TrimSpace(*value)
	if text == "" || text != *value || strings.ContainsAny(text, "\r\n\t ") {
		return false
	}
	at := strings.LastIndexByte(text, '@')
	if at < 1 || at != strings.IndexByte(text, '@') || at >= len(text)-3 {
		return false
	}
	domain := text[at+1:]
	dot := strings.LastIndexByte(domain, '.')
	return dot > 0 && dot < len(domain)-1
}

func decodeStringList(raw string) ([]string, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	var value []string
	if err := decoder.Decode(&value); err != nil || value == nil {
		return nil, errors.New("JSON 字符串数组无效")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, errors.New("JSON 包含尾随数据")
	}
	return value, nil
}

func pointerLength(value *string) int {
	if value == nil {
		return 0
	}
	return textLength(*value)
}
