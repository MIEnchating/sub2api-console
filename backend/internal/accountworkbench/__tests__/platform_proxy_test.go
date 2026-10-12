package accountworkbench_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestTemplateProxyIsUsedForLoginAndNotExposedInPreview(t *testing.T) {
	service, store := fixture(t, `{"data":`+strings.Replace(sourceAccount, `"priority":10`, `"priority":10,"proxy_id":3`, 1)+`}`)
	ctx := context.Background()
	owner, _ := previewOwner(t, store)
	source, err := service.TemplateSource(ctx, "41")
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.SaveTemplate(ctx, accountworkbench.TemplateInput{Name: "平台代理模板", SourceID: "41", SourceVersion: source.SourceVersion})
	if err != nil {
		t.Fatal(err)
	}
	service.UseTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/proxies/3") {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		return upstreamResponse(map[string]any{"data": map[string]any{"id": 3, "name": "固定出口", "protocol": "http", "host": "proxy.example.test", "port": 8080, "username": "operator", "password": "proxy-private", "status": "active"}}), nil
	}))
	preview, err := service.Preview(ctx, owner, accountworkbench.PreviewInput{Action: "import", Content: "rt_test"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err := store.WorkbenchDocument(ctx, "preview:"+owner)
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Settings accountworkbench.PreviewInput `json:"settings"`
	}
	if err = json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Settings.ProxyURL != "http://operator:proxy-private@proxy.example.test:8080" {
		t.Fatal("template proxy was not bound to login")
	}
	public, _ := json.Marshal(preview)
	if strings.Contains(string(public), "proxy-private") || strings.Contains(string(public), "operator") {
		t.Fatal("proxy credentials exposed")
	}
}

func platformProxyFixture() map[string]any {
	return map[string]any{"id": 3, "name": "固定出口", "protocol": "http", "host": "proxy.example.test", "port": 8080, "username": "operator", "password": "proxy-private", "status": "active"}
}

func TestLegacyPreviewWithPlatformProxyCannotExecuteWithoutBinding(t *testing.T) {
	service, store := fixture(t, "{}")
	owner, _ := previewOwner(t, store)
	runner, _ := runTasks(t, service)
	service.UseTransport(transportFunc(func(*http.Request) (*http.Response, error) {
		return upstreamResponse(map[string]any{"data": platformProxyFixture()}), nil
	}))
	preview, err := service.Preview(t.Context(), owner, accountworkbench.PreviewInput{Action: "import", Content: "rt_test", PlatformProxyID: "3"})
	if err != nil {
		t.Fatal(err)
	}
	raw, revision, err := store.WorkbenchDocument(t.Context(), "preview:"+owner)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err = json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	delete(document, "proxy_binding")
	raw, _ = json.Marshal(document)
	revision, err = store.SaveWorkbenchDocument(t.Context(), "preview:"+owner, revision, raw)
	if err != nil {
		t.Fatal(err)
	}
	service.UseOfficialTransport(transportFunc(func(*http.Request) (*http.Response, error) {
		t.Error("legacy task sent credentials without bound proxy")
		return nil, nil
	}))
	run, err := service.Start(t.Context(), owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: revision})
	if err != nil {
		t.Fatal(err)
	}
	awaitRun(t, runner)
	result, err := service.Run(t.Context(), owner, run.ID)
	if err != nil || result.Status == "completed" {
		t.Fatal("legacy task bypassed proxy binding")
	}
}

func TestRememberedLoginUsesCurrentPlatformProxyAndGenerationFollowsBinding(t *testing.T) {
	f := importedRememberedFixture(t, true)
	f.account["proxy_id"] = json.Number("3")
	f.service.UseTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/proxies/3") {
			return upstreamResponse(map[string]any{"data": platformProxyFixture()}), nil
		}
		if r.Method != "GET" || !strings.HasSuffix(r.URL.Path, "/accounts/41") {
			t.Fatal("unexpected request")
		}
		return upstreamResponse(map[string]any{"data": f.account}), nil
	}))
	saved, err := f.service.RememberedAccounts(t.Context(), f.owner)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := f.service.PreviewRememberedLogin(t.Context(), f.owner, saved[0].ID, saved[0].Revision)
	if err != nil || preview.Proxy == nil || preview.Proxy.ID != "3" {
		t.Fatalf("current proxy not selected: %v", err)
	}
	target, err := f.store.TargetSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	_, proxyURL, err := f.service.GenerationCredentials(t.Context(), target, f.account)
	if err != nil || proxyURL != "http://operator:proxy-private@proxy.example.test:8080" {
		t.Fatalf("generation did not follow current platform proxy: %v", err)
	}
}

func TestPlatformProxySelectionRejectsUnavailableOrChangedConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, key string
		value     any
	}{
		{"password changed", "password", "changed-secret"},
		{"disabled", "status", "inactive"},
		{"expired", "expires_at", "2000-01-01T00:00:00Z"},
		{"fallback to direct", "fallback_mode", "direct"},
		{"wrong stable ID", "id", 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, store := fixture(t, "{}")
			owner, _ := previewOwner(t, store)
			runTasks(t, service)
			proxy := platformProxyFixture()
			service.UseTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method != "GET" || !strings.HasSuffix(r.URL.Path, "/proxies/3") {
					t.Fatalf("unexpected management request: %s", r.URL.Path)
				}
				return upstreamResponse(map[string]any{"data": proxy}), nil
			}))
			preview, err := service.Preview(t.Context(), owner, accountworkbench.PreviewInput{Action: "import", Content: "rt_test", PlatformProxyID: "3"})
			if err != nil {
				t.Fatal(err)
			}
			if preview.Template == nil || preview.Template.Config.ProxyID == nil || *preview.Template.Config.ProxyID != "3" {
				t.Fatal("selected proxy not applied to imported account")
			}
			proxy[tc.key] = tc.value
			_, err = service.Start(t.Context(), owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision})
			if err == nil {
				t.Fatal("changed proxy allowed execution")
			}
			if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "proxy-private") {
				t.Fatal("error exposed proxy credentials")
			}
			runs, err := service.Runs(t.Context(), owner)
			if err != nil || len(runs) != 0 {
				t.Fatal("proxy mismatch launched task")
			}
		})
	}
}

func TestPlatformProxyDirectoryKeepsCredentialsPrivateAndMarksUnsupportedEntries(t *testing.T) {
	service, _ := fixture(t, "{}")
	active := platformProxyFixture()
	disabled := platformProxyFixture()
	disabled["id"] = 4
	disabled["status"] = "inactive"
	service.UseTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || !strings.HasSuffix(r.URL.Path, "/proxies") {
			t.Fatal("unexpected request")
		}
		return upstreamResponse(map[string]any{"data": map[string]any{"items": []any{active, disabled}, "total": 2}}), nil
	}))
	proxies, err := service.PlatformProxies(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(proxies) != 2 || !proxies[0].Available || proxies[1].Available || proxies[1].Reason == "" {
		t.Fatal("proxy availability lost")
	}
	raw, _ := json.Marshal(proxies)
	for _, secret := range []string{"proxy-private", "operator", "proxy.example.test", "password", "username"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("public directory exposed credentials or address")
		}
	}
}

func TestPlatformProxyRejectsIncompleteAuthenticationAndSessionExpansion(t *testing.T) {
	for _, tc := range []struct {
		key   string
		value any
	}{{"password", ""}, {"username", "operator-{session}"}, {"host", "127.0.0.1"}, {"protocol", "ftp"}} {
		t.Run(tc.key, func(t *testing.T) {
			service, store := fixture(t, "{}")
			owner, _ := previewOwner(t, store)
			proxy := platformProxyFixture()
			proxy[tc.key] = tc.value
			service.UseTransport(transportFunc(func(*http.Request) (*http.Response, error) {
				return upstreamResponse(map[string]any{"data": proxy}), nil
			}))
			if _, err := service.Preview(t.Context(), owner, accountworkbench.PreviewInput{Action: "import", Content: "rt_test", PlatformProxyID: "3"}); err == nil {
				t.Fatal("unusable proxy accepted")
			}
		})
	}
}

func TestPlatformProxyCannotBeCombinedWithManualProxyOrLocalExport(t *testing.T) {
	service, store := fixture(t, "{}")
	owner, _ := previewOwner(t, store)
	service.UseTransport(transportFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid scope contacted management")
		return nil, nil
	}))
	for _, input := range []accountworkbench.PreviewInput{
		{Action: "export", Content: "rt_test", PlatformProxyID: "3"},
		{Action: "import", Content: "rt_test", PlatformProxyID: "3", ProxyEnabled: true, ProxyURL: "http://proxy.example.test:8080"},
	} {
		if _, err := service.Preview(t.Context(), owner, input); err == nil {
			t.Fatal("ambiguous proxy scope accepted")
		}
	}
}
