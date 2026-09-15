package browserlogin

import (
	"context"
	"time"
)

func (b *chromiumBrowser) ClosePreservingOAuthCheckpoint() { b.Close() }

func (r *oauthRecoveryState) automaticCheckpointRef() OAuthCheckpointRef {
	return OAuthCheckpointRef{ID: r.options.Recovery.CheckpointID, Owner: r.options.Recovery.Owner, Lease: r.options.Recovery.Lease}
}

// The persist lock serializes durable invalidation with capture. Fetch handlers
// never take op: a browser input may be waiting for that request to continue.
func (r *oauthRecoveryState) revokeAutomaticCheckpointLocked() error {
	if !r.options.Recovery.AutoCheckpoint {
		return nil
	}
	if err := r.factory.DeleteOAuthCheckpoint(context.Background(), r.automaticCheckpointRef()); err != nil {
		return err
	}
	r.autoSaved = false
	return nil
}

func (b *chromiumBrowser) startAutomaticCheckpoints() {
	if b.recovery == nil || !b.recovery.options.Recovery.AutoCheckpoint {
		return
	}
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			ctx, cancel := context.WithTimeout(b.ctx, 3*time.Second)
			_, _ = b.captureOAuthCheckpoint(ctx, false)
			cancel()
			select {
			case <-b.ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (b *chromiumBrowser) discardAutomaticCheckpoint() error {
	r := b.recovery
	if r == nil {
		return nil
	}
	r.op.Lock()
	defer r.op.Unlock()
	r.persist.Lock()
	defer r.persist.Unlock()
	if err := r.revokeAutomaticCheckpointLocked(); err != nil {
		return err
	}
	r.mu.Lock()
	r.frozen = true
	r.mu.Unlock()
	return nil
}

func (b *chromiumBrowser) freezeForCheckpointRestore(ctx context.Context, options OAuthRestoreOptions) error {
	r := b.recovery
	if r == nil || !r.options.Recovery.AutoCheckpoint || options.Checkpoint != r.automaticCheckpointRef() {
		return ErrOAuthCheckpoint
	}
	r.op.Lock()
	defer r.op.Unlock()
	r.persist.Lock()
	defer r.persist.Unlock()
	if ctx.Err() != nil || !r.autoSaved {
		return ErrOAuthCheckpoint
	}
	dir, err := openOAuthCheckpointDirectory(r.factory.CheckpointDirectory)
	if err != nil {
		return err
	}
	defer dir.close()
	document, err := dir.read(options.Checkpoint.ID)
	if err != nil || validateCheckpointRestore(document, options) != nil {
		return ErrOAuthCheckpoint
	}
	r.mu.Lock()
	r.frozen = true
	r.mu.Unlock()
	return nil
}
