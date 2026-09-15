package browserlogin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

func (w *worker) openSecurityCheckpoint(r *http.Request) (wireResponse, error) {
	factory, ok := w.factory.(SecurityCheckpointFactory)
	if !ok || r.Method != http.MethodPost {
		return wireResponse{}, ErrOAuthCheckpoint
	}
	var options SecurityCheckpointOptions
	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 32768))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&options) != nil || options.Validate() != nil {
		return wireResponse{}, ErrOAuthCheckpoint
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.securityID != "" {
		return wireResponse{}, errors.New("验证浏览器正在使用中")
	}
	if w.oauthID != "" {
		current, ok := w.oauth.(*chromiumBrowser)
		if !ok || current.freezeForSecurityCheckpoint(r.Context(), options) != nil {
			return wireResponse{}, ErrOAuthCheckpoint
		}
		w.closeLocked()
	}
	ctx, cancel := context.WithDeadline(w.ctx, options.Checkpoint.ExpiresAt)
	stop := context.AfterFunc(r.Context(), cancel)
	browser, err := factory.OpenSecurityFromCheckpoint(ctx, options)
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
	w.securityID, w.security, w.cancel = options.SessionID, browser, cancel
	go func() { <-ctx.Done(); w.close(options.SessionID) }()
	return wireResponse{ID: options.SessionID}, nil
}

func (b *chromiumBrowser) freezeForSecurityCheckpoint(ctx context.Context, options SecurityCheckpointOptions) error {
	if options.Validate() != nil || b.recovery == nil {
		return ErrOAuthCheckpoint
	}
	meta, err := b.recovery.factory.ReadOAuthCheckpoint(ctx, options.restoreOptions().Checkpoint)
	if err != nil || meta.Stage != options.Checkpoint.Stage || meta.Revision != options.Checkpoint.Revision || !meta.ExpiresAt.Equal(options.Checkpoint.ExpiresAt) {
		return ErrOAuthCheckpoint
	}
	return b.freezeForCheckpointRestore(ctx, options.restoreOptions())
}
