package configstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"modernc.org/sqlite"
)

const workbenchSourceProfileSchema = `CREATE TABLE IF NOT EXISTS workbench_source_profiles (
 id TEXT PRIMARY KEY, owner_hash TEXT NOT NULL, scope TEXT NOT NULL CHECK(scope='local-export'),
 user_id TEXT NOT NULL, workspace_id TEXT NOT NULL, email TEXT NOT NULL,
 revision INTEGER NOT NULL CHECK(revision>0), updated_at TEXT NOT NULL,
 has_password INTEGER NOT NULL, has_totp INTEGER NOT NULL, has_proxy INTEGER NOT NULL,
 mail_kind TEXT NOT NULL, sms_provider TEXT NOT NULL, login BLOB NOT NULL,
 UNIQUE(owner_hash,scope,user_id,workspace_id)
)`

type WorkbenchSourceProfile struct {
	ID, OwnerHash, Scope, UserID, WorkspaceID, Email string
	Revision                                         int64
	UpdatedAt                                        string
	HasPassword, HasTOTP, HasProxy                   bool
	MailKind, SMSProvider                            string
	Login                                            json.RawMessage `json:"-"`
}

var ErrWorkbenchSourceProfile = errors.New("本地登录资料已变化、删除或不属于当前会话，请刷新后重新确认")

const sourceProfileColumns = `id,owner_hash,scope,user_id,workspace_id,email,revision,updated_at,has_password,has_totp,has_proxy,mail_kind,sms_provider`

func scanSourceProfile(row interface{ Scan(...any) error }, private bool) (WorkbenchSourceProfile, error) {
	var value WorkbenchSourceProfile
	fields := []any{&value.ID, &value.OwnerHash, &value.Scope, &value.UserID, &value.WorkspaceID, &value.Email, &value.Revision, &value.UpdatedAt, &value.HasPassword, &value.HasTOTP, &value.HasProxy, &value.MailKind, &value.SMSProvider}
	if private {
		fields = append(fields, &value.Login)
	}
	err := row.Scan(fields...)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrWorkbenchSourceProfile
	}
	return value, err
}

func (s *Store) WorkbenchSourceProfiles(ctx context.Context, owner, scope string) ([]WorkbenchSourceProfile, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+sourceProfileColumns+` FROM workbench_source_profiles WHERE owner_hash=? AND scope=? ORDER BY updated_at DESC,id`, owner, scope)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []WorkbenchSourceProfile{}
	for rows.Next() {
		value, err := scanSourceProfile(rows, false)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *Store) WorkbenchSourceProfile(ctx context.Context, owner, scope, id string) (WorkbenchSourceProfile, error) {
	return scanSourceProfile(s.db.QueryRowContext(ctx, `SELECT `+sourceProfileColumns+`,login FROM workbench_source_profiles WHERE owner_hash=? AND scope=? AND id=?`, owner, scope, id), true)
}

func (s *Store) SaveWorkbenchSourceProfile(ctx context.Context, value WorkbenchSourceProfile) (WorkbenchSourceProfile, error) {
	if !workbenchIdentifier.MatchString(value.ID) || len(value.OwnerHash) != 64 || strings.Trim(value.OwnerHash, "0123456789abcdef") != "" || value.Scope != "local-export" || value.UserID == "" || value.WorkspaceID == "" || value.Email == "" || value.Revision < 0 || value.Revision >= 1<<53-1 || len(value.Login) == 0 || len(value.Login) > 2<<20 || !json.Valid(value.Login) {
		return WorkbenchSourceProfile{}, ErrWorkbenchSourceProfile
	}
	previous := value.Revision
	value.Revision++
	value.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `INSERT INTO workbench_source_profiles(`+sourceProfileColumns+`,login)
 SELECT ?,?,?,?,?,?,?,?,?,?,?,?,?,? WHERE ?=0 OR EXISTS(SELECT 1 FROM workbench_source_profiles WHERE id=?)
 ON CONFLICT(id) DO UPDATE SET revision=excluded.revision,updated_at=excluded.updated_at,has_password=excluded.has_password,has_totp=excluded.has_totp,has_proxy=excluded.has_proxy,mail_kind=excluded.mail_kind,sms_provider=excluded.sms_provider,login=excluded.login
 WHERE workbench_source_profiles.revision=? AND workbench_source_profiles.owner_hash=excluded.owner_hash AND workbench_source_profiles.scope=excluded.scope AND workbench_source_profiles.user_id=excluded.user_id AND workbench_source_profiles.workspace_id=excluded.workspace_id AND lower(workbench_source_profiles.email)=lower(excluded.email)`, value.ID, value.OwnerHash, value.Scope, value.UserID, value.WorkspaceID, value.Email, value.Revision, value.UpdatedAt, value.HasPassword, value.HasTOTP, value.HasProxy, value.MailKind, value.SMSProvider, []byte(value.Login), previous, value.ID, previous)
	if err != nil {
		var sqliteError *sqlite.Error
		if errors.As(err, &sqliteError) && sqliteError.Code() == 2067 {
			return WorkbenchSourceProfile{}, ErrWorkbenchSourceProfile
		}
		return WorkbenchSourceProfile{}, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return WorkbenchSourceProfile{}, err
	}
	if count != 1 {
		return WorkbenchSourceProfile{}, ErrWorkbenchSourceProfile
	}
	value.Login = nil
	return value, nil
}

func (s *Store) DeleteWorkbenchSourceProfile(ctx context.Context, owner, scope, id string, revision int64) error {
	if !workbenchIdentifier.MatchString(id) || revision < 1 {
		return ErrWorkbenchSourceProfile
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM workbench_source_profiles WHERE owner_hash=? AND scope=? AND id=? AND revision=?`, owner, scope, id, revision)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrWorkbenchSourceProfile
	}
	return nil
}
