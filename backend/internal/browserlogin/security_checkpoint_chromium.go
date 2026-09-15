package browserlogin

import (
	"context"
	"net/url"
	"strings"

	"github.com/chromedp/cdproto/fetch"
)

const securityCheckpointBootstrapURL = "https://auth.openai.com/__console_security_checkpoint"

func (f Chromium) OpenSecurityFromCheckpoint(ctx context.Context, options SecurityCheckpointOptions) (SecurityBrowser, error) {
	if err := options.Validate(); err != nil {
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	document, err := f.claimSecurityCheckpoint(options)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithDeadline(ctx, options.Checkpoint.ExpiresAt)
	browser, err := f.openSecurity(ctx, SecurityOptions{ProxyURL: options.Options.ProxyURL}, &document)
	if err != nil {
		cancel()
		return nil, err
	}
	current := browser.(*securityChromium)
	current.mu.Lock()
	current.deadlineCancel = cancel
	current.mu.Unlock()
	if ctx.Err() != nil {
		browser.Close()
		return nil, ctx.Err()
	}
	return browser, nil
}

func (f Chromium) claimSecurityCheckpoint(options SecurityCheckpointOptions) (oauthCheckpointDocument, error) {
	dir, err := openOAuthCheckpointDirectory(f.CheckpointDirectory)
	if err != nil {
		return oauthCheckpointDocument{}, err
	}
	defer dir.close()
	document, err := dir.read(options.Checkpoint.ID)
	if err != nil || validateCheckpointRestore(document, options.restoreOptions()) != nil || document.Meta.Stage != options.Checkpoint.Stage {
		return oauthCheckpointDocument{}, ErrOAuthCheckpoint
	}
	if err := dir.remove(document.Meta.ID); err != nil {
		return oauthCheckpointDocument{}, err
	}
	return document, nil
}

func (b *securityChromium) ConfirmSecurityIdentity(ctx context.Context, expected SecurityIdentity) error {
	if expected.Validate() != nil {
		return ErrSecurityIdentity
	}
	identity, err := b.Identity(ctx)
	if err != nil {
		return err
	}
	if !sameConfirmedSecurityIdentity(identity, expected) {
		return ErrSecurityIdentity
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.confirmedIdentity.UserID != "" && !sameConfirmedSecurityIdentity(b.confirmedIdentity, expected) {
		return ErrSecurityIdentity
	}
	b.confirmedIdentity = identity
	return nil
}

func sameConfirmedSecurityIdentity(left, right SecurityIdentity) bool {
	return left.UserID == right.UserID && strings.EqualFold(left.Email, right.Email)
}

func (b *securityChromium) requireSecurityConfirmation(expected SecurityIdentity) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.confirmationRequired && (b.confirmedIdentity.UserID == "" || !sameConfirmedSecurityIdentity(b.confirmedIdentity, expected)) {
		return ErrSecurityConfirmation
	}
	return nil
}

func (b *securityChromium) allowSecurityRequest(request *fetch.EventRequestPaused) bool {
	if request.Request.Method == "GET" || request.Request.Method == "HEAD" || request.ResponseStatusCode != 0 {
		return true
	}
	u, err := url.Parse(request.Request.URL)
	if err != nil {
		return false
	}
	write := u.Host == "auth.openai.com" && u.Path == "/api/accounts/password/add" || u.Host == "chatgpt.com" && (u.Path == "/backend-api/accounts/mfa/enroll" || u.Path == "/backend-api/accounts/mfa/user/activate_enrollment")
	if !write {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return !b.confirmationRequired || b.confirmedIdentity.UserID != ""
}
