package browserlogin

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/storage"
	"github.com/chromedp/chromedp"
)

type oauthRecoveryRequest struct {
	url      string
	method   string
	main     bool
	status   int64
	mutation uint64
}

type oauthRecoveryState struct {
	mu        sync.Mutex
	op        sync.Mutex
	persist   sync.Mutex
	factory   Chromium
	options   OAuthOptions
	pending   map[network.RequestID]oauthRecoveryRequest
	url       string
	mutation  uint64
	unsafe    bool
	frozen    bool
	paused    bool
	restore   *oauthCheckpointDocument
	revision  int64
	autoSaved bool
}

func newOAuthRecoveryState(f Chromium, options OAuthOptions) *oauthRecoveryState {
	if options.Recovery == nil {
		return nil
	}
	binding := *options.Recovery
	options.Recovery = &binding
	return &oauthRecoveryState{factory: f, options: options, pending: make(map[network.RequestID]oauthRecoveryRequest), unsafe: true}
}

func checkpointPageStage(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "auth.openai.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" {
		return ""
	}
	switch u.Path {
	case "/log-in", "/log-in-or-create-account":
		return "email"
	case "/log-in/password":
		return "password"
	case "/email-verification", "/verify-email":
		return "email_code"
	case "/mfa", "/u/mfa":
		return "totp_code"
	case "/add-phone":
		return "phone"
	case "/phone-verification":
		return "sms_code"
	case "/choose-an-account", "/sign-in-with-chatgpt/codex/consent":
		return "workspace"
	}
	return ""
}

func (b *chromiumBrowser) observeOAuthRecovery(event any) {
	r := b.recovery
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	switch e := event.(type) {
	case *network.EventRequestWillBeSent:
		b.callback.mu.Lock()
		main := e.Type == network.ResourceTypeDocument && e.FrameID == b.callback.frameID
		b.callback.mu.Unlock()
		if e.Request.Method != "GET" && e.Request.Method != "HEAD" {
			r.mutation++
			r.unsafe = true
		}
		if main {
			r.url = ""
		}
		r.pending[e.RequestID] = oauthRecoveryRequest{url: e.Request.URL, method: e.Request.Method, main: main, mutation: r.mutation}
	case *network.EventResponseReceived:
		request, ok := r.pending[e.RequestID]
		if ok {
			request.status = e.Response.Status
			r.pending[e.RequestID] = request
		}
	case *network.EventLoadingFinished:
		request, ok := r.pending[e.RequestID]
		delete(r.pending, e.RequestID)
		if ok && request.main && request.method == "GET" && request.status >= 200 && request.status < 300 && checkpointPageStage(request.url) != "" && request.mutation == r.mutation {
			r.url, r.unsafe = request.url, false
		}
	case *network.EventLoadingFailed:
		request := r.pending[e.RequestID]
		delete(r.pending, e.RequestID)
		if request.main || (request.method != "" && request.method != "GET" && request.method != "HEAD") {
			r.unsafe = true
		}
	}
}

func (b *chromiumBrowser) allowRecoveryRequest(request *fetch.EventRequestPaused) bool {
	r := b.recovery
	if r == nil {
		return true
	}
	r.persist.Lock()
	defer r.persist.Unlock()
	r.mu.Lock()
	if r.frozen {
		r.mu.Unlock()
		return false
	}
	write := request.Request.Method != "GET" && request.Request.Method != "HEAD"
	if r.paused && (write || request.ResourceType == network.ResourceTypeDocument && request.Request.URL != r.restore.URL || isOAuthCallbackURL(request.Request.URL, b.callback.redirectURI)) {
		r.mu.Unlock()
		return false
	}
	r.mu.Unlock()
	if write || request.ResourceType == network.ResourceTypeDocument {
		return r.revokeAutomaticCheckpointLocked() == nil
	}
	return true
}

func (b *chromiumBrowser) checkRecoveryInput(manual bool) error {
	if b.recovery == nil {
		return nil
	}
	r := b.recovery
	r.persist.Lock()
	defer r.persist.Unlock()
	r.mu.Lock()
	if r.frozen {
		r.mu.Unlock()
		return ErrOAuthCheckpointUnsafe
	}
	if r.paused && !manual {
		r.mu.Unlock()
		return ErrOAuthRecoveryPaused
	}
	r.mu.Unlock()
	if err := r.revokeAutomaticCheckpointLocked(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.unsafe = true
	if manual {
		r.paused = false
	}
	return nil
}

func (b *chromiumBrowser) SuspendOAuth(ctx context.Context) (OAuthCheckpoint, error) {
	return b.captureOAuthCheckpoint(ctx, true)
}

func (b *chromiumBrowser) captureOAuthCheckpoint(ctx context.Context, suspend bool) (OAuthCheckpoint, error) {
	r := b.recovery
	if r == nil || r.factory.CheckpointDirectory == "" || ctx.Err() != nil {
		return OAuthCheckpoint{}, ErrOAuthCheckpoint
	}
	r.op.Lock()
	defer r.op.Unlock()
	r.persist.Lock()
	defer r.persist.Unlock()
	if r.autoSaved {
		meta, err := r.factory.ReadOAuthCheckpoint(ctx, r.automaticCheckpointRef())
		if err != nil {
			return OAuthCheckpoint{}, err
		}
		if suspend {
			r.mu.Lock()
			r.frozen = true
			r.mu.Unlock()
			b.Close()
		}
		return meta, nil
	}
	r.mu.Lock()
	if r.frozen || r.unsafe || len(r.pending) != 0 || checkpointPageStage(r.url) == "" || !r.options.Recovery.ExpiresAt.After(time.Now()) {
		r.mu.Unlock()
		return OAuthCheckpoint{}, ErrOAuthCheckpointUnsafe
	}
	r.frozen = true
	savedURL := r.url
	savedMutation := r.mutation
	r.mu.Unlock()
	complete := false
	defer func() {
		if !complete {
			_ = b.run(ctx, page.SetWebLifecycleState(page.SetWebLifecycleStateStateActive))
			r.mu.Lock()
			r.frozen = false
			r.mu.Unlock()
		}
	}()
	b.callback.mu.Lock()
	callbackReady := b.callback.code != "" || b.callback.err != nil
	b.callback.mu.Unlock()
	if callbackReady {
		return OAuthCheckpoint{}, ErrOAuthCheckpointUnsafe
	}
	var state oauthPageStorage
	var cookies []*network.Cookie
	err := b.run(ctx, page.SetWebLifecycleState(page.SetWebLifecycleStateStateFrozen), chromedp.Evaluate(`(() => { const read = storage => { const entries = Object.entries(storage); if (entries.length > 1024 || entries.reduce((size, [key,value]) => size + key.length + value.length, 0) > 524288) throw new Error("checkpoint storage limit"); return Object.fromEntries(entries); }; return {url: location.href, local: read(localStorage), session: read(sessionStorage)}; })()`, &state), chromedp.ActionFunc(func(c context.Context) error {
		var err error
		cookies, err = storage.GetCookies().Do(c)
		return err
	}))
	if err != nil || state.URL != savedURL {
		return OAuthCheckpoint{}, ErrOAuthCheckpointUnsafe
	}
	r.mu.Lock()
	changed := r.unsafe || r.url != savedURL || r.mutation != savedMutation || len(r.pending) != 0
	r.mu.Unlock()
	if changed {
		return OAuthCheckpoint{}, ErrOAuthCheckpointUnsafe
	}
	params, err := checkpointCookies(cookies)
	if err != nil {
		return OAuthCheckpoint{}, err
	}
	document := oauthCheckpointDocument{Version: 1, Meta: OAuthCheckpoint{Owner: r.options.Recovery.Owner, Lease: r.options.Recovery.Lease, ExpiresAt: r.options.Recovery.ExpiresAt, Stage: checkpointPageStage(savedURL)}, Options: r.options, URL: savedURL, Cookies: params, Storage: state}
	if r.options.Recovery.AutoCheckpoint {
		document.Meta.ID = r.options.Recovery.CheckpointID
		document.Meta.Revision = r.revision + 1
	}
	meta, err := r.factory.saveOAuthCheckpoint(document)
	if err != nil {
		return OAuthCheckpoint{}, err
	}
	if r.options.Recovery.AutoCheckpoint {
		r.autoSaved, r.revision = true, meta.Revision
	}
	if suspend {
		complete = true
		b.Close()
	}
	return meta, nil
}

func checkpointCookies(cookies []*network.Cookie) ([]*network.CookieParam, error) {
	result := make([]*network.CookieParam, 0, len(cookies))
	for _, cookie := range cookies {
		domain := strings.TrimPrefix(cookie.Domain, ".")
		if domain != "auth.openai.com" && domain != "openai.com" && domain != "chatgpt.com" {
			continue
		}
		if cookie.PartitionKeyOpaque || cookie.PartitionKey != nil {
			return nil, ErrOAuthCheckpointUnsafe
		}
		param := &network.CookieParam{Name: cookie.Name, Value: cookie.Value, Domain: cookie.Domain, Path: cookie.Path, Secure: cookie.Secure, HTTPOnly: cookie.HTTPOnly, SameSite: cookie.SameSite, Priority: cookie.Priority, SourceScheme: cookie.SourceScheme, SourcePort: cookie.SourcePort}
		if !cookie.Session {
			expires := cdp.TimeSinceEpoch(time.UnixMilli(int64(cookie.Expires * 1000)))
			param.Expires = &expires
		}
		result = append(result, param)
	}
	return result, nil
}

func (f Chromium) RestoreOAuth(ctx context.Context, options OAuthRestoreOptions) (OAuthBrowser, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	document, err := f.claimOAuthCheckpoint(options)
	if err != nil {
		return nil, err
	}
	r := newOAuthRecoveryState(f, options.Options)
	r.paused, r.restore, r.revision = true, &document, document.Meta.Revision
	browser, err := f.openChromium(ctx, "https://auth.openai.com", document.URL, oauthAllowedOrigins(), &oauthCallback{state: options.Options.State, redirectURI: options.Options.RedirectURI}, options.Options.ProxyURL, chromiumSessionState{recovery: r})
	if err != nil {
		return nil, err
	}
	browser.startAutomaticCheckpoints()
	return browser, nil
}

func oauthAllowedOrigins() []string {
	return []string{"https://auth.openai.com", "https://chatgpt.com", "https://cdn.oaistatic.com", "https://images.openai.com", "https://cdn.auth0.com"}
}

func (b *chromiumBrowser) seedOAuthCheckpoint(ctx context.Context) (page.ScriptIdentifier, error) {
	document := b.seed
	if b.recovery != nil {
		document = b.recovery.restore
	}
	if document == nil {
		return "", nil
	}
	if err := storage.SetCookies(document.Cookies).Do(ctx); err != nil {
		return "", err
	}
	state := document.Storage
	if b.seed != nil {
		state.URL = securityCheckpointBootstrapURL
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	return page.AddScriptToEvaluateOnNewDocument(`(() => { const saved = ` + string(raw) + `; if (window.top !== window || location.href !== saved.url) return; for (const [key,value] of Object.entries(saved.local)) localStorage.setItem(key,value); for (const [key,value] of Object.entries(saved.session)) sessionStorage.setItem(key,value); })()`).Do(ctx)
}
