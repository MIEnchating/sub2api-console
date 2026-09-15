package browserlogin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

type securityChromium struct {
	*chromiumBrowser
	mu                                                    sync.Mutex
	email                                                 string
	passwordIdentity                                      SecurityIdentity
	passwordStarted, passwordSubmitted, passwordSucceeded bool
	passwordRequest                                       fetch.RequestID
	frameID                                               cdp.FrameID
	enrolled, activated                                   bool
	enrollmentSession                                     string
	confirmationRequired                                  bool
	confirmedIdentity                                     SecurityIdentity
	deadlineCancel                                        context.CancelFunc
}

func (b *securityChromium) Close() {
	b.mu.Lock()
	cancel := b.deadlineCancel
	b.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	b.chromiumBrowser.Close()
}

func (f Chromium) OpenSecurity(ctx context.Context, options SecurityOptions) (SecurityBrowser, error) {
	if err := options.Validate(); err != nil {
		return nil, err
	}
	return f.openSecurity(ctx, options, nil)
}

func (f Chromium) openSecurity(ctx context.Context, options SecurityOptions, seed *oauthCheckpointDocument) (SecurityBrowser, error) {
	b := &securityChromium{email: options.Email, confirmationRequired: seed != nil}
	navigateURL := "https://chatgpt.com/"
	if seed != nil {
		navigateURL = securityCheckpointBootstrapURL
	}
	base, err := f.openChromium(ctx, "https://chatgpt.com", navigateURL, []string{
		"https://chatgpt.com", "https://auth.openai.com", "https://cdn.oaistatic.com", "https://images.openai.com", "https://cdn.auth0.com", "https://static.cloudflareinsights.com",
	}, nil, options.ProxyURL, chromiumSessionState{security: b, seed: seed})
	if err != nil {
		return nil, err
	}
	b.chromiumBrowser = base
	if err := b.run(ctx, fetch.Enable().WithPatterns([]*fetch.RequestPattern{
		{URLPattern: "https://auth.openai.com/api/accounts/password/add", RequestStage: fetch.RequestStageResponse},
		{URLPattern: "*", RequestStage: fetch.RequestStageRequest},
	}), chromedp.ActionFunc(func(c context.Context) error {
		tree, err := page.GetFrameTree().Do(c)
		if err == nil {
			b.mu.Lock()
			b.frameID = tree.Frame.ID
			b.mu.Unlock()
		}
		return err
	})); err != nil {
		b.Close()
		return nil, errors.New("账号安全浏览器启动失败")
	}
	if seed != nil {
		if err := b.run(ctx, chromedp.Navigate("https://chatgpt.com/")); err != nil {
			b.Close()
			return nil, errors.New("账号安全浏览器启动失败")
		}
		if _, err := b.Identity(ctx); err == nil {
			return b, nil
		} else if !errors.Is(err, ErrSecurityPending) {
			b.Close()
			return nil, err
		}
	}
	if err := b.startWebLogin(ctx, false); err != nil {
		b.Close()
		return nil, err
	}
	return b, nil
}

func (b *securityChromium) evaluate(ctx context.Context, script string, result any) error {
	return b.run(ctx, chromedp.Evaluate(script, result, func(p *runtime.EvaluateParams) *runtime.EvaluateParams { return p.WithAwaitPromise(true) }))
}

// Fixed same-origin requests keep web tokens and cookies inside Chromium.
const securitySessionScript = `
  if(location.origin !== "https://chatgpt.com") return {state:"pending"};
  const read = async (path, options = {}) => {
    const response = await fetch(path, {...options, credentials:"same-origin", cache:"no-store", redirect:"error", signal:AbortSignal.timeout(10000)});
    if (!response.ok) throw new Error("request");
    const text = await response.text();
    if (text.length > 65536) throw new Error("size");
    return JSON.parse(text);
  };
  const session = await read("/api/auth/session");
  if (!session || !session.user || typeof session.user.id !== "string" || typeof session.user.email !== "string" || !session.user.id || !session.user.email) return {state:"pending"};
  const identity = {user_id:session.user.id, email:session.user.email};
`

type securityScriptResult struct {
	State      string             `json:"state"`
	Identity   SecurityIdentity   `json:"identity"`
	Enabled    bool               `json:"enabled"`
	Enrollment SecurityEnrollment `json:"enrollment"`
}

func (b *securityChromium) readSecurity(ctx context.Context, body string) (securityScriptResult, error) {
	var result securityScriptResult
	err := b.evaluate(ctx, `(async()=>{try{`+securitySessionScript+body+`}catch{return {state:"failed"}}})()`, &result)
	if err != nil {
		return securityScriptResult{}, ErrSecurityUncertain
	}
	switch result.State {
	case "ok":
		return result, nil
	case "pending":
		return securityScriptResult{}, ErrSecurityPending
	case "identity":
		return securityScriptResult{}, ErrSecurityIdentity
	default:
		return securityScriptResult{}, ErrSecurityUncertain
	}
}

func (b *securityChromium) Identity(ctx context.Context) (SecurityIdentity, error) {
	result, err := b.readSecurity(ctx, `return {state:"ok", identity};`)
	if err != nil {
		return SecurityIdentity{}, err
	}
	if err := result.Identity.Validate(); err != nil || b.email != "" && !strings.EqualFold(result.Identity.Email, b.email) {
		return SecurityIdentity{}, ErrSecurityIdentity
	}
	return result.Identity, nil
}

func (b *securityChromium) startWebLogin(ctx context.Context, password bool) error {
	email := b.email
	if email == "" {
		b.mu.Lock()
		email = b.confirmedIdentity.Email
		b.mu.Unlock()
	}
	parameters := map[string]string{"login_hint": email, "prompt": "login", "screen_hint": "login"}
	if password {
		parameters = map[string]string{"login_hint": email, "connection": "password", "reauth": "password", "post_login_add_password": "true", "max_age": "0"}
	}
	raw, _ := json.Marshal(parameters)
	var ok bool
	err := b.evaluate(ctx, `(async()=>{try{
  if(location.origin!=="https://chatgpt.com") return false;
  const read = async (path, options={}) => {
    const r=await fetch(path,{...options,credentials:"same-origin",cache:"no-store",redirect:"error",signal:AbortSignal.timeout(10000)});
    if(!r.ok) throw new Error("request");
    const text=await r.text(); if(text.length>65536) throw new Error("size"); return JSON.parse(text);
  };
  const csrf=await read("/api/auth/csrf");
  if(typeof csrf.csrfToken!=="string" || !csrf.csrfToken) return false;
  const params = new URLSearchParams(`+string(raw)+`);
  const device=document.cookie.split(";").map(v=>v.trim()).find(v=>v.startsWith("oai-did="));
  if(device) params.set("ext-oai-did",decodeURIComponent(device.slice(8)));
  const data=await read("/api/auth/signin/openai?"+params, {method:"POST",headers:{"Content-Type":"application/x-www-form-urlencoded"},body:new URLSearchParams({callbackUrl:"https://chatgpt.com/",csrfToken:csrf.csrfToken,json:"true"}).toString()});
  if(typeof data.url!=="string") return false;
  const url=new URL(data.url);
  if(url.origin!=="https://auth.openai.com" || url.username || url.password || url.hash) return false;
  location.assign(url.href); return true;
}catch{return false}})()`, &ok)
	if err != nil || !ok {
		return errors.New("官方网页登录未能启动，请核对网络与官方登录服务后重新创建任务")
	}
	return nil
}

func (b *securityChromium) interactivePage(ctx context.Context) (bool, error) {
	var allowed bool
	err := b.evaluate(ctx, `document.readyState !== "loading" && location.origin === "https://auth.openai.com" && /^\/(log-in(?:\/password)?|log-in-or-create-account|email-verification|verify-email|mfa(?:\/.*)?|u\/mfa(?:\/.*)?|add-phone|phone-verification|reset-password\/new-password|authorize|oauth\/authorize)(\/|$)/.test(location.pathname)`, &allowed)
	return allowed, err
}

func (b *securityChromium) Screenshot(ctx context.Context) ([]byte, error) {
	before, err := b.securityFrame(ctx)
	if err != nil {
		return nil, err
	}
	allowed, err := b.interactivePage(ctx)
	if err != nil || !allowed {
		return nil, err
	}
	image, err := b.chromiumBrowser.Screenshot(ctx)
	if err != nil {
		return nil, err
	}
	after, err := b.securityFrame(ctx)
	if err != nil {
		return nil, err
	}
	// Navigation may commit between eligibility and CDP screenshot capture.
	// Discard the bytes if the document or URL changed, including same-document
	// history navigation, and verify eligibility again after capture.
	if before.ID != after.ID || before.LoaderID != after.LoaderID || before.URL != after.URL || before.URLFragment != after.URLFragment {
		return nil, nil
	}
	allowed, err = b.interactivePage(ctx)
	if err != nil || !allowed {
		return nil, err
	}
	return image, nil
}

func (b *securityChromium) securityFrame(ctx context.Context) (*cdp.Frame, error) {
	var frame *cdp.Frame
	err := b.run(ctx, chromedp.ActionFunc(func(c context.Context) error {
		tree, err := page.GetFrameTree().Do(c)
		if err != nil {
			return err
		}
		if tree == nil || tree.Frame == nil || tree.Frame.ID == "" || tree.Frame.LoaderID == "" {
			return ErrAuthPageChanged
		}
		frame = tree.Frame
		return nil
	}))
	return frame, err
}

func (b *securityChromium) Input(ctx context.Context, value Input) error {
	allowed, err := b.interactivePage(ctx)
	if err != nil || !allowed {
		return ErrAuthPageChanged
	}
	form, _, err := b.authForm(ctx)
	if err != nil || form.Stage == "new_password" {
		return ErrAuthPageChanged
	}
	return b.chromiumBrowser.Input(ctx, value)
}

func (b *securityChromium) BeginPassword(ctx context.Context, expected SecurityIdentity) error {
	if err := b.requireSecurityConfirmation(expected); err != nil {
		return err
	}
	if err := expected.Validate(); err != nil || b.email != "" && !strings.EqualFold(expected.Email, b.email) {
		return ErrSecurityIdentity
	}
	identity, err := b.Identity(ctx)
	if err != nil || identity.UserID != expected.UserID || !strings.EqualFold(identity.Email, expected.Email) {
		return ErrSecurityIdentity
	}
	b.mu.Lock()
	if b.passwordStarted {
		b.mu.Unlock()
		return ErrSecurityUncertain
	}
	b.passwordStarted = true
	b.passwordIdentity = expected
	b.mu.Unlock()
	return b.startWebLogin(ctx, true)
}

func (b *securityChromium) SubmitPassword(ctx context.Context, action AuthAction) error {
	if action.Stage != "new_password" || action.Validate() != nil {
		return ErrAuthPageChanged
	}
	b.mu.Lock()
	if !b.passwordStarted || b.passwordSubmitted {
		b.mu.Unlock()
		return ErrSecurityUncertain
	}
	b.passwordSubmitted = true
	b.mu.Unlock()
	form, revision, err := b.authForm(ctx)
	if err != nil || form.Stage != "new_password" || revision != action.Revision {
		return ErrAuthPageChanged
	}
	value, _ := json.Marshal(action.Value)
	expected, _ := json.Marshal(form)
	script := `(() => {
  if(JSON.stringify((` + authFormSnapshot + `)())!==JSON.stringify(` + string(expected) + `)) return false;
  const visible=e=>!e.disabled&&!e.readOnly&&e.getClientRects().length>0&&getComputedStyle(e).visibility!=="hidden";
  const fields=Array.from(document.querySelectorAll('input[type="password"]')).filter(visible);
  if(location.origin!=="https://auth.openai.com"||location.pathname!=="/reset-password/new-password"||fields.length<1||fields.length>2) return false;
  const form=fields[0].form; if(!form||fields.some(e=>e.form!==form)) return false;
  const submits=Array.from(form.querySelectorAll('button[type="submit"],input[type="submit"],button:not([type])')).filter(visible);
  if(submits.length!==1) return false;
  const value=` + string(value) + `;
  for(const field of fields){Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,"value").set.call(field,value);field.dispatchEvent(new Event("input",{bubbles:true}));field.dispatchEvent(new Event("change",{bubbles:true}));}
  if(!form.checkValidity()) return false; form.requestSubmit(submits[0]); return true;
})()`
	var applied bool
	if err := b.evaluate(ctx, script, &applied); err != nil || !applied {
		return ErrAuthPageChanged
	}
	return nil
}

func (b *securityChromium) capturePasswordResponse(ctx context.Context, event *fetch.EventRequestPaused) bool {
	endpoint, err := url.Parse(event.Request.URL)
	if err != nil || endpoint.Hostname() != "auth.openai.com" || endpoint.Path != "/api/accounts/password/add" {
		return false
	}
	if event.ResponseStatusCode == 0 {
		b.mu.Lock()
		allowed := b.passwordStarted && b.passwordSubmitted && b.passwordRequest == "" && event.Request.Method == http.MethodPost && event.FrameID == b.frameID && event.Request.URL == "https://auth.openai.com/api/accounts/password/add"
		if allowed {
			b.passwordRequest = event.RequestID
		}
		b.mu.Unlock()
		if !allowed {
			_ = fetch.FailRequest(event.RequestID, network.ErrorReasonBlockedByClient).Do(ctx)
			return true
		}
		_ = fetch.ContinueRequest(event.RequestID).WithInterceptResponse(true).Do(ctx)
		return true
	}
	defer func() { _ = fetch.ContinueResponse(event.RequestID).Do(ctx) }()
	b.mu.Lock()
	valid := b.passwordRequest == event.RequestID && event.Request.Method == http.MethodPost && event.FrameID == b.frameID && event.ResponseStatusCode == 200
	b.mu.Unlock()
	if !valid {
		return true
	}
	// Inspect the fixed response while paused, before the official page can
	// navigate away and Chromium evicts its response body.
	body, err := fetch.GetResponseBody(event.RequestID).Do(ctx)
	if err != nil || len(body) > 65536 {
		return true
	}
	var payload struct {
		Page struct {
			Type string `json:"type"`
		} `json:"page"`
		ContinueURL string          `json:"continue_url"`
		Error       json.RawMessage `json:"error"`
		Success     *bool           `json:"success"`
	}
	if json.Unmarshal(body, &payload) != nil || payload.Page.Type != "external_url" && payload.ContinueURL == "" {
		return true
	}
	if len(payload.Error) != 0 && string(payload.Error) != "null" || payload.Success != nil && !*payload.Success {
		return true
	}
	if payload.ContinueURL != "" {
		target, err := url.Parse(payload.ContinueURL)
		if err != nil || target.Scheme != "https" || target.Host != "chatgpt.com" || target.User != nil || target.Fragment != "" {
			return true
		}
	}
	b.mu.Lock()
	b.passwordSucceeded = true
	b.mu.Unlock()
	return true
}

func (b *securityChromium) PasswordResult(ctx context.Context) (bool, error) {
	b.mu.Lock()
	succeeded := b.passwordSucceeded
	expected := b.passwordIdentity
	b.mu.Unlock()
	if !succeeded {
		return false, ErrSecurityPending
	}
	// The success response is not sufficient: wait for the web app to return and
	// verify the same stable identity before reporting a completed operation.
	identity, err := b.Identity(ctx)
	if err != nil {
		return false, ErrSecurityPending
	}
	if identity.UserID != expected.UserID || !strings.EqualFold(identity.Email, expected.Email) {
		return false, ErrSecurityIdentity
	}
	return true, nil
}

func (b *securityChromium) TOTPEnabled(ctx context.Context, expected SecurityIdentity) (bool, error) {
	if err := expected.Validate(); err != nil {
		return false, err
	}
	result, err := b.readSecurity(ctx, securityMFAScript(expected)+`
  const info=await mfa("/backend-api/accounts/mfa_info");
  if(!info || typeof info.mfa_enabled_v2!=="boolean" || (info.mfa_enabled_v2 && (!info.factors || !Array.isArray(info.factors.totp)))) throw new Error("status");
  return {state:"ok",enabled:Boolean(info&&info.mfa_enabled_v2&&info.factors&&Array.isArray(info.factors.totp)&&info.factors.totp.some(v=>v&& (v.factor_type==="totp"||v.id)))};`)
	return result.Enabled, err
}

func securityMFAScript(expected SecurityIdentity) string {
	raw, _ := json.Marshal(expected)
	return `
  const expected=` + string(raw) + `;
  if(identity.user_id!==expected.user_id || identity.email.toLowerCase()!==expected.email.toLowerCase()) return {state:"identity"};
  if(typeof session.accessToken!=="string" || session.accessToken.length<20 || session.accessToken.length>32768) throw new Error("session");
  const device=document.cookie.split(";").map(v=>v.trim()).find(v=>v.startsWith("oai-did="));
  const mfa=(path,body)=>read(path,{method:body?"POST":"GET",headers:{authorization:"Bearer "+session.accessToken,"Content-Type":"application/json","oai-device-id":device?decodeURIComponent(device.slice(8)):"","oai-session-id":crypto.randomUUID(),"oai-language":"zh-CN","x-openai-target-path":path,"x-openai-target-route":path},...(body?{body:JSON.stringify(body)}:{})});
`
}

func (b *securityChromium) EnrollTOTP(ctx context.Context, expected SecurityIdentity) (SecurityEnrollment, error) {
	if err := b.requireSecurityConfirmation(expected); err != nil {
		return SecurityEnrollment{}, err
	}
	if err := expected.Validate(); err != nil {
		return SecurityEnrollment{}, err
	}
	b.mu.Lock()
	if b.enrolled {
		b.mu.Unlock()
		return SecurityEnrollment{}, ErrSecurityUncertain
	}
	b.enrolled = true
	b.mu.Unlock()
	result, err := b.readSecurity(ctx, securityMFAScript(expected)+`
  const value=await mfa("/backend-api/accounts/mfa/enroll",{factor_type:"totp"});
  if(!value||typeof value.secret!=="string"||!/^[A-Z2-7]{16,128}$/.test(value.secret.replace(/[\s=]/g,"").toUpperCase())||typeof value.session_id!=="string"||!value.session_id||value.session_id.length>256) throw new Error("enrollment");
  return {state:"ok",enrollment:{secret:value.secret.replace(/[\s=]/g,"").toUpperCase(),session_id:value.session_id}};`)
	if err != nil {
		return SecurityEnrollment{}, err
	}
	b.mu.Lock()
	b.enrollmentSession = result.Enrollment.SessionID
	b.mu.Unlock()
	return result.Enrollment, nil
}

func (b *securityChromium) ActivateTOTP(ctx context.Context, expected SecurityIdentity, enrollment SecurityEnrollment, code string) error {
	if err := b.requireSecurityConfirmation(expected); err != nil {
		return err
	}
	if err := expected.Validate(); err != nil || len(code) != 6 || strings.Trim(code, "0123456789") != "" {
		return ErrSecurityIdentity
	}
	b.mu.Lock()
	if !b.enrolled || b.activated || enrollment.SessionID == "" || enrollment.SessionID != b.enrollmentSession {
		b.mu.Unlock()
		return ErrSecurityUncertain
	}
	b.activated = true
	b.mu.Unlock()
	payload, _ := json.Marshal(map[string]string{"code": code, "factor_type": "totp", "session_id": enrollment.SessionID})
	_, err := b.readSecurity(ctx, securityMFAScript(expected)+`
  const value=await mfa("/backend-api/accounts/mfa/user/activate_enrollment",`+string(payload)+`);
  if(!value||value.success!==true) throw new Error("activation"); return {state:"ok"};`)
	return err
}
