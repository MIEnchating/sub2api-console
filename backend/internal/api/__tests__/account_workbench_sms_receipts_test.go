package api_test

import (
	"net/http"
	"testing"
)

func TestWorkbenchSMSReceiptRoutesRequireSessionAndReturnNoStore(t *testing.T) {
	router, token := workbenchRouter(t)
	for _, route := range []struct{ method, path string }{{"GET", "sms/receipts"}, {"POST", "sms/receipts/missing/inspect"}} {
		response := profileExportRequest(router, "", route.method, route.path, `{}`)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("unprotected receipt route: %s = %d", route.path, response.Code)
		}
	}
	for _, item := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "sms/receipts", "", http.StatusOK},
		{"POST", "sms/receipts/missing/inspect", `{"provider":123}`, http.StatusUnprocessableEntity},
		{"POST", "sms/receipts/missing/inspect", `{"provider":"smsbower","api_key":"private-test-key"}`, http.StatusConflict},
	} {
		response := profileExportRequest(router, token, item.method, item.path, item.body)
		if response.Code != item.status || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("receipt response %d %s", response.Code, response.Body.String())
		}
	}
}

func TestWorkbenchSMSReceiptListRejectsInvalidScopeWithoutReturningOrders(t *testing.T) {
	router, token := workbenchRouter(t)
	response := profileExportRequest(router, token, http.MethodGet, "sms/receipts?scope=invalid", "")
	if response.Code != http.StatusConflict {
		t.Fatalf("invalid scope returned %d: %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("invalid scope response can be cached")
	}
}
