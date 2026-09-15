package browserlogin_test

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

func TestIsolatedOAuthCheckpointDeletionRevokesActiveAutomaticSession(t *testing.T) {
	ctx, factory := isolatedCheckpointFactory(t, http.HandlerFunc(checkpointLoginPage))
	remote, _ := startCheckpointWorker(t, factory)
	options := automaticCheckpointOptions()
	browser, err := remote.OpenOAuth(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	awaitAutomaticCheckpoint(t, ctx, remote, options, 1)
	ref := automaticCheckpointRef(options)
	wrongOwner := ref
	wrongOwner.Owner = strings.Repeat("f", 64)
	if err := remote.DeleteOAuthCheckpoint(ctx, wrongOwner); !errors.Is(err, browserlogin.ErrOAuthCheckpoint) {
		t.Fatalf("foreign owner revoked an active checkpoint: %v", err)
	}
	if _, err := remote.ReadOAuthCheckpoint(ctx, ref); err != nil {
		t.Fatalf("foreign deletion affected the owned checkpoint: %v", err)
	}
	if err := remote.DeleteOAuthCheckpoint(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if err := browser.Input(ctx, browserlogin.Input{Kind: "text", Text: "123456"}); !errors.Is(err, browserlogin.ErrOAuthCheckpointUnsafe) {
		t.Fatalf("revoked authorization session still accepted input: %v", err)
	}
	if _, err := browser.(browserlogin.OAuthCheckpointBrowser).SuspendOAuth(ctx); !errors.Is(err, browserlogin.ErrOAuthCheckpointUnsafe) {
		t.Fatalf("revoked authorization session recreated a checkpoint: %v", err)
	}
}
