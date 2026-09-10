package uptimekuma

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestTemplatesPreserveCredentialsRejectStaleEditsAndApplyPrivately(t *testing.T) {
	f := resourceFixture(t)
	c := f.configure(t, true)
	ctx := context.Background()
	item, err := f.service.SaveTemplate(ctx, "", TemplateInput{Name: "JSON 探测", Method: "POST", Headers: `{"X-Key":"private-template-header"}`, Body: `{"token":"private-template-body"}`, AuthMethod: "bearer", AuthPassword: "private-template-token"})
	if err != nil {
		t.Fatal(err)
	}
	list, err := f.service.Templates(ctx)
	if err != nil || len(list) != 1 {
		t.Fatal(err)
	}
	data, _ := json.Marshal(list)
	if strings.Contains(string(data), "private-template") {
		t.Fatal("template response leaked credentials")
	}
	_, err = f.service.SaveTemplate(ctx, item.ID, TemplateInput{Revision: 0, Name: "stale", Method: "GET", AuthMethod: "none"})
	if PublicError(err).Code != "kuma_template_conflict" {
		t.Fatal("stale write accepted")
	}
	item, err = f.service.SaveTemplate(ctx, item.ID, TemplateInput{Revision: item.Revision, Name: "新名称", Method: "POST", AuthMethod: "bearer"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.service.Write(ctx, 0, WriteInput{Action: "create", ConfigRevision: c.Revision, Monitor: MonitorInput{Name: "API", Type: "http", URL: "https://monitor.example", Interval: 60, TemplateID: item.ID, TemplateRevision: item.Revision}})
	if err != nil {
		t.Fatal(err)
	}
	if rawString(f.edited, "headers") != `{"X-Key":"private-template-header"}` || rawString(f.edited, "body") != `{"token":"private-template-body"}` || rawString(f.edited, "bearer_token") != "private-template-token" {
		t.Fatal("template was not applied privately")
	}
	if err = f.service.DeleteTemplate(ctx, item.ID, item.Revision-1); PublicError(err).Code != "kuma_template_conflict" {
		t.Fatal("stale delete accepted")
	}
	if err = f.service.DeleteTemplate(ctx, item.ID, item.Revision); err != nil {
		t.Fatal(err)
	}
	_, err = f.service.Write(ctx, 0, WriteInput{Action: "create", ConfigRevision: c.Revision, Monitor: MonitorInput{Name: "API", Type: "http", URL: "https://monitor.example", Interval: 60, TemplateID: item.ID, TemplateRevision: item.Revision}})
	if PublicError(err).Code != "kuma_template_conflict" {
		t.Fatal("deleted template accepted")
	}
}

func TestTemplateValidationRejectsBadHeadersAndMissingCredentials(t *testing.T) {
	f := resourceFixture(t)
	for _, in := range []TemplateInput{{Name: "坏请求头", Method: "GET", Headers: "[]", AuthMethod: "none"}, {Name: "空鉴权", Method: "GET", AuthMethod: "bearer"}} {
		if _, err := f.service.SaveTemplate(context.Background(), "", in); err == nil {
			t.Fatal("invalid template accepted")
		}
	}
}

func TestTemplateAuthenticationOverrideReplacesOnlyRequestAuthentication(t *testing.T) {
	for _, method := range []string{"none", "basic", "bearer"} {
		t.Run(method, func(t *testing.T) {
			f := resourceFixture(t)
			c := f.configure(t, true)
			ctx := context.Background()
			item, err := f.service.SaveTemplate(ctx, "", TemplateInput{Name: "JSON", Method: "POST", Headers: `{"X-App":"health","Authorization":"Bearer template-header"}`, Body: `{"ping":true}`, AuthMethod: "bearer", AuthPassword: "template-token"})
			if err != nil {
				t.Fatal(err)
			}
			var in MonitorInput
			if err = json.Unmarshal([]byte(`{"name":"API","type":"http","url":"https://monitor.example","interval":60,"template_auth_override":true}`), &in); err != nil {
				t.Fatal(err)
			}
			in.TemplateID, in.TemplateRevision = item.ID, item.Revision
			in.Options = monitorOptions(nil)
			in.Options.AuthMethod, in.Options.AuthUsername, in.Options.AuthPassword = method, "monitor-user", "monitor-token"
			_, err = f.service.Write(ctx, 0, WriteInput{Action: "create", ConfigRevision: c.Revision, Monitor: in})
			if err != nil {
				t.Fatal(err)
			}
			if rawString(f.edited, "authMethod") != method {
				t.Fatal("template replaced the monitor authentication override")
			}
			var headers map[string]string
			if err = json.Unmarshal([]byte(rawString(f.edited, "headers")), &headers); err != nil {
				t.Fatal(err)
			}
			if len(headers) != 1 || headers["X-App"] != "health" || rawString(f.edited, "body") != `{"ping":true}` || rawString(f.edited, "method") != "POST" {
				t.Fatal("override must remove template Authorization but preserve other request settings")
			}
			if method == "basic" && (rawString(f.edited, "basic_auth_user") != "monitor-user" || rawString(f.edited, "basic_auth_pass") != "monitor-token") {
				t.Fatal("Basic override missing")
			}
			if method == "bearer" && rawString(f.edited, "bearer_token") != "monitor-token" {
				t.Fatal("Bearer override missing")
			}
			if method == "none" && (rawString(f.edited, "bearer_token") != "" || rawString(f.edited, "basic_auth_pass") != "") {
				t.Fatal("no authentication retained credentials")
			}
			stored, err := f.service.store.KumaTemplate(ctx, item.ID)
			if err != nil || stored.AuthPassword != "template-token" {
				t.Fatal("monitor override mutated template")
			}
		})
	}
}

func TestTemplateAuthenticationOverrideRejectsIncompleteCredentials(t *testing.T) {
	f := resourceFixture(t)
	c := f.configure(t, true)
	ctx := context.Background()
	item, err := f.service.SaveTemplate(ctx, "", TemplateInput{Name: "JSON", Method: "POST", AuthMethod: "bearer", AuthPassword: "template-token"})
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"basic", "bearer", "unsupported"} {
		var in MonitorInput
		if err = json.Unmarshal([]byte(`{"name":"API","type":"http","url":"https://monitor.example","interval":60,"template_auth_override":true}`), &in); err != nil {
			t.Fatal(err)
		}
		in.TemplateID, in.TemplateRevision = item.ID, item.Revision
		in.Options = monitorOptions(nil)
		in.Options.AuthMethod = method
		_, err = f.service.Write(ctx, 0, WriteInput{Action: "create", ConfigRevision: c.Revision, Monitor: in})
		if err == nil || PublicError(err).Code != "kuma_invalid_auth" {
			t.Fatalf("%s: incomplete override accepted: %v", method, err)
		}
	}
}

func TestApplyingTemplateWithoutOptionsPreservesExistingMonitorSettings(t *testing.T) {
	f := resourceFixture(t)
	c := f.configure(t, true)
	ctx := context.Background()
	f.monitors["19"] = json.RawMessage(`{"id":19,"name":"API","type":"http","url":"https://monitor.example","active":true,"interval":300,"timeout":45,"retryInterval":120,"maxretries":4,"notificationIDList":{"7":true}}`)
	monitor, _, err := decodeMonitor(f.monitors["19"])
	if err != nil {
		t.Fatal(err)
	}
	template, err := f.service.SaveTemplate(ctx, "", TemplateInput{Name: "请求模板", Method: "POST", AuthMethod: "none"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.service.Write(ctx, 19, WriteInput{Action: "edit", ConfigRevision: c.Revision, Revision: monitor.Revision, Monitor: MonitorInput{Name: monitor.Name, Type: monitor.Type, Interval: monitor.Interval, TemplateID: template.ID, TemplateRevision: template.Revision}})
	if err != nil {
		t.Fatal(err)
	}
	if rawInt(f.edited, "timeout", 0) != 45 || rawInt(f.edited, "retryInterval", 0) != 120 || rawInt(f.edited, "maxretries", 0) != 4 || string(f.edited["notificationIDList"]) != `{"7":true}` {
		t.Fatal("applying a request template overwrote unrelated monitoring settings")
	}
}

func TestTemplateAuthOverrideEditClearsOldCredentialsAndKeepsMonitorSettings(t *testing.T) {
	f := resourceFixture(t)
	c := f.configure(t, true)
	ctx := context.Background()
	f.monitors["19"] = json.RawMessage(`{"id":19,"name":"API","type":"http","url":"https://monitor.example","active":true,"interval":300,"timeout":45,"retryInterval":120,"maxretries":4,"authMethod":"basic","basic_auth_user":"old-user","basic_auth_pass":"old-pass","notificationIDList":{"7":true}}`)
	monitor, _, err := decodeMonitor(f.monitors["19"])
	if err != nil {
		t.Fatal(err)
	}
	item, err := f.service.SaveTemplate(ctx, "", TemplateInput{Name: "JSON", Method: "POST", AuthMethod: "none"})
	if err != nil {
		t.Fatal(err)
	}
	options := monitor.Options
	options.AuthMethod, options.AuthPassword = "bearer", "new-monitor-token"
	_, err = f.service.Write(ctx, 19, WriteInput{Action: "edit", ConfigRevision: c.Revision, Revision: monitor.Revision, Monitor: MonitorInput{Name: monitor.Name, Type: monitor.Type, Interval: monitor.Interval, Options: options, TemplateID: item.ID, TemplateRevision: item.Revision, TemplateAuthOverride: true}})
	if err != nil {
		t.Fatal(err)
	}
	if rawString(f.edited, "bearer_token") != "new-monitor-token" || rawString(f.edited, "basic_auth_user") != "" || rawString(f.edited, "basic_auth_pass") != "" {
		t.Fatal("authentication replacement retained old credentials")
	}
	if rawInt(f.edited, "timeout", 0) != 45 || string(f.edited["notificationIDList"]) != `{"7":true}` {
		t.Fatal("authentication override changed monitor settings")
	}
}

func TestClearedTemplateFieldsReplaceExistingMonitorRequestSettings(t *testing.T) {
	f := resourceFixture(t)
	c := f.configure(t, true)
	ctx := context.Background()
	template, err := f.service.SaveTemplate(ctx, "", TemplateInput{Name: "请求模板", Method: "POST", Headers: `{"X-Key":"test"}`, Body: "test-body", AuthMethod: "bearer", AuthPassword: "test-token"})
	if err != nil {
		t.Fatal(err)
	}
	template, err = f.service.SaveTemplate(ctx, template.ID, TemplateInput{Revision: template.Revision, Name: template.Name, Method: "GET", ClearHeaders: true, ClearBody: true, AuthMethod: "none"})
	if err != nil {
		t.Fatal(err)
	}
	monitor, _, err := decodeMonitor(f.monitors["19"])
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.service.Write(ctx, 19, WriteInput{Action: "edit", ConfigRevision: c.Revision, Revision: monitor.Revision, Monitor: MonitorInput{Name: monitor.Name, Type: monitor.Type, Interval: monitor.Interval, Options: monitor.Options, TemplateID: template.ID, TemplateRevision: template.Revision}})
	if err != nil {
		t.Fatal(err)
	}
	if rawString(f.edited, "headers") != "" || rawString(f.edited, "body") != "" || rawString(f.edited, "bearer_token") != "" || rawString(f.edited, "basic_auth_pass") != "" {
		t.Fatal("empty template retained old request credentials")
	}
}
