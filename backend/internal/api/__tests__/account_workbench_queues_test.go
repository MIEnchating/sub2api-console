package api_test

import (
	"net/http"
	"testing"
)

func TestWorkbenchQueueRoutesRequireAuthenticatedSession(t *testing.T) {
	router, _ := workbenchRouter(t)
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "queue-recoveries"},
		{http.MethodPost, "queue-recoveries/missing/oauth"},
		{http.MethodDelete, "queue-recoveries/missing"},
	} {
		t.Run(route.method, func(t *testing.T) {
			response := profileExportRequest(router, "", route.method, route.path, `{"revision":1,"confirmed":true}`)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("unauthenticated queue response = %d", response.Code)
			}
		})
	}
}

func TestWorkbenchQueueListAndInvalidActionsNeverCachePrivateState(t *testing.T) {
	router, token := workbenchRouter(t)
	for _, attempt := range []struct {
		name, method, path, body string
		status                   int
	}{
		{"empty managed list", http.MethodGet, "queue-recoveries", "", http.StatusOK},
		{"empty local list", http.MethodGet, "queue-recoveries?scope=local-export", "", http.StatusOK},
		{"invalid scope", http.MethodGet, "queue-recoveries?scope=invalid", "", http.StatusConflict},
		{"resume string revision", http.MethodPost, "queue-recoveries/missing/oauth", `{"revision":"1","confirmed":true}`, http.StatusUnprocessableEntity},
		{"resume unconfirmed", http.MethodPost, "queue-recoveries/missing/oauth", `{"revision":1,"confirmed":false}`, http.StatusConflict},
		{"resume zero revision", http.MethodPost, "queue-recoveries/missing/oauth", `{"revision":0,"confirmed":true}`, http.StatusConflict},
		{"resume missing source", http.MethodPost, "queue-recoveries/missing/oauth", `{"revision":1,"confirmed":true}`, http.StatusConflict},
		{"delete string revision", http.MethodDelete, "queue-recoveries/missing", `{"revision":"1","confirmed":true}`, http.StatusUnprocessableEntity},
		{"delete unconfirmed", http.MethodDelete, "queue-recoveries/missing", `{"revision":1,"confirmed":false}`, http.StatusConflict},
		{"delete zero revision", http.MethodDelete, "queue-recoveries/missing", `{"revision":0,"confirmed":true}`, http.StatusConflict},
		{"delete missing source", http.MethodDelete, "queue-recoveries/missing", `{"revision":1,"confirmed":true}`, http.StatusConflict},
	} {
		t.Run(attempt.name, func(t *testing.T) {
			response := profileExportRequest(router, token, attempt.method, attempt.path, attempt.body)
			if response.Code != attempt.status || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("queue response = %d %s", response.Code, response.Body.String())
			}
			if attempt.status == http.StatusOK && response.Body.String() != "[]" {
				t.Fatal("empty queue list was not an array")
			}
		})
	}
}
