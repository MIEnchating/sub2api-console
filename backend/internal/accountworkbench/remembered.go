package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

const rememberedAccountPrefix = "remembered-account:"

type RememberedAccount struct {
	ID             string    `json:"id"`
	AccountID      string    `json:"account_id"`
	Name           string    `json:"name"`
	Email          string    `json:"email"`
	Plan           string    `json:"plan"`
	LoginAvailable bool      `json:"login_available"`
	LastImportedAt time.Time `json:"last_imported_at"`
	Revision       int64     `json:"revision"`
}

// Records belong to a management target, not to the importing login session.
// Only the public summary can be returned to the browser.
type rememberedAccount struct {
	TargetKey   string            `json:"target_key"`
	SourceRunID string            `json:"source_run_id"`
	Public      RememberedAccount `json:"public"`
	Input       storedInput       `json:"input"`
}

func rememberedAccountID(target configstore.TargetSettings, accountID string) string {
	return digest([]string{targetKey(target), accountID})
}
func rememberedAccountKey(target configstore.TargetSettings, id string) string {
	return rememberedAccountPrefix + targetKey(target) + ":" + id
}

func (s *Service) saveRememberedAccount(ctx context.Context, value *privateRun, index int) error {
	if value.Public.Action != "import" || index < 0 || index >= len(value.Items) {
		return nil
	}
	row := value.Public.Items[index]
	if id, err := strconv.ParseUint(row.AccountID, 10, 63); err != nil || id == 0 {
		return errors.New("无法记住缺少稳定 ID 的账号")
	}
	if _, err := targetguard.Pin(targetguard.Expect(ctx, value.Target), s.private); err != nil {
		return err
	}
	s.rememberMu.Lock()
	defer s.rememberMu.Unlock()
	input := value.Items[index]
	// Durable login material needs verified identity, not an old OAuth access or
	// refresh token that a later maintenance task may already have rotated.
	identity := map[string]any{}
	copyInputIdentity(identity, input.Credentials)
	input.Credentials = identity
	input.Item.Credentials = nil
	item := rememberedAccount{TargetKey: targetKey(value.Target), SourceRunID: value.Public.ID, Input: input,
		Public: RememberedAccount{ID: rememberedAccountID(value.Target, row.AccountID), AccountID: row.AccountID, Name: row.Name, Email: row.Email, Plan: row.Plan, LoginAvailable: input.LoginSource != "", LastImportedAt: time.Now().UTC()}}
	key := rememberedAccountKey(value.Target, item.Public.ID)
	raw, revision, err := s.private.WorkbenchDocument(ctx, key)
	if err != nil {
		return err
	}
	if value.RememberedSource != nil && revision != value.RememberedSource.Revision {
		return configstore.ErrWorkbenchVersion
	}
	// Reimporting JSON must not erase the existing password/mailbox login source.
	if len(raw) > 0 && input.LoginSource == "" {
		var previous rememberedAccount
		if decodePrivate(raw, &previous) != nil {
			return errors.New("已记住账号资料无法读取")
		}
		oldWorkspace, oldUser := credentialIdentity(previous.Input.Credentials)
		workspace, user := credentialIdentity(identity)
		if oldWorkspace == workspace && oldUser == user {
			item.Input.LoginSource = previous.Input.LoginSource
			item.Input.LoginPassword = previous.Input.LoginPassword
			item.Input.MailRefreshToken = previous.Input.MailRefreshToken
			item.Input.ProxyURL = previous.Input.ProxyURL
			item.Public.LoginAvailable = item.Input.LoginSource != ""
		}
	}
	raw, err = json.Marshal(item)
	if err != nil {
		return err
	}
	revision, err = s.private.SaveWorkbenchDocument(ctx, key, revision, raw)
	if err == nil && value.RememberedSource != nil {
		value.RememberedSource.Revision = revision
	}
	return err
}

func (s *Service) readRememberedAccount(ctx context.Context, target configstore.TargetSettings, id string) (rememberedAccount, error) {
	var value rememberedAccount
	if len(id) != 64 || strings.Trim(id, "0123456789abcdef") != "" {
		return value, errors.New("已记住账号 ID 无效")
	}
	raw, revision, err := s.private.WorkbenchDocument(ctx, rememberedAccountKey(target, id))
	if err != nil {
		return value, err
	}
	if len(raw) == 0 {
		return value, errors.New("已记住账号不存在或已删除，请刷新列表")
	}
	if decodePrivate(raw, &value) != nil || value.TargetKey != targetKey(target) || value.Public.ID != id || rememberedAccountID(target, value.Public.AccountID) != id {
		return value, errors.New("已记住账号资料无法读取")
	}
	value.Public.Revision = revision
	return value, nil
}

func (s *Service) RememberedAccounts(ctx context.Context, owner string) ([]RememberedAccount, error) {
	if err := s.checkOwner(ctx, owner); err != nil {
		return nil, err
	}
	target, err := targetguard.Settings(ctx, s.private)
	if err != nil {
		return nil, err
	}
	result := []RememberedAccount{}
	err = s.documents(ctx, rememberedAccountPrefix+targetKey(target)+":", func(record configstore.WorkbenchDocumentRecord) error {
		var value struct {
			TargetKey string            `json:"target_key"`
			Public    RememberedAccount `json:"public"`
		}
		if json.Unmarshal(record.Payload, &value) != nil || value.TargetKey != targetKey(target) || value.Public.ID != rememberedAccountID(target, value.Public.AccountID) {
			return errors.New("已记住账号资料无法读取")
		}
		value.Public.Revision = record.Revision
		result = append(result, value.Public)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if _, err = targetguard.Pin(targetguard.Expect(ctx, target), s.private); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) DeleteRememberedAccount(ctx context.Context, owner, id string, revision int64) error {
	if err := s.checkOwner(ctx, owner); err != nil {
		return err
	}
	ctx, release, err := targetguard.Acquire(ctx, s.private)
	if err != nil {
		return err
	}
	defer release()
	s.rememberMu.Lock()
	defer s.rememberMu.Unlock()
	target, err := targetguard.Settings(ctx, s.private)
	if err != nil {
		return err
	}
	value, err := s.readRememberedAccount(ctx, target, id)
	if err != nil {
		return err
	}
	if revision < 1 || value.Public.Revision != revision {
		return configstore.ErrWorkbenchVersion
	}
	return s.private.DeleteWorkbenchDocument(ctx, rememberedAccountKey(target, id), revision)
}
