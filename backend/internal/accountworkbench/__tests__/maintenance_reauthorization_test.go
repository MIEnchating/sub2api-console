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

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

func TestMaintenanceReauthorizesOnceWhenRefreshedAccountStillReturns401(t *testing.T) {
	service, private := fixture(t, `{}`)
	owner, _ := previewOwner(t, private)
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
	view := checker.Configuration()
	payload := view.Active.Payload
	payload.SolProfile.Quick = []modelcheck.ProbeDefinition{{ID: "number", Kind: "numeric", Question: "Return the number seven.", Clusters: []modelcheck.NumericClusterDefinition{{ID: "seven", Center: 7}}, Tolerance: &modelcheck.NumericTolerance{Value: 0, Mode: "absolute"}, Weights: map[string][]float64{"seven": {0, -10, -10}}}}
	payload.SolProfile.Reserve = []modelcheck.ProbeDefinition{{ID: "reserve", Kind: "numeric", Question: "Return the number seven.", Clusters: []modelcheck.NumericClusterDefinition{{ID: "seven", Center: 7}}, Tolerance: &modelcheck.NumericTolerance{Value: 0, Mode: "absolute"}, Weights: map[string][]float64{"seven": {0, -10, -10}}}}
	draft, err := checker.SaveDraft(context.Background(), modelcheck.SaveDraftRequest{ExpectedFingerprint: view.Active.Fingerprint, Payload: payload}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = checker.PublishDraft(context.Background(), modelcheck.PublishRequest{ExpectedFingerprint: draft.Draft.Fingerprint}, "test"); err != nil {
		t.Fatal(err)
	}
	checker.UseOAuthTransport(transportFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://chatgpt.com/backend-api/codex/responses" {
			return nil, errors.New("unexpected check endpoint")
		}
		if request.Header.Get("Authorization") == "Bearer expired-access" {
			return &http.Response{StatusCode: 401, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"unauthorized"}}`))}, nil
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.output_text.delta\",\"delta\":\"[\\\"7\\\"]\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"model\":\"gpt-5.6-sol\"}}\n\n"))}, nil
	}))
	browser := &loginBrowser{stages: []string{"email", "password"}}
	service.UseExecution(tasks, runner, checker, browser, t.TempDir())
	exchanges := 0
	signedRunInput(t, service, func(*http.Request) error { exchanges++; return nil })
	var account map[string]any
	refreshes := 0
	service.UseTransport(transportFunc(func(request *http.Request) (*http.Response, error) {
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
	preview, err := service.Preview(ctx, owner, accountworkbench.PreviewInput{Action: "import", Content: "run@example.test----test-password", Promote: true})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.Start(ctx, owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision})
	if err != nil {
		t.Fatal(err)
	}
	awaitRun(t, runner)
	run, err = service.Run(ctx, owner, run.ID)
	if err != nil || run.Status != "completed" {
		t.Fatalf("initial login import failed: %v %+v", err, run)
	}
	account["status"] = "error"
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
	if err != nil || len(result.Results) != 1 || result.Results[0].Status != "repaired" || result.Results[0].Action != "reauthorize" {
		t.Fatalf("reauthorization did not recover: %v %+v", err, result)
	}
	if refreshes != 1 || exchanges != 2 || browser.opens != 2 {
		t.Fatalf("unexpected refresh or login replay: refresh=%d exchanges=%d browsers=%d", refreshes, exchanges, browser.opens)
	}
}
