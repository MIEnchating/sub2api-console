package uptimekuma

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestStandardProfilesGenerateProtocolSpecificBodies(t *testing.T) {
	for _, tc := range []struct{ profile, tokenKey, inputKey string }{
		{"claude-messages", "max_tokens", "messages"},
		{"openai-chat", "max_completion_tokens", "messages"},
		{"openai-responses", "max_output_tokens", "input"},
	} {
		t.Run(tc.profile, func(t *testing.T) {
			f := resourceFixture(t)
			ctx := context.Background()
			item, err := f.service.SaveTemplate(ctx, "", TemplateInput{Name: "API 探活", Method: "GET", AuthMethod: "bearer", AuthPassword: "fixture-secret", RequestProfile: tc.profile, Model: "selected-model", Monitoring: &configstore.KumaTemplateMonitoring{Type: "http", URL: "https://monitor.example/probe", Interval: 300, Timeout: 120, RetryInterval: 60, MaxRedirects: 10, AcceptedStatusCodes: []string{"200-299"}}})
			if err != nil {
				t.Fatal(err)
			}
			monitor := MonitorInput{TemplateID: item.ID, TemplateRevision: item.Revision}
			if err = f.service.resolveTemplate(ctx, &monitor); err != nil {
				t.Fatal(err)
			}
			var body map[string]any
			var headers map[string]string
			if err = json.Unmarshal([]byte(monitor.Options.Body), &body); err != nil {
				t.Fatal(err)
			}
			if err = json.Unmarshal([]byte(monitor.Options.Headers), &headers); err != nil {
				t.Fatal(err)
			}
			if body["model"] != "selected-model" || body[tc.tokenKey] != float64(16) || body[tc.inputKey] == nil || body["stream"] != false {
				t.Fatalf("wrong request body: %v", body)
			}
			for _, key := range []string{"max_tokens", "max_completion_tokens", "max_output_tokens"} {
				if key != tc.tokenKey && body[key] != nil {
					t.Fatalf("foreign token parameter %s", key)
				}
			}
			if body["system"] != nil || body["metadata"] != nil || headers["X-App"] != "" || headers["User-Agent"] != "" {
				t.Fatal("standard request contains CLI identity")
			}
			if tc.profile == "claude-messages" {
				if headers["anthropic-version"] != "2023-06-01" {
					t.Fatal("missing Anthropic version")
				}
			} else if body["store"] != false || headers["anthropic-version"] != "" {
				t.Fatal("wrong OpenAI options")
			}
			if tc.profile == "openai-responses" && body["messages"] != nil {
				t.Fatal("Responses must use input")
			}
			if monitor.Options.Method != "POST" || monitor.Options.bodyEncoding != "json" || monitor.Options.AuthPassword != "fixture-secret" || headers["Content-Type"] != "application/json" {
				t.Fatal("generated request settings or auth missing")
			}
		})
	}
}

func TestBuiltInProfilesRejectUnknownModesAndInvalidModels(t *testing.T) {
	for _, tc := range []struct{ profile, model, kind string }{
		{"openai-invalid", "model", "http"}, {"openai-chat", "", "http"}, {"openai-responses", "model\ninjected", "http"}, {"claude-messages", "model", "port"},
	} {
		t.Run(tc.profile+tc.kind, func(t *testing.T) {
			item := configstore.KumaTemplate{RequestProfile: tc.profile, Model: tc.model, Monitoring: &configstore.KumaTemplateMonitoring{Type: tc.kind}}
			if prepareTemplateProfile(&item) == nil {
				t.Fatal("invalid profile accepted")
			}
		})
	}
}

func TestStandardTemplateSeedsAreIndependentAndRespectEditsAndDeletion(t *testing.T) {
	f := resourceFixture(t)
	ctx := context.Background()
	if err := SeedTemplates(ctx, f.service.store); err != nil {
		t.Fatal(err)
	}
	items, err := f.service.Templates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 {
		t.Fatalf("want four built-ins, got %d", len(items))
	}
	for _, item := range items {
		stored, err := f.service.store.KumaTemplate(ctx, item.ID)
		if err != nil {
			t.Fatal(err)
		}
		stored.Name = "用户模板"
		if err = f.service.store.SaveKumaTemplate(ctx, stored); err != nil {
			t.Fatal(err)
		}
		if err = SeedTemplates(ctx, f.service.store); err != nil {
			t.Fatal(err)
		}
		saved, err := f.service.store.KumaTemplate(ctx, item.ID)
		if err != nil || saved.Name != "用户模板" {
			t.Fatal("seed overwrote edits")
		}
		if err = f.service.DeleteTemplate(ctx, saved.ID, saved.Revision); err != nil {
			t.Fatal(err)
		}
		if err = SeedTemplates(ctx, f.service.store); err != nil {
			t.Fatal(err)
		}
		if _, err = f.service.store.KumaTemplate(ctx, item.ID); err != configstore.ErrKumaTemplateConflict {
			t.Fatal("seed resurrected deleted template")
		}
	}
}

func TestProfileSwitchRebuildsProtocolHeadersAndKeepsPrivateAPIKey(t *testing.T) {
	item := configstore.KumaTemplate{RequestProfile: "claude-cli", Model: "model", Monitoring: &configstore.KumaTemplateMonitoring{Type: "http"}, Headers: `{"x-api-key":"private-key","X-Custom":"custom"}`}
	if err := prepareTemplateProfile(&item); err != nil {
		t.Fatal(err)
	}
	item.RequestProfile = "openai-chat"
	if err := prepareTemplateProfile(&item); err != nil {
		t.Fatal(err)
	}
	var headers map[string]string
	if err := json.Unmarshal([]byte(item.Headers), &headers); err != nil {
		t.Fatal(err)
	}
	if headers["x-api-key"] != "private-key" || headers["X-Custom"] != "custom" || headers["anthropic-version"] != "" || headers["X-App"] != "" {
		t.Fatal("profile switch lost custom headers or retained CLI headers")
	}
}

func TestIndependentMonitorAuthRemovesTemplateAPIKeyHeader(t *testing.T) {
	f := resourceFixture(t)
	ctx := context.Background()
	item, err := f.service.SaveTemplate(ctx, "", TemplateInput{Name: "API", Method: "POST", AuthMethod: "none", Headers: `{"X-API-Key":"private-key","X-Custom":"keep"}`})
	if err != nil {
		t.Fatal(err)
	}
	in := MonitorInput{Type: "http", TemplateID: item.ID, TemplateRevision: item.Revision, TemplateAuthOverride: true, Options: &MonitorOptions{AuthMethod: "bearer", AuthPassword: "override"}}
	if err = f.service.resolveTemplate(ctx, &in); err != nil {
		t.Fatal(err)
	}
	var headers map[string]string
	if err = json.Unmarshal([]byte(in.Options.Headers), &headers); err != nil {
		t.Fatal(err)
	}
	if headers["X-API-Key"] != "" || headers["X-Custom"] != "keep" || in.Options.AuthPassword != "override" {
		t.Fatal("independent auth retained private template key or lost unrelated headers")
	}
}

func TestExplicitTemplateRequestKeepsEditedBodyHeadersAndEncoding(t *testing.T) {
	f := resourceFixture(t)
	ctx := context.Background()
	var in TemplateInput
	if err := json.Unmarshal([]byte(`{"name":"可编辑请求","method":"POST","auth_method":"none","request_profile":"openai-chat","model":"custom","body_encoding":"form","body":"model=custom&input=ping","clear_headers":true,"monitoring":{"type":"http","interval":60,"timeout":30,"retry_interval":60,"max_redirects":10,"accepted_status_codes":["200-299"]}}`), &in); err != nil {
		t.Fatal(err)
	}
	item, err := f.service.SaveTemplate(ctx, "", in)
	if err != nil {
		t.Fatal(err)
	}
	monitor := MonitorInput{TemplateID: item.ID, TemplateRevision: item.Revision}
	if err = f.service.resolveTemplate(ctx, &monitor); err != nil {
		t.Fatal(err)
	}
	if monitor.Options.Body != "model=custom&input=ping" || monitor.Options.Headers != "" || monitor.Options.bodyEncoding != "form" {
		t.Fatal("explicit request was replaced by preset")
	}
}

func TestExplicitTemplateRejectsInvalidEncodingAndJSON(t *testing.T) {
	f := resourceFixture(t)
	for _, tc := range []struct{ encoding, body string }{{"raw", "text"}, {"json", "not json"}} {
		_, err := f.service.SaveTemplate(context.Background(), "", TemplateInput{Name: "校验", Method: "POST", AuthMethod: "none", BodyEncoding: tc.encoding, Body: tc.body})
		if err == nil {
			t.Fatal("invalid request accepted")
		}
	}
}

func TestTemplateBodyEncodingReachesKumaMonitor(t *testing.T) {
	for _, tc := range []struct{ encoding, body string }{{"json", `{"input":"ping"}`}, {"form", "input=ping"}, {"xml", "<input>ping</input>"}} {
		t.Run(tc.encoding, func(t *testing.T) {
			f := resourceFixture(t)
			c := f.configure(t, true)
			ctx := context.Background()
			item, err := f.service.SaveTemplate(ctx, "", TemplateInput{Name: "编码检测", Method: "POST", AuthMethod: "none", Body: tc.body, BodyEncoding: tc.encoding})
			if err != nil {
				t.Fatal(err)
			}
			_, err = f.service.Write(ctx, 0, WriteInput{Action: "create", ConfigRevision: c.Revision, Monitor: MonitorInput{Name: "编码检测", Type: "http", URL: "https://monitor.example/probe", Interval: 60, TemplateID: item.ID, TemplateRevision: item.Revision}})
			if err != nil {
				t.Fatal(err)
			}
			if rawString(f.edited, "httpBodyEncoding") != tc.encoding || rawString(f.edited, "body") != tc.body {
				t.Fatal("monitor did not receive selected encoding and exact body")
			}
		})
	}
}

func TestMonitorTemplateModelReachesKumaWithoutChangingStoredTemplate(t *testing.T) {
	for _, profile := range []string{"claude-cli", "claude-messages", "openai-chat", "openai-responses"} {
		t.Run(profile, func(t *testing.T) {
			f := resourceFixture(t)
			c := f.configure(t, true)
			ctx := context.Background()
			preset, err := TemplatePreset(profile, "template-model")
			if err != nil {
				t.Fatal(err)
			}
			var body map[string]json.RawMessage
			if err = json.Unmarshal([]byte(preset.Body), &body); err != nil {
				t.Fatal(err)
			}
			body["custom_counter"] = json.RawMessage(`9007199254740993`)
			data, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			item, err := f.service.SaveTemplate(ctx, "", TemplateInput{Name: "模型模板", Method: "POST", AuthMethod: "none", BodyEncoding: "json", RequestProfile: profile, Monitoring: &configstore.KumaTemplateMonitoring{Type: "http", Interval: 60, Timeout: 16, RetryInterval: 60, MaxRedirects: 10, AcceptedStatusCodes: []string{"200-299"}}, Model: "template-model", Body: string(data), Headers: `{"X-Key":"fixture-key"}`})
			if err != nil {
				t.Fatal(err)
			}
			var in MonitorInput
			if err = json.Unmarshal([]byte(`{"name":"独立模型","type":"http","url":"https://monitor.example/probe","interval":60,"template_model":"monitor-model"}`), &in); err != nil {
				t.Fatal(err)
			}
			in.TemplateID, in.TemplateRevision = item.ID, item.Revision
			if _, err = f.service.Write(ctx, 0, WriteInput{Action: "create", ConfigRevision: c.Revision, Monitor: in}); err != nil {
				t.Fatal(err)
			}
			var actual map[string]json.RawMessage
			if err = json.Unmarshal([]byte(rawString(f.edited, "body")), &actual); err != nil {
				t.Fatal(err)
			}
			if string(actual["model"]) != `"monitor-model"` {
				t.Fatalf("monitor model not applied: %s", actual["model"])
			}
			delete(actual, "model")
			delete(body, "model")
			actualJSON, _ := json.Marshal(actual)
			expectedJSON, _ := json.Marshal(body)
			if string(actualJSON) != string(expectedJSON) {
				t.Fatal("model override changed other request fields")
			}
			if rawString(f.edited, "headers") != `{"X-Key":"fixture-key"}` {
				t.Fatal("model override changed template headers")
			}
			stored, err := f.service.Template(ctx, item.ID)
			if err != nil || stored.Body != string(data) || stored.Model != "template-model" || stored.Revision != item.Revision {
				t.Fatal("monitor override modified stored template")
			}
		})
	}
}

func TestMonitorModelOverrideRejectsInvalidInputBeforeRemoteWrite(t *testing.T) {
	for _, tc := range []struct {
		name, body, encoding, model string
		noTemplate                  bool
	}{
		{"missing-template", `{"model":"old"}`, "json", "new", true},
		{"whitespace", `{"model":"old"}`, "json", "invalid model", false},
		{"control-character", `{"model":"old"}`, "json", "invalid\x00model", false},
		{"too-long", `{"model":"old"}`, "json", strings.Repeat("a", 201), false},
		{"json-array", `[]`, "json", "new", false},
		{"json-null", `null`, "json", "new", false},
		{"empty-body", ``, "json", "new", false},
		{"form-body", `model=old`, "form", "new", false},
		{"xml-body", `<model>old</model>`, "xml", "new", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := resourceFixture(t)
			c := f.configure(t, true)
			ctx := context.Background()
			item, err := f.service.SaveTemplate(ctx, "", TemplateInput{Name: "请求模板", Method: "POST", AuthMethod: "none", BodyEncoding: tc.encoding, Body: tc.body})
			if err != nil {
				t.Fatal(err)
			}
			in := MonitorInput{Name: "模型校验", Type: "http", URL: "https://monitor.example/probe", Interval: 60, TemplateID: item.ID, TemplateRevision: item.Revision, TemplateModel: tc.model}
			if tc.noTemplate {
				in.TemplateID = ""
			}
			_, err = f.service.Write(ctx, 0, WriteInput{Action: "create", ConfigRevision: c.Revision, Monitor: in})
			if err == nil || PublicError(err).Code != "kuma_invalid_template_model" {
				t.Fatalf("expected model validation error, got %v", err)
			}
			if f.edited != nil {
				t.Fatal("invalid model reached remote write")
			}
		})
	}
}

func TestMonitorTemplateModelEmptyKeepsExactBodyAndEditCanOverrideModel(t *testing.T) {
	f := resourceFixture(t)
	c := f.configure(t, true)
	ctx := context.Background()
	body := "{\n  \"model\": \"original\", \"messages\": []\n}"
	item, err := f.service.SaveTemplate(ctx, "", TemplateInput{Name: "自定义 JSON", Method: "POST", AuthMethod: "none", BodyEncoding: "json", Body: body})
	if err != nil {
		t.Fatal(err)
	}
	in := MonitorInput{Name: "默认模型", Type: "http", URL: "https://monitor.example/probe", Interval: 60, TemplateID: item.ID, TemplateRevision: item.Revision}
	if err = f.service.resolveTemplate(ctx, &in); err != nil {
		t.Fatal(err)
	}
	if in.Options.Body != body {
		t.Fatal("empty override rewrote original body")
	}
	monitor, _, err := decodeMonitor(f.monitors["19"])
	if err != nil {
		t.Fatal(err)
	}
	in.Options = nil
	in.TemplateModel = "custom-edited-model"
	_, err = f.service.Write(ctx, 19, WriteInput{Action: "edit", ConfigRevision: c.Revision, Revision: monitor.Revision, Monitor: in})
	if err != nil {
		t.Fatal(err)
	}
	var actual map[string]json.RawMessage
	if err = json.Unmarshal([]byte(rawString(f.edited, "body")), &actual); err != nil {
		t.Fatal(err)
	}
	if string(actual["model"]) != `"custom-edited-model"` {
		t.Fatal("editing did not apply model override")
	}
}
