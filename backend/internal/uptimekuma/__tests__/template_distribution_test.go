package uptimekuma_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/uptimekuma"
)

func TestTemplateDistributionUpdatesBoundMonitorsAndPreservesIndividualSettings(t *testing.T) {
	f, peer, template := distributionFixture(t)
	preview := distributionPreview(t, f, template)
	if len(preview.Items) != 2 || preview.Items[0].MonitorID != 1 || preview.Items[1].MonitorID != 2 {
		t.Fatalf("wrong scope: %#v", preview)
	}
	public, _ := json.Marshal(preview)
	if strings.Contains(string(public), "private-") {
		t.Fatal("preview leaked credentials")
	}
	runDistribution(t, f, preview)
	final := f.completed(t)
	if final.Status != "succeeded" || final.Result["completed"] != json.Number("2") {
		t.Fatalf("distribution failed: %#v", final)
	}
	public, _ = json.Marshal(final)
	if strings.Contains(string(public), "private-") {
		t.Fatal("task leaked credentials")
	}
	peer.mu.Lock()
	defer peer.mu.Unlock()
	if len(peer.writes) != 2 {
		t.Fatalf("wrong writes: %v", peer.writes)
	}
	for _, id := range []string{"1", "2"} {
		var monitor map[string]json.RawMessage
		_ = json.Unmarshal(peer.monitors[id], &monitor)
		for key, want := range map[string]string{"interval": "300", "timeout": "90", "retryInterval": "120", "maxretries": "3", "active": "false", "parent": "99", "url": `"https://isolated.example.test/health?key=private-url"`, "bearer_token": `"private-token"`, "customOption": `"keep"`, "method": `"POST"`, "notificationIDList": `{"5":true,"7":false}`} {
			if string(monitor[key]) != want {
				t.Errorf("monitor %s %s = %s, want %s", id, key, monitor[key], want)
			}
		}
		var headers, body string
		_ = json.Unmarshal(monitor["headers"], &headers)
		_ = json.Unmarshal(monitor["body"], &body)
		if !strings.Contains(headers, "private-header") || strings.Contains(headers, "private-template") || strings.Contains(headers, "X-Old") || !strings.Contains(headers, "X-New") {
			t.Errorf("incorrect header merge: %s", headers)
		}
		model := "template-model"
		if id == "1" {
			model = "monitor-model"
		}
		if !strings.Contains(body, model) || !strings.Contains(body, "32") {
			t.Errorf("incorrect model/body: %s", body)
		}
	}
	links, err := f.config.KumaMonitorTemplates(f.ctx, f.server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if links[1].Revision != template.Revision || links[2].Revision != template.Revision || links[1].Model != "monitor-model" {
		t.Fatal("provenance not updated")
	}
}

func TestTemplateDistributionRejectsChangedPreviewWithoutWriting(t *testing.T) {
	for _, change := range []string{"monitor", "binding", "template", "config", "scope"} {
		t.Run(change, func(t *testing.T) {
			f, peer, template := distributionFixture(t)
			preview := distributionPreview(t, f, template)
			switch change {
			case "monitor":
				peer.mu.Lock()
				peer.monitors["1"] = json.RawMessage(`{"id":1,"name":"已修改","type":"http","interval":60,"url":"https://changed.example.test"}`)
				peer.mu.Unlock()
			case "binding":
				if err := f.config.DeleteKumaMonitorTemplate(f.ctx, f.server.URL, 1); err != nil {
					t.Fatal(err)
				}
			case "template":
				if err := f.service.Service.DeleteTemplate(f.ctx, template.ID, template.Revision); err != nil {
					t.Fatal(err)
				}
			case "config":
				c, err := f.config.UptimeKuma(f.ctx)
				if err != nil {
					t.Fatal(err)
				}
				if err = f.config.SaveUptimeKuma(f.ctx, c); err != nil {
					t.Fatal(err)
				}
			case "scope":
				preview.Scope = strings.Repeat("0", 64)
			}
			runDistribution(t, f, preview)
			final := f.completed(t)
			if final.Status != "failed" || final.Result["remote_write"] != false {
				t.Fatalf("stale preview accepted: %#v", final)
			}
			peer.mu.Lock()
			defer peer.mu.Unlock()
			if len(peer.writes) != 0 {
				t.Fatalf("unexpected writes: %v", peer.writes)
			}
		})
	}
}

func TestTemplateDistributionSkipsMissingAndIncompatibleMonitors(t *testing.T) {
	f, peer, template := distributionFixture(t)
	peer.mu.Lock()
	peer.monitors["2"] = json.RawMessage(`{"id":2,"name":"TCP","type":"port","interval":60}`)
	peer.mu.Unlock()
	if err := f.config.SaveKumaMonitorTemplate(f.ctx, f.server.URL, configstore.KumaMonitorTemplate{MonitorID: 4, TemplateID: template.ID, Revision: 1}); err != nil {
		t.Fatal(err)
	}
	preview := distributionPreview(t, f, template)
	if len(preview.Items) != 3 || preview.Items[1].Reason == "" || preview.Items[2].Reason == "" {
		t.Fatalf("skip reasons missing: %#v", preview)
	}
	runDistribution(t, f, preview)
	final := f.completed(t)
	if final.Status != "succeeded" || final.Result["completed"] != json.Number("1") {
		t.Fatalf("eligible monitor not updated: %#v", final)
	}
	peer.mu.Lock()
	defer peer.mu.Unlock()
	if len(peer.writes) != 1 || peer.writes[0] != 1 {
		t.Fatalf("wrong writes: %v", peer.writes)
	}
}

func TestTemplateDistributionStopsAfterFailureAndRetainsCompletedResults(t *testing.T) {
	f, peer, template := distributionFixture(t)
	peer.failID = 2
	if err := f.config.SaveKumaMonitorTemplate(f.ctx, f.server.URL, configstore.KumaMonitorTemplate{MonitorID: 3, TemplateID: template.ID, Revision: 1}); err != nil {
		t.Fatal(err)
	}
	preview := distributionPreview(t, f, template)
	runDistribution(t, f, preview)
	final := f.completed(t)
	if final.Status != "failed" || final.Result["completed"] != json.Number("1") {
		t.Fatalf("partial result lost: %#v", final)
	}
	items := final.Result["items"].([]any)
	for i, want := range []string{"succeeded", "failed", "not_run"} {
		if items[i].(map[string]any)["status"] != want {
			t.Fatalf("wrong result: %#v", items)
		}
	}
	peer.mu.Lock()
	defer peer.mu.Unlock()
	if len(peer.writes) != 2 {
		t.Fatalf("write retried or continued: %v", peer.writes)
	}
}

func TestTemplateDistributionEmptyScopeDoesNotWrite(t *testing.T) {
	f, peer, template := distributionFixture(t)
	for _, id := range []int64{1, 2} {
		if err := f.config.DeleteKumaMonitorTemplate(f.ctx, f.server.URL, id); err != nil {
			t.Fatal(err)
		}
	}
	preview := distributionPreview(t, f, template)
	if len(preview.Items) != 0 {
		t.Fatal("empty scope not represented")
	}
	runDistribution(t, f, preview)
	final := f.completed(t)
	if final.Status != "failed" || final.Result["error_code"] != "kuma_distribution_empty" {
		t.Fatalf("empty scope accepted: %#v", final)
	}
	peer.mu.Lock()
	defer peer.mu.Unlock()
	if len(peer.writes) != 0 {
		t.Fatal("empty scope wrote monitors")
	}
}

func TestTemplateDistributionPreservesHeaderCredentialsWithoutExplicitAuthMethod(t *testing.T) {
	f, peer, template := distributionFixture(t)
	peer.mu.Lock()
	var monitor map[string]json.RawMessage
	_ = json.Unmarshal(peer.monitors["1"], &monitor)
	delete(monitor, "authMethod")
	delete(monitor, "bearer_token")
	monitor["headers"] = json.RawMessage(`"{\"x-api-key\":\"private-monitor-key\"}"`)
	peer.monitors["1"], _ = json.Marshal(monitor)
	peer.mu.Unlock()
	preview := distributionPreview(t, f, template)
	runDistribution(t, f, preview)
	final := f.completed(t)
	if final.Status != "succeeded" {
		t.Fatalf("distribution failed: %#v", final)
	}
	peer.mu.Lock()
	defer peer.mu.Unlock()
	if !strings.Contains(string(peer.monitors["1"]), "private-monitor-key") {
		t.Fatal("header API key was removed when authMethod was absent")
	}
}

func TestTemplateDistributionCancellationKeepsCompletedItemsAndStopsLaterWrites(t *testing.T) {
	f, peer, template := distributionFixture(t)
	peer.blockedID = 2
	peer.entered = make(chan struct{})
	peer.stopped = make(chan struct{})
	if err := f.config.SaveKumaMonitorTemplate(f.ctx, f.server.URL, configstore.KumaMonitorTemplate{MonitorID: 3, TemplateID: template.ID, Revision: 1}); err != nil {
		t.Fatal(err)
	}
	preview := distributionPreview(t, f, template)
	queued, err := f.service.DistributeTemplate(f.ctx, template.ID, uptimekuma.TemplateDistributionInput{Revision: template.Revision, Scope: preview.Scope})
	if err != nil {
		t.Fatal(err)
	}
	final := cancelKumaWrite(t, f, queued, peer.entered, peer.stopped)
	if final.Result["completed"] != json.Number("1") {
		t.Fatalf("completed result lost: %#v", final)
	}
	items := final.Result["items"].([]any)
	if items[0].(map[string]any)["status"] != "succeeded" || items[2].(map[string]any)["status"] != "not_run" {
		t.Fatalf("partial result lost: %#v", items)
	}
	peer.mu.Lock()
	defer peer.mu.Unlock()
	if len(peer.writes) != 2 {
		t.Fatalf("cancelled write was replayed: %v", peer.writes)
	}
}
