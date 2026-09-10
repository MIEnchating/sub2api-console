package uptimekuma

import (
	"context"
	"encoding/json"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"strings"
	"testing"
)

func TestTemplateIncludesMonitoringSettingsAndAppliesWithoutChangingGroup(t *testing.T) {
	f := resourceFixture(t)
	f.monitors["7"] = json.RawMessage(`{"id":7,"name":"分组","type":"group","interval":60,"active":true}`)
	c := f.configure(t, true)
	ctx := context.Background()
	var input TemplateInput
	err := json.Unmarshal([]byte(`{"name":"完整监控","method":"POST","auth_method":"none","monitoring":{"type":"http","url":"https://monitor.example/v1/messages?key=private-url","interval":300,"timeout":120,"retry_interval":180,"max_retries":3,"max_redirects":4,"accepted_status_codes":["200-299","301"],"ignore_tls":true,"upside_down":true}}`), &input)
	if err != nil {
		t.Fatal(err)
	}
	item, err := f.service.SaveTemplate(ctx, "", input)
	if err != nil {
		t.Fatal(err)
	}
	summary, _ := json.Marshal(item)
	if strings.Contains(string(summary), "private-url") {
		t.Fatal("template leaked URL credentials")
	}
	if !containsJSONField(summary, "monitoring") {
		t.Fatal("monitoring settings missing from template")
	}
	parent := int64(7)
	_, err = f.service.Write(ctx, 0, WriteInput{Action: "create", ConfigRevision: c.Revision, Monitor: MonitorInput{Name: "API", Type: "http", Interval: 60, Parent: &parent, TemplateID: item.ID, TemplateRevision: item.Revision}})
	if err != nil {
		t.Fatal(err)
	}
	if rawInt(f.edited, "interval", 0) != 300 || rawInt(f.edited, "timeout", 0) != 120 || rawInt(f.edited, "parent", 0) != 7 || rawInt(f.edited, "maxretries", 0) != 3 || !rawBool(f.edited, "ignoreTls") || !rawBool(f.edited, "upsideDown") || rawString(f.edited, "url") != "https://monitor.example/v1/messages?key=private-url" {
		t.Fatal("complete template settings were not applied")
	}
}

func TestClaudeTemplateSeedIsCompleteAndDoesNotRestoreDeletedTemplate(t *testing.T) {
	f := resourceFixture(t)
	ctx := context.Background()
	if err := SeedTemplates(ctx, f.service.store); err != nil {
		t.Fatal(err)
	}
	item, err := f.service.store.KumaTemplate(ctx, claudeTemplateID)
	if err != nil {
		t.Fatal(err)
	}
	var headers map[string]string
	var body map[string]any
	if json.Unmarshal([]byte(item.Headers), &headers) != nil || json.Unmarshal([]byte(item.Body), &body) != nil {
		t.Fatal("invalid built-in JSON")
	}
	if headers["User-Agent"] != "claude-cli/2.1.114 (external, sdk-cli)" || headers["X-App"] != "cli" || headers["anthropic-beta"] == "" || headers["anthropic-version"] != "2023-06-01" {
		t.Fatal("missing CLI headers")
	}
	if body["max_tokens"] != float64(16) || body["model"] != "claude-sonnet-4-6" || body["system"] == nil || body["metadata"] == nil || body["messages"] == nil {
		t.Fatal("missing CLI message structure")
	}
	if item.Monitoring.Interval != 300 || item.Monitoring.Timeout != 120 {
		t.Fatal("incorrect probe defaults")
	}
	item.Name = "用户修改的模板"
	if err = f.service.store.SaveKumaTemplate(ctx, item); err != nil {
		t.Fatal(err)
	}
	if err = SeedTemplates(ctx, f.service.store); err != nil {
		t.Fatal(err)
	}
	saved, err := f.service.store.KumaTemplate(ctx, claudeTemplateID)
	if err != nil || saved.Name != item.Name {
		t.Fatal("seed overwrote edited template")
	}
	if err = f.service.DeleteTemplate(ctx, saved.ID, saved.Revision); err != nil {
		t.Fatal(err)
	}
	if err = SeedTemplates(ctx, f.service.store); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.store.KumaTemplate(ctx, claudeTemplateID); err != configstore.ErrKumaTemplateConflict {
		t.Fatal("seed restored deleted template")
	}
}

func TestMonitoringTemplateRejectsInvalidSettingsAndAllowsExplicitOverrides(t *testing.T) {
	f := resourceFixture(t)
	c := f.configure(t, true)
	ctx := context.Background()
	if err := SeedTemplates(ctx, f.service.store); err != nil {
		t.Fatal(err)
	}
	_, err := f.service.Write(ctx, 0, WriteInput{Action: "create", ConfigRevision: c.Revision, Monitor: MonitorInput{Name: "API", Type: "http", URL: "https://monitor.example/v1/messages", Interval: 600, TemplateID: claudeTemplateID, TemplateRevision: 1, TemplateSettingsOverride: true, Options: &MonitorOptions{Method: "GET", Timeout: 45, RetryInterval: 300, MaxRetries: 2, MaxRedirects: 5, AcceptedStatusCodes: []string{"201"}, AuthMethod: "none"}}})
	if err != nil {
		t.Fatal(err)
	}
	if rawInt(f.edited, "interval", 0) != 600 || rawInt(f.edited, "timeout", 0) != 45 || rawString(f.edited, "method") != "POST" {
		t.Fatal("template overwrote explicit monitoring adjustments")
	}
	if rawString(f.edited, "httpBodyEncoding") != "json" {
		t.Fatal("CLI request must use JSON body encoding")
	}
	var invalid TemplateInput
	_ = json.Unmarshal([]byte(`{"name":"坏模板","method":"POST","auth_method":"none","monitoring":{"type":"http","interval":1,"timeout":0}}`), &invalid)
	if _, err = f.service.SaveTemplate(ctx, "", invalid); err == nil {
		t.Fatal("invalid monitoring parameters accepted")
	}
}

func containsJSONField(data []byte, key string) bool {
	var value map[string]json.RawMessage
	_ = json.Unmarshal(data, &value)
	return len(value[key]) > 0
}

func TestTCPTemplateSelectsTypeAndTargetWithoutLosingGroup(t *testing.T) {
	f := resourceFixture(t)
	c := f.configure(t, true)
	ctx := context.Background()
	var input TemplateInput
	if err := json.Unmarshal([]byte(`{"name":"TCP 服务","method":"GET","auth_method":"none","monitoring":{"type":"port","hostname":"service.example","port":8443,"interval":120,"timeout":30,"retry_interval":60,"max_retries":2,"max_redirects":10,"accepted_status_codes":["200-299"]}}`), &input); err != nil {
		t.Fatal(err)
	}
	item, err := f.service.SaveTemplate(ctx, "", input)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.service.Write(ctx, 0, WriteInput{Action: "create", ConfigRevision: c.Revision, Monitor: MonitorInput{Name: "TCP", TemplateID: item.ID, TemplateRevision: item.Revision}})
	if err != nil {
		t.Fatal(err)
	}
	if rawString(f.edited, "type") != "port" || rawString(f.edited, "hostname") != "service.example" || rawInt(f.edited, "port", 0) != 8443 {
		t.Fatal("TCP template target not applied")
	}
}

func TestTemplateURLBlankRetainsSecretAndExplicitClearRemovesIt(t *testing.T) {
	f := resourceFixture(t)
	ctx := context.Background()
	input := TemplateInput{Name: "URL", Method: "GET", AuthMethod: "none", Monitoring: &configstore.KumaTemplateMonitoring{Type: "http", URL: "https://monitor.example?token=secret", Interval: 60, Timeout: 16, RetryInterval: 60, MaxRedirects: 10, AcceptedStatusCodes: []string{"200-299"}}}
	item, err := f.service.SaveTemplate(ctx, "", input)
	if err != nil {
		t.Fatal(err)
	}
	if !item.URLRedacted || item.Monitoring.URL != "" {
		t.Fatal("secret URL exposed")
	}
	input.Revision = item.Revision
	input.Monitoring.URL = ""
	item, err = f.service.SaveTemplate(ctx, item.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := f.service.store.KumaTemplate(ctx, item.ID)
	if err != nil || saved.Monitoring.URL != "https://monitor.example?token=secret" {
		t.Fatal("blank URL lost existing secret")
	}
	input.Revision = item.Revision
	input.ClearURL = true
	item, err = f.service.SaveTemplate(ctx, item.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	if item.URLConfigured {
		t.Fatal("explicit URL clear did not apply")
	}
}
