package browserlogin

import (
	"context"
	"errors"
)

var ErrSecurityConfirmation = errors.New("请先核对并确认官方返回的用户 ID 和邮箱，再执行账号安全设置")

// Options is the original transaction, including its original recovery lease.
// SessionID names the new security browser; it never resumes that transaction.
type SecurityCheckpointOptions struct {
	Checkpoint OAuthCheckpoint `json:"checkpoint"`
	SessionID  string          `json:"session_id"`
	Options    OAuthOptions    `json:"options"`
}

func (o SecurityCheckpointOptions) Validate() error {
	if o.Options.Validate() != nil || o.Options.Recovery == nil || len(o.SessionID) != 48 || !validRecoveryToken(o.SessionID) || o.SessionID == o.Checkpoint.ID || o.SessionID == o.Checkpoint.Lease || o.Checkpoint.Revision <= 0 || o.Checkpoint.Stage == "" {
		return ErrOAuthCheckpoint
	}
	binding := o.Options.Recovery
	if binding.Owner != o.Checkpoint.Owner || binding.Lease != o.Checkpoint.Lease || !binding.ExpiresAt.Equal(o.Checkpoint.ExpiresAt) {
		return ErrOAuthCheckpoint
	}
	return o.restoreOptions().validate()
}

func (o SecurityCheckpointOptions) restoreOptions() OAuthRestoreOptions {
	options := o.Options
	if options.Recovery != nil {
		binding := *options.Recovery
		binding.Lease = o.SessionID
		options.Recovery = &binding
	}
	return OAuthRestoreOptions{Checkpoint: OAuthCheckpointRef{ID: o.Checkpoint.ID, Owner: o.Checkpoint.Owner, Lease: o.Checkpoint.Lease}, Lease: o.SessionID, Options: options, Revision: o.Checkpoint.Revision}
}

type SecurityCheckpointFactory interface {
	OpenSecurityFromCheckpoint(context.Context, SecurityCheckpointOptions) (SecurityBrowser, error)
}

// A transferred browser reports its official identity before the operator
// confirms it. No password or MFA write is permitted before confirmation.
type SecurityIdentityConfirmation interface {
	ConfirmSecurityIdentity(context.Context, SecurityIdentity) error
}
