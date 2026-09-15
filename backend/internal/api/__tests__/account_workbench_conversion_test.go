package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestWorkbenchInputConversionRequiresAuthentication(t *testing.T) {
	router, _ := workbenchRouter(t)
	req := httptest.NewRequest(http.MethodPost, "http://console.test/api/account-workbench/exports/from-input", strings.NewReader(`{"preview_id":"missing","confirmed":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://console.test")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated conversion status = %d", response.Code)
	}
}

func TestWorkbenchInputConversionReturnsAcceptedTaskAndConsumesOnlyConfirmedPreview(t *testing.T) {
	router, token := workbenchRouter(t)
	call := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "http://console.test/api/account-workbench/"+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://console.test")
		req.AddCookie(&http.Cookie{Name: "sub2api_console_session", Value: token})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	input, err := json.Marshal(accountworkbench.PreviewInput{ExportOnly: true, Content: `{"name":"Private account","credentials":{"access_token":"conversion-access-private","refresh_token":"rt_conversion_private","chatgpt_account_id":"workspace-1","chatgpt_user_id":"user-1"}}`})
	if err != nil {
		t.Fatal(err)
	}
	response := call("preview", string(input))
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("conversion preview response = %d %s", response.Code, response.Body.String())
	}
	var view accountworkbench.Preview
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil || view.ID == "" || !view.ExportOnly || len(view.Items) != 1 {
		t.Fatalf("conversion preview = %+v, %v", view, err)
	}
	if strings.Contains(response.Body.String(), "conversion-access-private") || strings.Contains(response.Body.String(), "rt_conversion_private") {
		t.Fatal("conversion preview exposed credentials")
	}
	for _, attempt := range []struct {
		body   string
		status int
	}{
		{`{"preview_id":"` + view.ID + `","confirmed":"true"}`, http.StatusUnprocessableEntity},
		{`{"preview_id":"` + view.ID + `","confirmed":false}`, http.StatusConflict},
		{`{"preview_id":"` + view.ID + `","confirmed":true}`, http.StatusAccepted},
		{`{"preview_id":"` + view.ID + `","confirmed":true}`, http.StatusConflict},
	} {
		response := call("exports/from-input", attempt.body)
		if response.Code != attempt.status || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("conversion response = %d %s", response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), "conversion-access-private") || strings.Contains(response.Body.String(), "rt_conversion_private") {
			t.Fatal("conversion task exposed credentials")
		}
		if attempt.status == http.StatusAccepted {
			var task struct {
				ID        string `json:"id"`
				Operation string `json:"operation"`
				Status    string `json:"status"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &task); err != nil || task.ID == "" || task.Operation != "account-workbench-convert" || task.Status != "queued" {
				t.Fatalf("accepted conversion = %+v, %v", task, err)
			}
		}
	}
}
