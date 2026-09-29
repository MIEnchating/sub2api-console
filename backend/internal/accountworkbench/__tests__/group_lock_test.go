package accountworkbench_test

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestLockedExistingAccountCannotApplyWorkbenchTemplateGroups(t *testing.T) {
	service, private := fixture(t, `{"data":`+sourceAccount+`}`)
	store, err := business.Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncManagementSnapshot(t.Context(), []map[string]any{{"id": "41", "name": "account", "platform": "openai", "group_ids": []any{"6"}}}, []map[string]any{{"id": "6", "name": "original", "platform": "openai"}}, "test"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAccountGroupsLocked(t.Context(), "41", true, "test"); err != nil {
		t.Fatal(err)
	}
	service.UseAccountGroupProtection(store)
	owner, _ := previewOwner(t, private)
	runner, _ := runTasks(t, service)
	source, err := service.TemplateSource(context.Background(), "41")
	if err != nil {
		t.Fatal(err)
	}
	library, err := service.SaveTemplate(context.Background(), accountworkbench.TemplateInput{Name: "导入模板", SourceID: "41", SourceVersion: source.SourceVersion})
	if err != nil {
		t.Fatal(err)
	}
	current := map[string]any{"id": json.Number("41"), "platform": "openai", "type": "oauth", "name": "原账号", "status": "active", "schedulable": true, "group_ids": []any{json.Number("6")}, "credentials": map[string]any{"chatgpt_account_id": "run-workspace", "chatgpt_user_id": "run-user"}}
	writes := 0
	service.UseTransport(transportFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet {
			writes++
			t.Fatalf("locked account reached remote write: %s", request.URL.Path)
		}
		if strings.HasSuffix(request.URL.Path, "/admin/accounts") {
			return upstreamResponse(map[string]any{"data": map[string]any{"items": []any{current}, "total": 1}}), nil
		}
		return upstreamResponse(map[string]any{"data": current}), nil
	}))
	preview, err := service.Preview(context.Background(), owner, accountworkbench.PreviewInput{Action: "import", Content: signedRunInput(t, service), TemplateID: library.PreferredID})
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
	if result.Status == "completed" || writes != 0 || !strings.Contains(result.Items[0].Message, "分组已锁定") {
		t.Fatalf("status=%s writes=%d message=%s", result.Status, writes, result.Items[0].Message)
	}
}
