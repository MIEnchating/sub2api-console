package browserlogin

import (
	"context"
	"net/http"
	"time"
)

func (b *remoteOAuthBrowser) ClosePreservingOAuthCheckpoint() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _ = b.remote.call(ctx, http.MethodPost, "/oauth/sessions/"+b.id+"/detach", nil)
}

func (b *remoteOAuthBrowser) SuspendOAuth(ctx context.Context) (OAuthCheckpoint, error) {
	response, err := b.remote.call(ctx, http.MethodPost, "/oauth/sessions/"+b.id+"/suspend", nil)
	if err != nil {
		return OAuthCheckpoint{}, err
	}
	if response.Checkpoint == nil {
		return OAuthCheckpoint{}, ErrOAuthCheckpoint
	}
	meta := *response.Checkpoint
	if (OAuthCheckpointRef{ID: meta.ID, Owner: meta.Owner, Lease: meta.Lease}).validate() != nil || !meta.ExpiresAt.After(time.Now()) || meta.Stage == "" {
		return OAuthCheckpoint{}, ErrOAuthCheckpoint
	}
	return meta, nil
}

func (r *Remote) RestoreOAuth(ctx context.Context, options OAuthRestoreOptions) (OAuthBrowser, error) {
	if options.validate() != nil {
		return nil, ErrOAuthCheckpoint
	}
	response, err := r.call(ctx, http.MethodPost, "/oauth/checkpoints/restore", options)
	if err != nil {
		return nil, err
	}
	if len(response.ID) != 48 || !validRecoveryToken(response.ID) {
		return nil, ErrOAuthCheckpoint
	}
	return &remoteOAuthBrowser{remote: r, id: response.ID, state: options.Options.State}, nil
}

func (r *Remote) DeleteOAuthCheckpoint(ctx context.Context, ref OAuthCheckpointRef) error {
	if ref.validate() != nil {
		return ErrOAuthCheckpoint
	}
	_, err := r.call(ctx, http.MethodPost, "/oauth/checkpoints/delete", ref)
	return err
}

func (r *Remote) ReadOAuthCheckpoint(ctx context.Context, ref OAuthCheckpointRef) (OAuthCheckpoint, error) {
	if ref.validate() != nil {
		return OAuthCheckpoint{}, ErrOAuthCheckpoint
	}
	response, err := r.call(ctx, http.MethodPost, "/oauth/checkpoints/read", ref)
	if err != nil {
		return OAuthCheckpoint{}, err
	}
	meta := response.Checkpoint
	if meta == nil || meta.ID != ref.ID || meta.Owner != ref.Owner || meta.Lease != ref.Lease || !meta.ExpiresAt.After(time.Now()) || meta.Revision <= 0 {
		return OAuthCheckpoint{}, ErrOAuthCheckpoint
	}
	return *meta, nil
}
