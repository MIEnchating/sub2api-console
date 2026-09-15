package accountworkbench_test

import (
	"context"
	"sync"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

type automaticCheckpointFactory struct {
	base       *checkpointFactory
	mu         sync.Mutex
	latest     map[string]browserlogin.OAuthCheckpoint
	beforeOpen func(browserlogin.OAuthOptions)
	restores   int
}

func automaticCheckpointFixture(t *testing.T) (*checkpointFixture, *automaticCheckpointFactory) {
	t.Helper()
	f := newCheckpointFixture(t)
	factory := &automaticCheckpointFactory{base: f.factory, latest: make(map[string]browserlogin.OAuthCheckpoint)}
	f.service.UseOAuthBrowser(factory)
	return f, factory
}

type automaticCheckpointBrowser struct {
	*checkpointBrowser
	factory *automaticCheckpointFactory
}

func (f *automaticCheckpointFactory) OpenOAuth(ctx context.Context, options browserlogin.OAuthOptions) (browserlogin.OAuthBrowser, error) {
	if f.beforeOpen != nil {
		f.beforeOpen(options)
	}
	browser, err := f.base.OpenOAuth(ctx, options)
	if err != nil {
		return nil, err
	}
	return &automaticCheckpointBrowser{checkpointBrowser: browser.(*checkpointBrowser), factory: f}, nil
}

func (f *automaticCheckpointFactory) capture(options browserlogin.OAuthOptions) browserlogin.OAuthCheckpoint {
	f.mu.Lock()
	defer f.mu.Unlock()
	prior := f.latest[options.Recovery.CheckpointID]
	meta := browserlogin.OAuthCheckpoint{ID: options.Recovery.CheckpointID, Owner: options.Recovery.Owner, Lease: options.Recovery.Lease, Stage: "email_code", ExpiresAt: options.Recovery.ExpiresAt, Revision: prior.Revision + 1}
	f.latest[meta.ID] = meta
	return meta
}

func (f *automaticCheckpointFactory) ReadOAuthCheckpoint(_ context.Context, ref browserlogin.OAuthCheckpointRef) (browserlogin.OAuthCheckpoint, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	meta, ok := f.latest[ref.ID]
	if !ok || meta.Owner != ref.Owner || meta.Lease != ref.Lease {
		return browserlogin.OAuthCheckpoint{}, browserlogin.ErrOAuthCheckpoint
	}
	return meta, nil
}

func (f *automaticCheckpointFactory) DeleteOAuthCheckpoint(_ context.Context, ref browserlogin.OAuthCheckpointRef) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	meta, ok := f.latest[ref.ID]
	if !ok || meta.Owner != ref.Owner || meta.Lease != ref.Lease {
		return browserlogin.ErrOAuthCheckpoint
	}
	delete(f.latest, ref.ID)
	return nil
}

func (f *automaticCheckpointFactory) RestoreOAuth(ctx context.Context, options browserlogin.OAuthRestoreOptions) (browserlogin.OAuthBrowser, error) {
	f.mu.Lock()
	meta, ok := f.latest[options.Checkpoint.ID]
	if !ok || meta.Owner != options.Checkpoint.Owner || meta.Lease != options.Checkpoint.Lease || meta.Revision != options.Revision || options.Options.Recovery.CheckpointID != meta.ID || options.Options.Recovery.Lease != options.Lease || !options.Options.Recovery.AutoCheckpoint || !options.Options.Recovery.ExpiresAt.Equal(meta.ExpiresAt) {
		f.mu.Unlock()
		return nil, browserlogin.ErrOAuthCheckpoint
	}
	delete(f.latest, meta.ID)
	f.restores++
	f.mu.Unlock()
	return f.OpenOAuth(ctx, options.Options)
}

func (b *automaticCheckpointBrowser) Close() {
	if b.options.Recovery != nil && b.options.Recovery.AutoCheckpoint {
		_ = b.factory.DeleteOAuthCheckpoint(context.Background(), browserlogin.OAuthCheckpointRef{ID: b.options.Recovery.CheckpointID, Owner: b.options.Recovery.Owner, Lease: b.options.Recovery.Lease})
	}
	b.checkpointBrowser.Close()
}

func (b *automaticCheckpointBrowser) ClosePreservingOAuthCheckpoint() { b.checkpointBrowser.Close() }

func (b *automaticCheckpointBrowser) SuspendOAuth(context.Context) (browserlogin.OAuthCheckpoint, error) {
	meta := b.factory.capture(b.options)
	b.checkpointBrowser.Close()
	return meta, nil
}

func startAutomaticCheckpoint(t *testing.T, f *checkpointFixture) accountworkbench.OAuthView {
	t.Helper()
	view, err := f.service.StartOAuthWithInput(context.Background(), "checkpoint-owner", accountworkbench.OAuthStartInput{RecoveryEnabled: true})
	if err != nil || !view.RecoveryEnabled || view.CheckpointID == "" {
		t.Fatalf("automatic OAuth start = %+v, %v", view, err)
	}
	f.active = append(f.active, view.ID)
	f.phase(t, "waiting")
	return view
}
