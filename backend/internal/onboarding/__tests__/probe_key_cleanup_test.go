package onboarding_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/onboarding"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

type reconciledProbeKeys struct{ *modelOptionsKeys }

func (keys *reconciledProbeKeys) ReconcileCreatedKey(_ context.Context, _ configstore.AuthRecord, _ string, group string) (upstreamsync.CreatedKey, bool, error) {
	return upstreamsync.CreatedKey{KeyID: "reconciled-91", GroupID: group, Secret: testKey}, true, nil
}

func TestUncertainProbeCreationWithConfirmedKeyAndFailedDeletionRetainsBindingForRetry(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"model":"model-a","output_text":"hello"}`))
	}))
	t.Cleanup(gateway.Close)
	_, keys, _, _, repo, private := modelOptionsFixtureStores(t, gateway.URL, false)
	keys.createErr = errors.New("create response lost")
	keys.deleteErr = errors.New("HTTP 403")
	service := onboarding.New(repo, private, &reconciledProbeKeys{keys}, nil)
	if _, err := service.Probe(t.Context(), "options.test", "6", "model-a"); err == nil {
		t.Fatal("uncertain creation must not claim that generation ran")
	}
	service = onboarding.New(repo, private, keys, nil)
	result, err := service.Probe(t.Context(), "options.test", "6", "model-a")
	if err != nil || result.Status != "passed" || len(keys.revealed) != 1 || keys.revealed[0] != "reconciled-91" {
		t.Fatalf("retry did not use reconciled stable binding: result=%+v err=%v ids=%v", result, err, keys.revealed)
	}
}

func TestProbeDeleteFailurePreservesSuccessfulResultAndReusesBindingAfterRestart(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"model":"model-a","output_text":"hello"}`))
	}))
	t.Cleanup(gateway.Close)
	service, keys, tasks, runner, repo, private := modelOptionsFixtureStores(t, gateway.URL, false)
	keys.deleteErr = errors.New("HTTP 403: Operation failed")
	for range 2 {
		if _, err := service.EnqueueProbe(t.Context(), "probe", "options.test", "6", "model-a", "default"); err != nil {
			t.Fatal(err)
		}
		runner.run(t.Context())
		if tasks.final.Status != "succeeded" {
			t.Fatalf("successful generation must survive failed deletion: %+v", tasks.final)
		}
		raw, err := json.Marshal(tasks.final)
		if err != nil || strings.Contains(string(raw), testKey) {
			t.Fatal("task must not expose credentials")
		}
		service = onboarding.New(repo, private, keys, tasks)
		service.UseTaskRunner(runner)
	}
	if len(keys.created) != 1 || len(keys.deleted) != 1 || len(keys.revealed) != 1 || keys.revealed[0] != keys.created[0] {
		t.Fatalf("retained key must be reused without creating or deleting again: created=%v deleted=%v", keys.created, keys.deleted)
	}
}

func TestRetainedProbeKeyReadFailureDoesNotCreateReplacementUnlessMissing(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "read_failure", true: "confirmed_missing"}[missing], func(t *testing.T) {
			gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"model":"model-a","output_text":"hello"}`))
			}))
			t.Cleanup(gateway.Close)
			service, keys, _, _, repo, private := modelOptionsFixtureStores(t, gateway.URL, false)
			keys.deleteErr = errors.New("HTTP 403")
			if _, err := service.Probe(t.Context(), "options.test", "6", "model-a"); err != nil {
				t.Fatal(err)
			}
			keys.revealErr = errors.New("HTTP 503")
			if missing {
				keys.revealErr = upstreamsync.ErrKeyNotFound
				keys.byMarker = map[string]string{}
			}
			service = onboarding.New(repo, private, keys, nil)
			_, err := service.Probe(t.Context(), "options.test", "6", "model-a")
			if missing {
				if err != nil || len(keys.created) != 2 {
					t.Fatalf("missing Key should be replaced: err=%v created=%v", err, keys.created)
				}
			} else if err == nil || len(keys.created) != 1 {
				t.Fatalf("read failure must preserve original binding: err=%v created=%v", err, keys.created)
			}
		})
	}
}

func TestProbeGenerationFailureRemainsFailureWhenKeyBindingIsRetained(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "generation unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(gateway.Close)
	service, keys, _, _ := modelOptionsFixture(t, gateway.URL, false)
	keys.deleteErr = errors.New("HTTP 403")
	result, err := service.Probe(t.Context(), "options.test", "6", "model-a")
	if err == nil || result.Status != "failed" || !strings.Contains(err.Error(), "503") || strings.Contains(err.Error(), "403") {
		t.Fatalf("generation failure must remain the cause: result=%+v err=%v", result, err)
	}
}

func TestProbeDeleteFailureWithUnavailablePrivateStoreReportsPersistenceFailure(t *testing.T) {
	service, keys, _, _, _, private := modelOptionsFixtureStores(t, "https://unused.test", false)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = private.Close()
		_, _ = w.Write([]byte(`{"model":"model-a","output_text":"hello"}`))
	}))
	t.Cleanup(gateway.Close)
	auth, err := private.AuthRecord(t.Context(), "options.test")
	if err != nil {
		t.Fatal(err)
	}
	auth.BaseURL = gateway.URL
	if err := private.SaveAuthRecord(t.Context(), *auth, nil); err != nil {
		t.Fatal(err)
	}
	keys.deleteErr = errors.New("HTTP 403")
	_, err = service.Probe(t.Context(), "options.test", "6", "model-a")
	if err == nil || !strings.Contains(err.Error(), "保存 Key 绑定失败") {
		t.Fatalf("failed persistence must not claim reusable binding: %v", err)
	}
}

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
