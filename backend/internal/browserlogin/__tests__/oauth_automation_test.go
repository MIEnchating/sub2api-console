package browserlogin_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

func TestOAuthAutomationRejectsInvalidOrUnboundedInputs(t *testing.T) {
	for _, test := range []struct{ stage, value string }{
		{"email", "Display <a@example.com>"}, {"email_code", "1234567"}, {"sms_code", "a12345"},
		{"phone", "123456789"}, {"workspace", "id/other"}, {"password", ""}, {"script", "alert(1)"},
	} {
		t.Run(test.stage+test.value, func(t *testing.T) {
			if err := (browserlogin.AuthAction{Stage: test.stage, Revision: strings.Repeat("a", 64), Value: test.value}).Validate(); err == nil {
				t.Fatal("invalid auth action accepted")
			}
		})
	}
}

func TestIsolatedOAuthAutomationSubmitsRecognizedFormAndRejectsStalePage(t *testing.T) {
	fixture := `<html><body><form action="/continue" onsubmit="event.preventDefault(); if(this.email.value==='owner@example.com'){history.replaceState({},'', '/log-in/password');document.body.innerHTML='<form action=&quot;/verify&quot;><input type=&quot;password&quot; value=&quot;never-expose-this&quot;><button type=&quot;submit&quot;>Continue</button></form>'}"><input type="email" name="email"><button type="submit">Continue</button></form></body></html>`
	ctx, browser, _ := isolatedOAuthBrowser(t, "", false, fixture)
	automation, ok := browser.(browserlogin.OAuthAutomation)
	if !ok {
		t.Fatal("Chromium automation unavailable")
	}
	page, err := automation.InspectAuth(ctx)
	if err != nil || page.Stage != "email" {
		t.Fatalf("email form unrecognized: %+v %v", page, err)
	}
	action := browserlogin.AuthAction{Stage: page.Stage, Revision: page.Revision, Value: "owner@example.com"}
	if err := automation.ApplyAuth(ctx, action); err != nil {
		t.Fatal(err)
	}
	page, err = automation.InspectAuth(ctx)
	if err != nil || page.Stage != "password" {
		t.Fatalf("form not submitted: %+v %v", page, err)
	}
	if err := automation.ApplyAuth(ctx, action); !errors.Is(err, browserlogin.ErrAuthPageChanged) {
		t.Fatalf("stale action replayed: %v", err)
	}
	raw, _ := json.Marshal(page)
	if strings.Contains(string(raw), "never-expose") || strings.Contains(string(raw), "owner@example.com") {
		t.Fatal("form values exposed in page metadata")
	}
}

func TestIsolatedOAuthAutomationLeavesAmbiguousDisabledAndExternalFormsManual(t *testing.T) {
	fixtures := map[string]string{
		"ambiguous":       `<form><input type="email"><input type="email"><button type="submit">Continue</button></form>`,
		"disabled":        `<form><input type="email" disabled><button type="submit">Continue</button></form>`,
		"external_action": `<form action="https://chatgpt.com/capture"><input type="email"><button type="submit">Continue</button></form>`,
		"external_submit": `<form><input type="email"><button type="submit" formaction="https://chatgpt.com/capture">Continue</button></form>`,
		"unrecognized":    `<p>Complete verification</p><iframe title="Verification"></iframe>`,
	}
	for name, fixture := range fixtures {
		t.Run(name, func(t *testing.T) {
			ctx, browser, _ := isolatedOAuthBrowser(t, "", false, "<html><body>"+fixture+"</body></html>")
			page, err := browser.(browserlogin.OAuthAutomation).InspectAuth(ctx)
			if err != nil || page.Stage != "manual" {
				t.Fatalf("unsafe form automated: %+v %v", page, err)
			}
		})
	}
}

func TestIsolatedOAuthAutomationWorkspaceRequiresExactVisibleID(t *testing.T) {
	fixture := `<html><body><script>history.replaceState({},'', '/workspace')</script><form action="/choose" onsubmit="event.preventDefault();history.replaceState({},'', '/done');document.body.textContent='Selected'"><label><input type="radio" name="workspace_id" value="workspace-1">One</label><label><input type="radio" name="workspace_id" value="workspace-2">Two</label><button type="submit">Continue</button></form></body></html>`
	ctx, browser, _ := isolatedOAuthBrowser(t, "", false, fixture)
	automation := browser.(browserlogin.OAuthAutomation)
	page, err := automation.InspectAuth(ctx)
	if err != nil || page.Stage != "workspace" || len(page.WorkspaceIDs) != 2 {
		t.Fatalf("workspace choices missing: %+v %v", page, err)
	}
	if err := automation.ApplyAuth(ctx, browserlogin.AuthAction{Stage: page.Stage, Revision: page.Revision, Value: "missing-id"}); !errors.Is(err, browserlogin.ErrAuthPageChanged) {
		t.Fatalf("missing stable ID selected: %v", err)
	}
	if err := automation.ApplyAuth(ctx, browserlogin.AuthAction{Stage: page.Stage, Revision: page.Revision, Value: "workspace-2"}); err != nil {
		t.Fatal(err)
	}
	page, err = automation.InspectAuth(ctx)
	if err != nil || page.Stage != "manual" {
		t.Fatalf("workspace selection not submitted: %+v %v", page, err)
	}
}
