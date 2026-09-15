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

// Login is private at both storage and domain boundaries. Listing metadata
// selects no login payload; marshaling this record never returns credentials.
type WorkbenchLoginProfile struct {
	ID                string
	TargetURL         string
	TargetFingerprint string
	AccountID         string
	UserID            string
	WorkspaceID       string
	Email             string
	Revision          int64
	UpdatedAt         string
	HasPassword       bool
	HasTOTP           bool
	HasProxy          bool
	MailKind          string
	SMSProvider       string
	Login             json.RawMessage `json:"-"`
}

var ErrWorkbenchLoginProfile = errors.New("登录资料已修改、删除或绑定身份不一致，请刷新后重新确认")

const profileColumns = `id,target_url,target_fingerprint,account_id,user_id,workspace_id,email,revision,updated_at,has_password,has_totp,mail_kind,sms_provider,has_proxy`

func scanLoginProfile(row interface{ Scan(...any) error }, private bool) (WorkbenchLoginProfile, error) {
	var value WorkbenchLoginProfile
	fields := []any{&value.ID, &value.TargetURL, &value.TargetFingerprint, &value.AccountID, &value.UserID, &value.WorkspaceID, &value.Email, &value.Revision, &value.UpdatedAt, &value.HasPassword, &value.HasTOTP, &value.MailKind, &value.SMSProvider, &value.HasProxy}
	if private {
		fields = append(fields, &value.Login)
	}
	err := row.Scan(fields...)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrWorkbenchLoginProfile
	}
	return value, err
}

func (s *Store) WorkbenchLoginProfiles(ctx context.Context, targetFingerprint string) ([]WorkbenchLoginProfile, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+profileColumns+` FROM workbench_login_profiles WHERE target_fingerprint=? ORDER BY account_id,id`, targetFingerprint)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []WorkbenchLoginProfile{}
	for rows.Next() {
		value, err := scanLoginProfile(rows, false)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *Store) WorkbenchLoginProfile(ctx context.Context, targetFingerprint, id string) (WorkbenchLoginProfile, error) {
	return scanLoginProfile(s.db.QueryRowContext(ctx, `SELECT `+profileColumns+`,login FROM workbench_login_profiles WHERE target_fingerprint=? AND id=?`, targetFingerprint, id), true)
}

func (s *Store) SaveWorkbenchLoginProfile(ctx context.Context, value WorkbenchLoginProfile) (WorkbenchLoginProfile, error) {
	if !workbenchIdentifier.MatchString(value.ID) || value.TargetURL == "" || len(value.TargetFingerprint) != 64 || strings.Trim(value.TargetFingerprint, "0123456789abcdef") != "" || value.AccountID == "" || value.UserID == "" || value.WorkspaceID == "" || value.Email == "" || value.Revision < 0 || value.Revision >= 1<<53-1 || len(value.Login) == 0 || len(value.Login) > 2<<20 || !json.Valid(value.Login) {
		return WorkbenchLoginProfile{}, ErrWorkbenchLoginProfile
	}
	previous := value.Revision
	value.Revision++
	value.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	// A single conditional write avoids read-then-write races and never updates
	// a different target/account/official identity under an existing profile ID.
	result, err := s.db.ExecContext(ctx, `INSERT INTO workbench_login_profiles(`+profileColumns+`,login)
 SELECT ?,?,?,?,?,?,?,?,?,?,?,?,?,?,? WHERE ?=0 OR EXISTS(SELECT 1 FROM workbench_login_profiles WHERE id=?)
 ON CONFLICT(id) DO UPDATE SET revision=excluded.revision,updated_at=excluded.updated_at,has_password=excluded.has_password,has_totp=excluded.has_totp,mail_kind=excluded.mail_kind,sms_provider=excluded.sms_provider,has_proxy=excluded.has_proxy,login=excluded.login
 WHERE workbench_login_profiles.revision=? AND workbench_login_profiles.target_fingerprint=excluded.target_fingerprint AND workbench_login_profiles.target_url=excluded.target_url AND workbench_login_profiles.account_id=excluded.account_id AND workbench_login_profiles.user_id=excluded.user_id AND workbench_login_profiles.workspace_id=excluded.workspace_id AND lower(workbench_login_profiles.email)=lower(excluded.email)`, value.ID, value.TargetURL, value.TargetFingerprint, value.AccountID, value.UserID, value.WorkspaceID, value.Email, value.Revision, value.UpdatedAt, value.HasPassword, value.HasTOTP, value.MailKind, value.SMSProvider, value.HasProxy, []byte(value.Login), previous, value.ID, previous)
	if err != nil {
		var sqliteError *sqlite.Error
		if errors.As(err, &sqliteError) && sqliteError.Code() == 2067 {
			return WorkbenchLoginProfile{}, ErrWorkbenchLoginProfile
		}
		return WorkbenchLoginProfile{}, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return WorkbenchLoginProfile{}, err
	}
	if count != 1 {
		return WorkbenchLoginProfile{}, ErrWorkbenchLoginProfile
	}
	value.Login = nil
	return value, nil
}

func (s *Store) DeleteWorkbenchLoginProfile(ctx context.Context, targetFingerprint, id string, revision int64) error {
	if !workbenchIdentifier.MatchString(id) || revision < 1 {
		return ErrWorkbenchLoginProfile
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM workbench_login_profiles WHERE target_fingerprint=? AND id=? AND revision=?`, targetFingerprint, id, revision)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrWorkbenchLoginProfile
	}
	return nil
}
