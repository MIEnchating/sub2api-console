package accountworkbench_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func previewConversion(t *testing.T, f *importFixture, input accountworkbench.PreviewInput) accountworkbench.Preview {
	t.Helper()
	input.ExportOnly = true
	view, err := f.service.Preview(context.Background(), "convert-owner", input)
	if err != nil || len(view.Errors) != 0 || view.ID == "" {
		t.Fatalf("conversion preview = %+v, %v", view, err)
	}
	return view
}

func TestInputConversionWithCompleteCredentialsUsesNoRemoteRequestsAndPreservesExactTemplateValues(t *testing.T) {
	f, directory := exportFixture(t)
	var requests atomic.Int32
	f.service.UseTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, errors.New("remote request forbidden in complete-input conversion")
	}))
	template, err := f.service.SaveTemplate(context.Background(), "", accountworkbench.TemplateInput{
		Name: "Private conversion", Config: configstore.WorkbenchTemplateConfig{
			"group_ids": json.RawMessage(`[7]`), "concurrency": json.RawMessage(`4`),
			"rate_multiplier": json.RawMessage(`0.123456789012345678901`),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	content := `[` + importedJSON + `,{"name":"Second","credentials":{"access_token":"second-private","chatgpt_account_id":"workspace-2","chatgpt_user_id":"user-2"}}]`
	view := previewConversion(t, f, accountworkbench.PreviewInput{Content: content, TemplateID: template.ID, CheckAfterImport: true})
	if !view.ExportOnly || view.CheckAfterImport || len(view.Items) != 2 || view.Items[0].TemplateRevision != template.Revision || view.Items[0].Duplicate || view.Items[0].AccountID != "" {
		t.Fatalf("unexpected conversion scope = %+v", view)
	}
	if _, err := f.service.ExportInput(context.Background(), "convert-owner", view.ID, true); err != nil {
		t.Fatal(err)
	}
	task := f.await(t)
	if task.Status != "succeeded" || task.Operation != "account-workbench-convert" {
		t.Fatalf("conversion task = %+v", task)
	}
	artifacts, err := f.service.Exports(context.Background(), "convert-owner")
	if err != nil || len(artifacts) != 1 || artifacts[0].Count != 2 {
		t.Fatalf("conversion artifacts = %+v, %v", artifacts, err)
	}
	payload, err := os.ReadFile(filepath.Join(directory, artifacts[0].ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Type     string           `json:"type"`
		Version  int              `json:"version"`
		Accounts []map[string]any `json:"accounts"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.UseNumber()
	if err := decoder.Decode(&data); err != nil {
		t.Fatal(err)
	}
	if data.Type != "sub2api-data" || data.Version != 1 || len(data.Accounts) != 2 {
		t.Fatal("conversion omitted the transferable Sub2API account envelope")
	}
	for _, account := range data.Accounts {
		if account["rate_multiplier"] != json.Number("0.123456789012345678901") || account["concurrency"] != json.Number("4") {
			t.Fatal("conversion did not preserve reviewed template values")
		}
		groups, _ := json.Marshal(account["group_ids"])
		if string(groups) != `[7]` {
			t.Fatalf("template groups = %s", groups)
		}
	}
	credentials, ok := data.Accounts[0]["credentials"].(map[string]any)
	if !ok || credentials["access_token"] != "access-new-private" || credentials["refresh_token"] != "rt_new_private" {
		t.Fatal("private conversion lost source credentials")
	}
	public, _ := json.Marshal([]any{view, task, artifacts})
	for _, secret := range []string{"access-new-private", "rt_new_private", "second-private", "test-admin-key"} {
		if strings.Contains(string(public), secret) {
			t.Fatalf("public conversion metadata leaked %s", secret)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("complete input conversion made %d remote requests", requests.Load())
	}
}

func TestInputConversionWithRefreshTokenMaterializesOnceAndNeverReadsOrWritesAccounts(t *testing.T) {
	f, directory := exportFixture(t)
	var requests atomic.Int32
	f.service.UseTransport(oauthTransportFunc(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/admin/openai/refresh-token" {
			return nil, errors.New("unexpected request during refresh-token conversion")
		}
		return http.DefaultTransport.RoundTrip(r)
	}))
	view := previewConversion(t, f, accountworkbench.PreviewInput{Content: "rt_input_private"})
	if requests.Load() != 1 || view.Items[0].Email != "owner@example.com" {
		t.Fatal("refresh-only preview did not materialize stable account identity once")
	}
	if _, err := f.service.ExportInput(context.Background(), "convert-owner", view.ID, true); err != nil {
		t.Fatal(err)
	}
	if task := f.await(t); task.Status != "succeeded" {
		t.Fatalf("refresh conversion task = %+v", task)
	}
	if requests.Load() != 1 {
		t.Fatalf("conversion repeated refresh or contacted accounts: requests=%d", requests.Load())
	}
	artifacts, err := f.service.Exports(context.Background(), "convert-owner")
	if err != nil || len(artifacts) != 1 {
		t.Fatalf("conversion artifacts = %+v, %v", artifacts, err)
	}
	payload, err := os.ReadFile(filepath.Join(directory, artifacts[0].ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), `"refresh_token":"rt_new_private"`) || strings.Contains(string(payload), "rt_input_private") {
		t.Fatal("conversion did not export rotated credentials from its reviewed preview")
	}
}

func TestInputConversionWhenRefreshFailsDoesNotRetainAnExecutablePartialBatch(t *testing.T) {
	f, directory := exportFixture(t)
	var requests atomic.Int32
	f.service.UseTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return oauthResponse(`{"code":400,"message":"invalid_grant"}`), nil
	}))
	view, err := f.service.Preview(context.Background(), "convert-owner", accountworkbench.PreviewInput{ExportOnly: true, Content: `["rt_invalid",` + importedJSON + `]`})
	if err != nil || len(view.Errors) != 1 || view.ID != "" || len(view.Items) != 0 {
		t.Fatalf("failed refresh preview = %+v, %v", view, err)
	}
	if _, err := f.service.ExportInput(context.Background(), "convert-owner", view.ID, true); !errors.Is(err, accountworkbench.ErrPreview) {
		t.Fatalf("partial preview conversion = %v", err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 0 || requests.Load() != 1 {
		t.Fatalf("failed refresh artifact count=%d requests=%d error=%v", len(entries), requests.Load(), err)
	}
}

func TestInputConversionWhenPrivateStorageFailsFinishesItsResultItem(t *testing.T) {
	f, _ := exportFixture(t)
	view := previewConversion(t, f, accountworkbench.PreviewInput{Content: importedJSON})
	if err := f.service.CloseExports(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ExportInput(context.Background(), "convert-owner", view.ID, true); err != nil {
		t.Fatal(err)
	}
	task := f.await(t)
	if task.Status != "failed" {
		t.Fatalf("storage failure task = %s", task.Status)
	}
	rows, ok := task.Result["items"].([]accountworkbench.ResultItem)
	if !ok || len(rows) != 1 || rows[0].Status != "failed" || rows[0].Message == "" || rows[0].Report != nil {
		t.Fatalf("terminal conversion result = %+v", task.Result)
	}
}
