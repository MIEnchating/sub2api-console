package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestWorkbenchLocalExportRoutesRequireConsoleSessionAndDoNotExposeFiles(t *testing.T) {
	router, token := workbenchRouter(t, true)
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		path := "local-exports"
		if method == http.MethodDelete {
			path += "/" + strings.Repeat("a", 32)
		}
		response := profileExportRequest(router, "", method, path, "")
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated local export route returned %d", response.Code)
		}
	}
	list := profileExportRequest(router, token, http.MethodGet, "local-exports", "")
	if list.Code != http.StatusOK || strings.TrimSpace(list.Body.String()) != "[]" || list.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unconfigured local list = %d %s", list.Code, list.Body.String())
	}
	managed := profileExportRequest(router, token, http.MethodGet, "exports", "")
	if managed.Code != http.StatusConflict {
		t.Fatal("managed artifact listing unexpectedly worked without target")
	}
	file := profileExportRequest(router, token, http.MethodGet, "local-exports/"+strings.Repeat("a", 32), "")
	if file.Code != http.StatusNotFound {
		t.Fatal("local artifact route allowed browser download")
	}
}

func TestWorkbenchLocalPreviewWithoutManagementRejectsImportAndMarksRefreshOnly(t *testing.T) {
	router, token := workbenchRouter(t, true)
	input, _ := json.Marshal(accountworkbench.PreviewInput{Scope: accountworkbench.ScopeLocalExport, ExportOnly: true, Content: `{"name":"Local account","credentials":{"access_token":"private-local-access","refresh_token":"rt_local_private","chatgpt_user_id":"user-1","chatgpt_account_id":"workspace-1"}}`})
	response := profileExportRequest(router, token, http.MethodPost, "preview", string(input))
	var view accountworkbench.Preview
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil || response.Code != http.StatusOK || view.ID == "" || view.Scope != accountworkbench.ScopeLocalExport || view.Target != "" || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("local preview = %d %s, %v", response.Code, response.Body.String(), err)
	}
	if strings.Contains(response.Body.String(), "private-local-access") || strings.Contains(response.Body.String(), "rt_local_private") {
		t.Fatal("local preview returned credentials")
	}
	denied := profileExportRequest(router, token, http.MethodPost, "import", `{"preview_id":"`+view.ID+`","confirmed":true}`)
	if denied.Code != http.StatusConflict {
		t.Fatalf("managed import accepted local scope: %d", denied.Code)
	}
	refresh := profileExportRequest(router, token, http.MethodPost, "preview", `{"scope":"local-export","export_only":true,"content":"rt_local_private"}`)
	var pending accountworkbench.Preview
	if err := json.Unmarshal(refresh.Body.Bytes(), &pending); err != nil || refresh.Code != http.StatusOK || len(pending.Errors) != 0 || pending.ID == "" || len(pending.Items) != 1 || !pending.Items[0].RefreshRequired {
		t.Fatalf("local refresh-only preview = %d %s", refresh.Code, refresh.Body.String())
	}
	checkpoints := profileExportRequest(router, token, http.MethodGet, "oauth-checkpoints?scope=local-export", "")
	if checkpoints.Code != http.StatusOK || strings.TrimSpace(checkpoints.Body.String()) != "[]" {
		t.Fatalf("local checkpoint list = %d %s", checkpoints.Code, checkpoints.Body.String())
	}
	invalidScope := profileExportRequest(router, token, http.MethodGet, "oauth-checkpoints?scope=other", "")
	if invalidScope.Code != http.StatusConflict {
		t.Fatal("checkpoint listing accepted an unknown scope")
	}
}
