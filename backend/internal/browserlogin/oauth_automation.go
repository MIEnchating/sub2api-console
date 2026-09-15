package browserlogin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/mail"
	"regexp"
	"strings"

	"github.com/chromedp/chromedp"
)

var ErrAuthPageChanged = errors.New("官方授权页面已变化或尚未就绪，请刷新画面后重试")

// OAuthAutomation is available only over the private browser socket. It never
// exposes DOM text, field values, page URLs, cookies, or arbitrary selectors.
type OAuthAutomation interface {
	InspectAuth(context.Context) (AuthPage, error)
	ApplyAuth(context.Context, AuthAction) error
}

type AuthPage struct {
	Stage        string   `json:"stage"`
	Revision     string   `json:"revision"`
	WorkspaceIDs []string `json:"workspace_ids,omitempty"`
}

type AuthAction struct {
	Stage    string `json:"stage"`
	Revision string `json:"revision"`
	Value    string `json:"value"`
}

var authCodePattern = regexp.MustCompile(`^[0-9]{6}$`)
var authPhonePattern = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)
var authWorkspacePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,160}$`)

func (action AuthAction) Validate() error {
	if len(action.Revision) != 64 || len(action.Value) > 4096 {
		return ErrAuthPageChanged
	}
	if _, err := hex.DecodeString(action.Revision); err != nil {
		return ErrAuthPageChanged
	}
	switch action.Stage {
	case "email":
		parsed, err := mail.ParseAddress(action.Value)
		if err == nil && parsed.Address == action.Value && len(action.Value) <= 320 {
			return nil
		}
	case "password":
		if len(action.Value) > 0 && !strings.ContainsAny(action.Value, "\r\n\x00") {
			return nil
		}
	case "new_password":
		if len(action.Value) >= 12 && len(action.Value) <= 128 && !strings.ContainsAny(action.Value, "\r\n\x00") {
			return nil
		}
	case "email_code", "totp_code", "sms_code":
		if authCodePattern.MatchString(action.Value) {
			return nil
		}
	case "phone":
		if authPhonePattern.MatchString(action.Value) {
			return nil
		}
	case "workspace":
		if authWorkspacePattern.MatchString(action.Value) {
			return nil
		}
	}
	return errors.New("授权辅助输入不符合当前步骤要求")
}

// This fixed script reads only the shape of visible official forms. The same
// snapshot is checked again in the single fill-and-submit browser operation.
const authFormSnapshot = `() => {
  const empty = {stage:"manual", path:"", selector:"", action:"", workspaces:[]};
  if (location.origin !== "https://auth.openai.com") return empty;
  const path = location.pathname;
  const visible = (e) => !e.disabled && !e.readOnly && e.getClientRects().length > 0 && getComputedStyle(e).visibility !== "hidden";
  const trusted = (form) => {
    if (!form) return false;
    const u = new URL(form.action || location.href, location.href);
    return u.origin === location.origin && !u.username && !u.password;
  };
  const choices = Array.from(document.querySelectorAll('input[name="workspace_id"][type="radio"], button[name="workspace_id"]')).filter(e => visible(e) && trusted(e.form));
  if (path.includes("workspace") && choices.length > 0 && choices.length <= 100) {
    const ids = choices.map(e => e.value);
    if (ids.every(v => /^[a-zA-Z0-9_-]{1,160}$/.test(v)) && new Set(ids).size === ids.length) {
      return {stage:"workspace", path, selector:'[name="workspace_id"]', action:choices[0].form.action, workspaces:ids};
    }
  }
  let stage = "manual", selector = "";
  if (/\/(email-verification|verify-email)(\/|$)/.test(path)) { stage="email_code"; selector='input[name="code"], input[autocomplete="one-time-code"]'; }
  else if (/\/(mfa|u\/mfa)(\/|$)/.test(path)) { stage="totp_code"; selector='input[name="code"], input[autocomplete="one-time-code"]'; }
  else if (/\/(add-phone|phone-verification)(\/|$)/.test(path)) {
    if (Array.from(document.querySelectorAll('input[name="code"], input[autocomplete="one-time-code"]')).some(visible)) {
      stage="sms_code"; selector='input[name="code"], input[autocomplete="one-time-code"]';
    } else { stage="phone"; selector='input[type="tel"], input[name="phone_number"]'; }
  }
  else if (path === "/log-in/password") { stage="password"; selector='input[type="password"]'; }
  else if (path === "/reset-password/new-password") { stage="new_password"; selector='input[type="password"]'; }
  else if (["/log-in", "/log-in-or-create-account", "/oauth/authorize", "/authorize"].includes(path)) { stage="email"; selector='input[type="email"], input[name="email"], input[autocomplete="username"]'; }
  if (stage === "manual") return empty;
  const fields = Array.from(document.querySelectorAll(selector)).filter(visible);
  if (fields.length < 1 || fields.length > (stage === "new_password" ? 2 : 1) || !trusted(fields[0].form) || fields.some(e => e.form !== fields[0].form)) return empty;
  const form = fields[0].form;
  const submits = Array.from(form.querySelectorAll('button[type="submit"], input[type="submit"], button:not([type])')).filter(visible);
  if (submits.length !== 1 || (submits[0].formAction && new URL(submits[0].formAction).origin !== location.origin)) return empty;
  return {stage, path, selector, action:form.action, workspaces:[]};
}`

type authForm struct {
	Stage      string   `json:"stage"`
	Path       string   `json:"path"`
	Selector   string   `json:"selector"`
	Action     string   `json:"action"`
	Workspaces []string `json:"workspaces"`
}

func (b *chromiumBrowser) authForm(ctx context.Context) (authForm, string, error) {
	if (b.callback == nil && !b.securityAuth) || b.ctx.Err() != nil {
		return authForm{}, "", ErrSession
	}
	var form authForm
	if err := b.run(ctx, chromedp.Evaluate("("+authFormSnapshot+")()", &form)); err != nil {
		return authForm{}, "", ErrAuthPageChanged
	}
	raw, err := json.Marshal(form)
	if err != nil {
		return authForm{}, "", ErrAuthPageChanged
	}
	digest := sha256.Sum256(raw)
	return form, hex.EncodeToString(digest[:]), nil
}

func (b *chromiumBrowser) InspectAuth(ctx context.Context) (AuthPage, error) {
	form, revision, err := b.authForm(ctx)
	if err != nil {
		return AuthPage{}, err
	}
	return AuthPage{Stage: form.Stage, Revision: revision, WorkspaceIDs: form.Workspaces}, nil
}

func (b *chromiumBrowser) ApplyAuth(ctx context.Context, action AuthAction) error {
	if b.recovery != nil {
		b.recovery.op.Lock()
		defer b.recovery.op.Unlock()
	}
	if err := b.checkRecoveryInput(false); err != nil {
		return err
	}
	if err := action.Validate(); err != nil {
		return err
	}
	form, revision, err := b.authForm(ctx)
	if err != nil || action.Stage != form.Stage || action.Revision != revision {
		return ErrAuthPageChanged
	}
	expected, _ := json.Marshal(form)
	value, _ := json.Marshal(action.Value)
	script := `(() => {
  const snapshot = (` + authFormSnapshot + `)();
  const expected = ` + string(expected) + `;
  if (JSON.stringify(snapshot) !== JSON.stringify(expected)) return false;
  const value = ` + string(value) + `;
  const visible = e => !e.disabled && !e.readOnly && e.getClientRects().length > 0 && getComputedStyle(e).visibility !== "hidden";
  if (snapshot.stage === "workspace") {
    const options = Array.from(document.querySelectorAll(snapshot.selector)).filter(e => visible(e) && e.value === value);
    if (options.length !== 1 || !options[0].form || new URL(options[0].form.action).origin !== location.origin) return false;
    const option = options[0];
    if (option.tagName === "BUTTON") { option.click(); return true; }
    const submits = Array.from(option.form.querySelectorAll('button[type="submit"], input[type="submit"], button:not([type])')).filter(visible);
    if (submits.length !== 1 || (submits[0].formAction && new URL(submits[0].formAction).origin !== location.origin)) return false;
    option.checked = true;
    option.dispatchEvent(new Event("change", {bubbles:true}));
    option.form.requestSubmit(submits[0]);
    return true;
  }
  const fields = Array.from(document.querySelectorAll(snapshot.selector)).filter(visible);
  if (fields.length !== 1 || !fields[0].form) return false;
  const field = fields[0], form = field.form;
  const submits = Array.from(form.querySelectorAll('button[type="submit"], input[type="submit"], button:not([type])')).filter(visible);
  if (submits.length !== 1) return false;
  Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,"value").set.call(field, value);
  field.dispatchEvent(new Event("input", {bubbles:true}));
  field.dispatchEvent(new Event("change", {bubbles:true}));
  if (!form.checkValidity()) return false;
  form.requestSubmit(submits[0]);
  return true;
})()`
	var applied bool
	if err := b.run(ctx, chromedp.Evaluate(script, &applied)); err != nil || !applied {
		return ErrAuthPageChanged
	}
	return nil
}
