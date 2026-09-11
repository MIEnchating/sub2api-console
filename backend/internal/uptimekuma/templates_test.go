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

func TestMonitorTemplateReadbackRetainsBindingAfterTemplateDeletionAndNormalEdit(t *testing.T) {
	f := resourceFixture(t)
	cfg := f.configure(t, true)
	ctx := context.Background()
	item, err := f.service.SaveTemplate(ctx, "", TemplateInput{Name: "原模板", Method: "POST", AuthMethod: "none", BodyEncoding: "json", Body: `{"model":"original","messages":[{"role":"user","content":"keep me"}]}`, Headers: `{"X-Key":"private-link-key"}`})
	if err != nil {
		t.Fatal(err)
	}
	id, err := f.service.Write(ctx, 0, WriteInput{Action: "create", ConfigRevision: cfg.Revision, Monitor: MonitorInput{Name: "关联监控", Type: "http", URL: "https://monitor.example/probe", Interval: 60, TemplateID: item.ID, TemplateRevision: item.Revision, TemplateModel: "my-model"}})
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.edited["id"], _ = json.Marshal(id)
	f.edited["basic_auth_pass"] = json.RawMessage(`"private-monitor-password"`)
	f.edited["authMethod"] = json.RawMessage(`"basic"`)
	f.edited["basic_auth_user"] = json.RawMessage(`"private-user"`)
	f.edited["timeout"] = json.RawMessage(`45`)
	raw, _ := json.Marshal(f.edited)
	f.monitors["20"] = raw
	f.mu.Unlock()
	if err = f.service.DeleteTemplate(ctx, item.ID, item.Revision); err != nil {
		t.Fatal(err)
	}
	// A new service must recover the saved association from backend storage.
	service := New(f.service.store, f.service.client)
	snapshot, err := service.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(snapshot.Monitors)
	var items []map[string]json.RawMessage
	if err = json.Unmarshal(data, &items); err != nil {
		t.Fatal(err)
	}
	var saved map[string]json.RawMessage
	for _, m := range items {
		if string(m["id"]) == "20" {
			saved = m
		}
	}
	if rawString(saved, "template_id") != item.ID || rawString(saved, "template_name") != "原模板" || rawString(saved, "template_model") != "my-model" {
		t.Fatalf("template association missing: %s", data)
	}
	if strings.Contains(string(data), "private-") {
		t.Fatal("readback exposed credentials")
	}
	var in MonitorInput
	if err = json.Unmarshal([]byte(`{"name":"改名监控","type":"http","interval":60,"template_retain":true,"template_model":"my-model"}`), &in); err != nil {
		t.Fatal(err)
	}
	in.TemplateID, in.TemplateRevision = item.ID, item.Revision
	_, err = service.Write(ctx, id, WriteInput{Action: "edit", ConfigRevision: cfg.Revision, Revision: rawString(saved, "revision"), Monitor: in})
	if err != nil {
		t.Fatal(err)
	}
	if rawString(f.edited, "body") != `{"messages":[{"content":"keep me","role":"user"}],"model":"my-model"}` {
		var body map[string]any
		if json.Unmarshal([]byte(rawString(f.edited, "body")), &body) != nil || body["model"] != "my-model" {
			t.Fatal("normal edit changed request body")
		}
	}
	if rawInt(f.edited, "timeout", 0) != 45 || rawString(f.edited, "basic_auth_pass") != "private-monitor-password" || rawString(f.edited, "headers") != `{"X-Key":"private-link-key"}` {
		t.Fatal("normal edit overwrote request configuration")
	}
}

func TestEditingMonitorWithTemplatePreservesUnchangedAuthentication(t *testing.T) {
	for _, scenario := range []struct {
		name, method   string
		withoutOptions bool
	}{
		{"basic", "basic", false}, {"bearer", "bearer", false}, {"other-auth", "ntlm", false},
		{"headers", "none", false}, {"without-options", "basic", true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := resourceFixture(t)
			c := f.configure(t, true)
			ctx := context.Background()
			var raw map[string]json.RawMessage
			_ = json.Unmarshal([]byte(`{"id":19,"name":"API","type":"http","url":"https://monitor.example","interval":300,"authMethod":"basic","basic_auth_user":"original-user","basic_auth_pass":"original-pass","bearer_token":"original-token","authDomain":"original-domain","headers":"{\"Authorization\":\"Bearer original-header\",\"X-API-Key\":\"original-key\",\"X-Old\":\"old\"}"}`), &raw)
			raw["authMethod"], _ = json.Marshal(scenario.method)
			f.monitors["19"], _ = json.Marshal(raw)
			current, _, err := decodeMonitor(f.monitors["19"])
			if err != nil {
				t.Fatal(err)
			}
			template, err := f.service.SaveTemplate(ctx, "", TemplateInput{Name: "新模板", Method: "POST", Headers: `{"Authorization":"Bearer template-header","X-App":"new"}`, Body: `{"model":"new-model"}`, AuthMethod: "bearer", AuthPassword: "template-token"})
			if err != nil {
				t.Fatal(err)
			}
			options := current.Options
			options.Timeout = 42
			if scenario.withoutOptions {
				options = nil
			}
			_, err = f.service.Write(ctx, 19, WriteInput{Action: "edit", ConfigRevision: c.Revision, Revision: current.Revision, Monitor: MonitorInput{Name: "改名", Type: "http", Interval: 60, URL: "https://monitor.example/new", Options: options, TemplateID: template.ID, TemplateRevision: template.Revision, TemplateAuthOverride: !scenario.withoutOptions}})
			if err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"authMethod", "basic_auth_user", "basic_auth_pass", "bearer_token", "authDomain"} {
				if string(f.edited[key]) != string(raw[key]) {
					t.Fatalf("unchanged authentication field %s was overwritten", key)
				}
			}
			var headers map[string]string
			if err := json.Unmarshal([]byte(rawString(f.edited, "headers")), &headers); err != nil {
				t.Fatal(err)
			}
			if headers["Authorization"] != "Bearer original-header" || headers["X-API-Key"] != "original-key" || headers["X-App"] != "new" {
				t.Fatal("template lost original authentication headers")
			}
			if rawString(f.edited, "body") != `{"model":"new-model"}` || rawString(f.edited, "method") != "POST" {
				t.Fatal("template request settings were not applied")
			}
		})
	}
}

func TestEditingTemplateAllowsPartialCredentialUpdateAndExplicitClear(t *testing.T) {
	for _, scenario := range []struct {
		name, password string
		clear          bool
	}{
		{"password-only", "replacement", false}, {"clear", "", true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := resourceFixture(t)
			c := f.configure(t, true)
			ctx := context.Background()
			f.monitors["19"] = json.RawMessage(`{"id":19,"name":"API","type":"http","url":"https://monitor.example","interval":300,"authMethod":"basic","basic_auth_user":"original-user","basic_auth_pass":"original-pass"}`)
			current, _, err := decodeMonitor(f.monitors["19"])
			if err != nil {
				t.Fatal(err)
			}
			template, err := f.service.SaveTemplate(ctx, "", TemplateInput{Name: "新模板", Method: "POST", AuthMethod: "none"})
			if err != nil {
				t.Fatal(err)
			}
			options := current.Options
			options.AuthPassword, options.ClearAuth = scenario.password, scenario.clear
			_, err = f.service.Write(ctx, 19, WriteInput{Action: "edit", ConfigRevision: c.Revision, Revision: current.Revision, Monitor: MonitorInput{Name: current.Name, Type: "http", Interval: 60, Options: options, TemplateID: template.ID, TemplateRevision: template.Revision, TemplateAuthOverride: true}})
			if err != nil {
				t.Fatal(err)
			}
			if scenario.clear {
				if rawString(f.edited, "basic_auth_user") != "" || rawString(f.edited, "basic_auth_pass") != "" {
					t.Fatal("explicit clear was ignored")
				}
			} else if rawString(f.edited, "basic_auth_user") != "original-user" || rawString(f.edited, "basic_auth_pass") != "replacement" {
				t.Fatal("partial credential update lost original username")
			}
		})
	}
}
