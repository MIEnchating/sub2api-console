package onboarding_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUnknownProbeKeyWithoutReconciliationDirectsBothModelActionsToUnboundKeyCleanup(t *testing.T) {
	for _, action := range []string{"models", "model-options"} {
		t.Run(action, func(t *testing.T) {
			gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				t.Error("model request must not run after an uncertain Key creation")
				http.Error(w, "unexpected model request", http.StatusInternalServerError)
			}))
			t.Cleanup(gateway.Close)
			service, keys, tasks, runner := modelOptionsFixture(t, gateway.URL, false)
			keys.createErr = errors.New("isolated create response lost")
			if _, err := service.EnqueueProbe(t.Context(), action, "options.test", "6", "", ""); err != nil {
				t.Fatal(err)
			}

			runner.run(t.Context())

			completed := tasks.final
			if completed.Status != "failed" || completed.Result["models"] != nil {
				t.Fatalf("uncertain creation must fail without models: %+v", completed)
			}
			if !strings.Contains(completed.Message, "请在无绑定 Key 清理中核对并清理") || strings.Contains(completed.Message, "后续探活将复用") {
				t.Fatalf("missing actionable Key recovery guidance: %s", completed.Message)
			}
			if !strings.Contains(completed.Message, "marker console-probe-") || !strings.Contains(completed.Message, keys.createErr.Error()) {
				t.Fatalf("recovery guidance lost marker or cause: %s", completed.Message)
			}
		})
	}
}
