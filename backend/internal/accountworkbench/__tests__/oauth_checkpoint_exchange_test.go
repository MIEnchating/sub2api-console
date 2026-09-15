package accountworkbench_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

func TestAutomaticCheckpointIsRevokedBeforeOneUseExchangeEvenWhenResponseIsLost(t *testing.T) {
	f, factory := automaticCheckpointFixture(t)
	view := startAutomaticCheckpoint(t, f)
	f.factory.mu.Lock()
	options := f.factory.original
	f.factory.mu.Unlock()
	factory.capture(options)
	exchanges := 0
	f.service.UseOAuthTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) {
		exchanges++
		list, err := f.service.OAuthCheckpoints(context.Background(), "checkpoint-owner")
		if err != nil || len(list) != 0 {
			t.Error("authorization code exchange began with a recoverable private transaction")
		}
		return nil, errors.New("isolated lost token response")
	}))
	if err := f.service.InputOAuth(context.Background(), "checkpoint-owner", view.ID, browserlogin.Input{Kind: "key", Key: "Enter"}); err != nil {
		t.Fatal(err)
	}
	if err := f.service.FinishOAuth("checkpoint-owner", view.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-f.completed:
	case <-time.After(5 * time.Second):
		t.Fatal("authorization did not finish after token response loss")
	}
	list, err := f.service.OAuthCheckpoints(context.Background(), "checkpoint-owner")
	if err != nil || len(list) != 0 || exchanges != 1 {
		t.Fatal("uncertain one-use exchange retained a restorable transaction or was replayed")
	}
	if _, err := factory.ReadOAuthCheckpoint(context.Background(), browserlogin.OAuthCheckpointRef{ID: options.Recovery.CheckpointID, Owner: options.Recovery.Owner, Lease: options.Recovery.Lease}); err == nil {
		t.Fatal("uncertain exchange retained the browser checkpoint")
	}
}

func TestAutomaticCheckpointRevocationPersistenceFailurePreventsCodeExchange(t *testing.T) {
	f, factory := automaticCheckpointFixture(t)
	view := startAutomaticCheckpoint(t, f)
	f.factory.mu.Lock()
	options := f.factory.original
	f.factory.mu.Unlock()
	factory.capture(options)
	f.state.failStatus = "deleting"
	if err := f.service.InputOAuth(context.Background(), "checkpoint-owner", view.ID, browserlogin.Input{Kind: "key", Key: "Enter"}); err != nil {
		t.Fatal(err)
	}
	if err := f.service.FinishOAuth("checkpoint-owner", view.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-f.completed:
	case <-time.After(5 * time.Second):
		t.Fatal("authorization did not stop after checkpoint storage failed")
	}
	if f.exchanges.Load() != 0 {
		t.Fatal("authorization code was exchanged before durable checkpoint revocation")
	}
}
