package browserlogin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
)

func (w *worker) suspendOAuth(ctx context.Context, id string) (wireResponse, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.oauthID != id || w.oauth == nil {
		return wireResponse{}, ErrSession
	}
	checkpoint, ok := w.oauth.(OAuthCheckpointBrowser)
	if !ok {
		return wireResponse{}, ErrOAuthCheckpoint
	}
	meta, err := checkpoint.SuspendOAuth(ctx)
	if err != nil {
		return wireResponse{}, err
	}
	w.closeLocked()
	return wireResponse{Checkpoint: &meta}, nil
}

func (w *worker) handleOAuthRecovery(r *http.Request) (wireResponse, error) {
	factory, ok := w.factory.(OAuthRecoveryFactory)
	if !ok || r.Method != http.MethodPost {
		return wireResponse{}, ErrOAuthCheckpoint
	}
	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 32768))
	decoder.DisallowUnknownFields()
	if r.URL.Path == "/oauth/checkpoints/read" {
		reader, ok := w.factory.(OAuthCheckpointReader)
		var ref OAuthCheckpointRef
		if !ok || decoder.Decode(&ref) != nil || ref.validate() != nil {
			return wireResponse{}, ErrOAuthCheckpoint
		}
		meta, err := reader.ReadOAuthCheckpoint(r.Context(), ref)
		return wireResponse{Checkpoint: &meta}, err
	}
	if r.URL.Path == "/oauth/checkpoints/delete" {
		var ref OAuthCheckpointRef
		if decoder.Decode(&ref) != nil || ref.validate() != nil {
			return wireResponse{}, ErrOAuthCheckpoint
		}
		w.mu.Lock()
		defer w.mu.Unlock()
		if current, ok := w.oauth.(*chromiumBrowser); ok && current.recovery != nil &&
			current.recovery.options.Recovery.AutoCheckpoint && ref == current.recovery.automaticCheckpointRef() {
			return wireResponse{}, current.discardAutomaticCheckpoint()
		}
		return wireResponse{}, factory.DeleteOAuthCheckpoint(r.Context(), ref)
	}
	if r.URL.Path != "/oauth/checkpoints/restore" {
		return wireResponse{}, ErrOAuthCheckpoint
	}
	var options OAuthRestoreOptions
	if decoder.Decode(&options) != nil || options.validate() != nil {
		return wireResponse{}, ErrOAuthCheckpoint
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.securityID != "" {
		return wireResponse{}, errors.New("验证浏览器正在使用中")
	}
	if w.oauthID != "" {
		current, ok := w.oauth.(*chromiumBrowser)
		if !ok || current.freezeForCheckpointRestore(r.Context(), options) != nil {
			return wireResponse{}, ErrOAuthCheckpoint
		}
		w.closeLocked()
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return wireResponse{}, ErrOAuthCheckpoint
	}
	id := hex.EncodeToString(raw)
	ctx, cancel := context.WithDeadline(w.ctx, options.Options.Recovery.ExpiresAt)
	stop := context.AfterFunc(r.Context(), cancel)
	browser, err := factory.RestoreOAuth(ctx, options)
	stop()
	if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		err = r.Context().Err()
	}
	if err != nil || browser == nil {
		cancel()
		if browser != nil {
			browser.Close()
		}
		if err == nil {
			err = ErrOAuthCheckpoint
		}
		return wireResponse{}, err
	}
	w.oauthID, w.oauth, w.cancel = id, browser, cancel
	go func() { <-ctx.Done(); w.close(id) }()
	return wireResponse{ID: id}, nil
}
