package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestMixedInputAPIPreviewKeepsCredentialsPrivateAndRejectsUnconfirmedStart(t *testing.T) {
	router, token := workbenchRouter(t)
	body, _ := json.Marshal(accountworkbench.WorkbenchRunInput{Content: "rt_private_mixed\nowner@example.com----PrivatePassword123!", ExportOnly: true})
	req := httptest.NewRequest(http.MethodPost, "http://console.test/api/account-workbench/runs/preview", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://console.test")
	req.AddCookie(&http.Cookie{Name: "sub2api_console_session", Value: token})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("preview status = %d", response.Code)
	}
	if strings.Contains(response.Body.String(), "rt_private_mixed") || strings.Contains(response.Body.String(), "PrivatePassword123!") {
		t.Fatal("mixed preview leaked credentials")
	}
	var preview accountworkbench.WorkbenchRunPreview
	if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if len(preview.Items) != 2 || preview.ID == "" || !preview.ExportOnly {
		t.Fatalf("mixed preview = %+v", preview)
	}
	req = httptest.NewRequest(http.MethodPost, "http://console.test/api/account-workbench/runs", strings.NewReader(`{"preview_id":"`+preview.ID+`","confirmed":false}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://console.test")
	req.AddCookie(&http.Cookie{Name: "sub2api_console_session", Value: token})
	response = httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusConflict {
		t.Fatalf("unconfirmed start status = %d", response.Code)
	}
}

func TestHistoryAndMixedReadAPIsValidateInputsAndNeverCache(t *testing.T) {
	router, token := workbenchRouter(t)
	for _, item := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "history/active", "", 200},
		{"POST", "history/query", `{"emails":["Owner <owner@example.com>"]}`, 422},
		{"POST", "history/query", `{"offset":-1}`, 422},
		{"POST", "history/query", `{"limit":50,"emails":["owner@example.com"]}`, 200},
		{"GET", "runs/missing", "", 404},
		{"POST", "runs/missing/preview", "", 404},
		{"DELETE", "runs/missing", "", 404},
		{"POST", "queue-recoveries/missing/mixed", `{"revision":"1","confirmed":true}`, 422},
		{"POST", "queue-recoveries/missing/mixed", `{"revision":1,"confirmed":false}`, 409},
		{"POST", "queue-recoveries/missing/mixed", `{"revision":1,"confirmed":true}`, 409},
	} {
		t.Run(item.method+item.path+item.body, func(t *testing.T) {
			req := httptest.NewRequest(item.method, "http://console.test/api/account-workbench/"+item.path, strings.NewReader(item.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", "http://console.test")
			req.AddCookie(&http.Cookie{Name: "sub2api_console_session", Value: token})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != item.status || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestMixedQueueRecoveryRequiresAuthenticatedConsoleSession(t *testing.T) {
	router, _ := workbenchRouter(t)
	response := profileExportRequest(router, "", http.MethodPost, "queue-recoveries/missing/mixed", `{"revision":1,"confirmed":true}`)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous mixed recovery returned %d", response.Code)
	}
}
