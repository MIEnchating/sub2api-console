package accountworkbench_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

// Management account reads redact OAuth secrets. Import verification must use
// the stable identity and safe configuration fields instead of comparing the
// redacted credentials object byte-for-byte.
func TestImportAcceptsRedactedCredentialsOnReadback(t *testing.T) {
	service, store := fixture(t, `{}`)
	owner, _ := previewOwner(t, store)
	runner, _ := runTasks(t, service)
	var account map[string]any
	service.UseTransport(transportFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != "isolated.invalid" {
			return nil, fmt.Errorf("unexpected management host")
		}
		path := strings.TrimPrefix(request.URL.Path, "/api/v1")
		if request.Method == http.MethodGet {
			if path == "/admin/accounts" {
				items := []any{}
				if account != nil {
					items = append(items, redactedAccount(account))
				}
				return upstreamResponse(map[string]any{"data": map[string]any{"items": items, "total": len(items)}}), nil
			}
			return upstreamResponse(map[string]any{"data": redactedAccount(account)}), nil
		}
		var body map[string]any
		decoder := json.NewDecoder(request.Body)
		decoder.UseNumber()
		if err := decoder.Decode(&body); err != nil {
			return nil, err
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
		case "/admin/accounts/41/schedulable":
			account["schedulable"] = body["schedulable"]
		default:
			return nil, fmt.Errorf("unexpected write %s", path)
		}
		return upstreamResponse(map[string]any{"data": account}), nil
	}))

	preview, err := service.Preview(context.Background(), owner, accountworkbench.PreviewInput{Action: "import", Content: signedRunInput(t, service), Promote: true})
	if err != nil {
		t.Fatal(err)
	}
	queued, err := service.Start(context.Background(), owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision})
	if err != nil {
		t.Fatal(err)
	}
	awaitRun(t, runner)
	result, err := service.Run(context.Background(), owner, queued.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "completed" || result.Items[0].Status != "completed" {
		t.Fatalf("redacted credentials were treated as unapplied: %+v", result.Items[0])
	}
}

func redactedAccount(account map[string]any) map[string]any {
	if account == nil {
		return nil
	}
	copy := map[string]any{}
	for key, value := range account {
		copy[key] = value
	}
	credentials := map[string]any{}
	for key, value := range objectForTest(account["credentials"]) {
		credentials[key] = value
	}
	delete(credentials, "access_token")
	delete(credentials, "refresh_token")
	delete(credentials, "id_token")
	copy["credentials"] = credentials
	copy["credentials_status"] = map[string]any{"has_access_token": true, "has_refresh_token": true}
	return copy
}

// Keep this test independent from private account-workbench implementation
// helpers while preserving the JSON shape returned by the management API.
func objectForTest(value any) map[string]any {
	result, _ := value.(map[string]any)
	return result
}
