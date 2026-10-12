package accountworkbench_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

func TestMaintenanceReusesCorrectedPasswordWhenRefreshedAccountStillReturns401(t *testing.T) {
	testMaintenanceRememberedLogin(t, false)
}

func TestMaintenanceDoesNotReuseDeletedLoginSource(t *testing.T) {
	testMaintenanceRememberedLogin(t, true)
}

func TestMaintenanceReauthorizationAndDetectionUseCurrentPlatformProxy(t *testing.T) {
	testMaintenanceRememberedLogin(t, false, true)
}

func testMaintenanceRememberedLogin(t *testing.T, deleteSource bool, useProxy ...bool) {
	t.Helper()
	service, private := fixture(t, `{}`)
	owner, token := previewOwner(t, private)
	runner, tasks := runTasks(t, service)
	catalog, err := business.Open(filepath.Join(t.TempDir(), "business.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()
	reader := upstreamsync.NewReader(&http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("OAuth flow must not reveal upstream keys")
	})})
	checker, err := modelcheck.New(tasks, private, catalog, reader)
	if err != nil {
		t.Fatal(err)
	}
	checker.UseOAuthTransport(transportFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://chatgpt.com/backend-api/codex/responses" {
			return nil, errors.New("unexpected check endpoint")
		}
		if request.Header.Get("Authorization") == "Bearer expired-access" {
			return &http.Response{StatusCode: 401, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"unauthorized"}}`))}, nil
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: {\"type\": \"response.completed\", \"response\": {\"status\": \"completed\", \"model\": \"gpt-6-astra\", \"output\": [{\"type\": \"message\", \"content\": [{\"type\": \"output_text\", \"text\": \"21\"}]}]}}\n\n"))}, nil
	}))
	protocol := &protocolFixture{t: t}
	service.UseExecution(tasks, runner, checker, t.TempDir())
	exchanges := 0
	attempts := 0
	expectedSourceID := ""
	signedRunInputWithProtocol(t, service, func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/api/accounts/password/verify" {
			if expectedSourceID != "" {
				records, err := private.WorkbenchDocuments(req.Context(), "maintenance:", 1)
				if err != nil || len(records) != 1 {
					return nil, errors.New("maintenance source checkpoint missing")
				}
				var checkpoint struct {
					Attempts map[string]struct {
						SourceRunID string `json:"source_run_id"`
					} `json:"attempts"`
				}
				if json.Unmarshal(records[0].Payload, &checkpoint) != nil || checkpoint.Attempts["41"].SourceRunID != expectedSourceID {
					return nil, errors.New("maintenance selected an ineligible login source")
				}
			}
			attempts++
			if attempts == 1 {
				rejected := upstreamResponse(map[string]any{"error": map[string]any{"code": "invalid_password"}})
				rejected.StatusCode = http.StatusBadRequest
				return rejected, nil
			}
		}
		return protocol.response(req)
	}, func(*http.Request) error { exchanges++; return nil })
	var account map[string]any
	refreshes := 0
	service.UseTransport(transportFunc(func(request *http.Request) (*http.Response, error) {
		if len(useProxy) > 0 && strings.HasSuffix(request.URL.Path, "/proxies/3") {
			return upstreamResponse(map[string]any{"data": platformProxyFixture()}), nil
		}
		if request.URL.Host != "isolated.invalid" {
			return nil, errors.New("unexpected site")
		}
		path := strings.TrimPrefix(request.URL.Path, "/api/v1")
		if request.Method == "GET" {
			if path == "/admin/accounts" {
				items := []any{}
				if account != nil {
					items = append(items, account)
				}
				return upstreamResponse(map[string]any{"data": map[string]any{"items": items, "total": len(items)}}), nil
			}
			return upstreamResponse(map[string]any{"data": account}), nil
		}
		var body map[string]any
		if request.Body != nil {
			decoder := json.NewDecoder(request.Body)
			decoder.UseNumber()
			_ = decoder.Decode(&body)
		}
		switch path {
		case "/admin/accounts":
			account = body
			account["id"] = json.Number("41")
		case "/admin/accounts/41":
			for key, value := range body {
				account[key] = value
			}
		case "/admin/accounts/41/clear-error":
			account["status"] = "active"
			account["error_message"] = ""
		case "/admin/accounts/41/schedulable":
			account["schedulable"] = body["schedulable"]
		case "/admin/openai/accounts/41/refresh":
			refreshes++
			account["credentials"].(map[string]any)["access_token"] = "expired-access"
		default:
			return nil, errors.New("unexpected write")
		}
		return upstreamResponse(map[string]any{"data": account}), nil
	}))
	ctx := context.Background()
	preview, err := service.Preview(ctx, owner, accountworkbench.PreviewInput{Action: "import", Content: "run@example.test----old-password", Promote: true})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.Start(ctx, owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision})
	if err != nil {
		t.Fatal(err)
	}
	run = waitProtocolPrompt(t, service, owner, run.ID, "")
	if err := service.SubmitLoginInput(ctx, owner, run.ID, run.Items[0].ID, accountworkbench.LoginInput{PromptID: run.Items[0].LoginPrompt.ID, Value: "test-password"}); err != nil {
		t.Fatal(err)
	}
	awaitRun(t, runner)
	run, err = service.Run(ctx, owner, run.ID)
	if err != nil || run.Status != "completed" {
		t.Fatalf("initial login import failed: %v %+v", err, run)
	}
	expectedSourceID = run.ID
	// Transient processing records must not replace the durable login source.
	raw, _, err := private.WorkbenchDocument(ctx, "run:"+owner+":"+run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for index, condition := range []string{"target", "expired", "account", "status", "identity", "login", "action"} {
		var document map[string]any
		if err := json.Unmarshal(raw, &document); err != nil {
			t.Fatal(err)
		}
		public := document["public"].(map[string]any)
		row := public["items"].([]any)[0].(map[string]any)
		input := document["items"].([]any)[0].(map[string]any)
		switch condition {
		case "target":
			document["target"].(map[string]any)["BaseURL"] = "https://other.invalid"
		case "expired":
			public["expires_at"] = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
		case "account":
			row["account_id"] = "42"
		case "status":
			row["status"] = "failed"
		case "identity":
			input["credentials"].(map[string]any)["chatgpt_account_id"] = "other-workspace"
		case "login":
			input["login_source"] = ""
		case "action":
			public["action"] = "export"
		}
		id := strings.Repeat(string(rune('a'+index)), 32)
		public["id"] = id
		payload, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := private.SaveWorkbenchDocument(ctx, "run:"+owner+":"+id, 0, payload); err != nil {
			t.Fatal(err)
		}
	}
	if err := private.RevokeSession(ctx, token); err != nil {
		t.Fatal(err)
	}
	if err := service.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	owner, _ = previewOwner(t, private)
	if deleteSource {
		saved, err := service.RememberedAccounts(ctx, owner)
		if err != nil || len(saved) != 1 {
			t.Fatalf("saved login source missing: %v %+v", err, saved)
		}
		if err := service.DeleteRememberedAccount(ctx, owner, saved[0].ID, saved[0].Revision); err != nil {
			t.Fatal(err)
		}
	}
	account["status"] = "error"
	if len(useProxy) > 0 {
		account["proxy_id"] = json.Number("3")
	}
	account["schedulable"] = false
	account["error_message"] = "HTTP 401"
	plan, err := service.PreviewMaintenance(ctx, owner, accountworkbench.MaintenanceSettings{Enabled: true, IntervalMinutes: 5, CooldownMinutes: 10, CheckAfterRepair: true})
	if err != nil {
		t.Fatal(err)
	}
	settings, err := service.ConfigureMaintenance(ctx, owner, accountworkbench.RunConfirmation{ID: plan.ID, Revision: plan.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.CheckMaintenance(ctx, owner, settings.Revision); err != nil {
		t.Fatal(err)
	}
	awaitRun(t, runner)
	result, err := service.Maintenance(ctx, owner)
	if deleteSource {
		if err != nil || len(result.Results) != 1 || result.Results[0].Reason != "manual_login_required" || exchanges != 1 || protocol.bootstraps != 1 {
			t.Fatalf("deleted login source reused: %v %+v exchanges=%d logins=%d", err, result, exchanges, protocol.bootstraps)
		}
		return
	}
	if err != nil || len(result.Results) != 1 || result.Results[0].Status != "repaired" || result.Results[0].Action != "reauthorize" {
		t.Fatalf("reauthorization did not recover: %v %+v", err, result)
	}
	if refreshes != 1 || exchanges != 2 || protocol.bootstraps != 2 {
		t.Fatalf("unexpected refresh or login replay: refresh=%d exchanges=%d logins=%d", refreshes, exchanges, protocol.bootstraps)
	}
	if len(useProxy) > 0 && result.Results[0].Check["transport"] != "oauth-proxy" {
		t.Fatal("maintenance detection bypassed account proxy")
	}
}
