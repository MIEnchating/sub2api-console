package browserlogin

import (
	"context"
	"encoding/hex"
	"errors"
	"time"
)

var ErrOAuthCheckpoint = errors.New("登录检查点不存在、已过期或会话绑定已变化，请重新授权")
var ErrOAuthCheckpointUnsafe = errors.New("当前登录步骤无法安全保存，请等待官方页面完成或重新授权")
var ErrOAuthRecoveryPaused = errors.New("恢复后的登录需要先人工接管，自动提交尚未启用")

type OAuthRecoveryBinding struct {
	Owner          string    `json:"owner"`
	Lease          string    `json:"lease"`
	ExpiresAt      time.Time `json:"expires_at"`
	AutoCheckpoint bool      `json:"auto_checkpoint,omitempty"`
	CheckpointID   string    `json:"checkpoint_id,omitempty"`
}

type OAuthCheckpointRef struct {
	ID    string `json:"id"`
	Owner string `json:"owner"`
	Lease string `json:"lease"`
}

// Checkpoints expose only opaque references and a bounded stage. Cookies and
// browser storage never cross the worker socket.
type OAuthCheckpoint struct {
	ID        string    `json:"id"`
	Owner     string    `json:"owner"`
	Lease     string    `json:"lease"`
	Stage     string    `json:"stage"`
	ExpiresAt time.Time `json:"expires_at"`
	Revision  int64     `json:"revision,omitempty"`
}

type OAuthRestoreOptions struct {
	Checkpoint OAuthCheckpointRef `json:"checkpoint"`
	Lease      string             `json:"lease"`
	Options    OAuthOptions       `json:"options"`
	Revision   int64              `json:"revision,omitempty"`
}

type OAuthCheckpointBrowser interface {
	SuspendOAuth(context.Context) (OAuthCheckpoint, error)
}

type OAuthRecoveryFactory interface {
	RestoreOAuth(context.Context, OAuthRestoreOptions) (OAuthBrowser, error)
	DeleteOAuthCheckpoint(context.Context, OAuthCheckpointRef) error
}

type OAuthCheckpointReader interface {
	ReadOAuthCheckpoint(context.Context, OAuthCheckpointRef) (OAuthCheckpoint, error)
}

type OAuthCheckpointPreserver interface {
	ClosePreservingOAuthCheckpoint()
}

func validRecoveryToken(value string) bool {
	if len(value) < 32 || len(value) > 128 || len(value)%2 != 0 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func (b OAuthRecoveryBinding) validate() error {
	if !validRecoveryToken(b.Owner) || !validRecoveryToken(b.Lease) || !b.ExpiresAt.After(time.Now()) || b.ExpiresAt.After(time.Now().Add(Lifetime)) {
		return ErrOAuthCheckpoint
	}
	if b.AutoCheckpoint && (len(b.CheckpointID) != 48 || !validRecoveryToken(b.CheckpointID)) || !b.AutoCheckpoint && b.CheckpointID != "" {
		return ErrOAuthCheckpoint
	}
	return nil
}

func (r OAuthCheckpointRef) validate() error {
	if len(r.ID) != 48 || !validRecoveryToken(r.ID) || !validRecoveryToken(r.Owner) || !validRecoveryToken(r.Lease) {
		return ErrOAuthCheckpoint
	}
	return nil
}

func (o OAuthRestoreOptions) validate() error {
	if o.Checkpoint.validate() != nil || o.Options.Validate() != nil || o.Options.Recovery == nil || !validRecoveryToken(o.Lease) || o.Lease == o.Checkpoint.Lease || o.Options.Recovery.Lease != o.Lease || o.Options.Recovery.Owner != o.Checkpoint.Owner {
		return ErrOAuthCheckpoint
	}
	return nil
}
