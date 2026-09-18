package modelcheck_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestOAuthAnimationRejectsUnsafeIncompleteAndSensitiveResponses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"unsafe SVG", 200, `{"status":"completed","output_text":"<svg><script>alert(1)</script></svg>"}`},
		{"external resource", 200, `{"status":"completed","output_text":"<svg><image href=\"https://example.invalid/image.png\"/></svg>"}`},
		{"incomplete stream", 200, fmt.Sprintf("data: {\"type\":\"response.output_text.delta\",\"delta\":%q}\n\n", fixtureSVG)},
		{"failed HTTP", 401, `{"error":{"message":"isolated-oauth-access-token"}}`},
		{"business failure", 200, `{"status":"failed","error":{"message":"isolated-refresh-token"}}`},
		{"echoed credential", 200, `{"status":"completed","output_text":"<svg><text>isolated-refresh-token</text></svg>"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, _ := oauthAccountFixture(t, oauthAnimationAccount)
			f.service.UseOAuthTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) {
				return oauthResponse(tc.status, "text/event-stream", tc.body), nil
			}))
			if _, err := f.service.EnqueueAnimation(context.Background(), request("1")); err != nil {
				t.Fatal(err)
			}
			task := finished(t, f)
			rows := task.Result["animations"].([]modelcheck.AnimationResult)
			if task.Status != "failed" || rows[0].SVG != "" || rows[0].Error == "" {
				t.Fatalf("unsafe animation result: %#v", rows)
			}
			raw, err := json.Marshal(task)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), oauthFixtureToken) || strings.Contains(string(raw), "isolated-refresh-token") {
				t.Error("failed animation exposed credentials")
			}
		})
	}
}

func TestOAuthAnimationRejectsQueuedTargetAndAccountChangesBeforeGeneration(t *testing.T) {
	for _, changed := range []string{"target", "account"} {
		t.Run(changed, func(t *testing.T) {
			f, private := oauthAccountFixture(t, oauthAnimationAccount)
			runner := &deferredRunner{}
			f.service.UseTaskRunner(runner)
			f.service.UseOAuthTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) {
				t.Error("changed configuration reached generation endpoint")
				return oauthResponse(500, "application/json", `{}`), nil
			}))
			if _, err := f.service.EnqueueAnimation(context.Background(), request("1")); err != nil {
				t.Fatal(err)
			}
			if changed == "target" {
				private.target.AdminKey = "changed-key"
			} else {
				platform := "anthropic"
				f.catalog.details["1"].Platform = &platform
			}
			runner.runs[0](context.Background())
			task := finished(t, f)
			rows := task.Result["animations"].([]modelcheck.AnimationResult)
			if task.Status != "failed" || !strings.Contains(rows[0].Error, "已变化") {
				t.Fatalf("changed configuration accepted: %#v", rows)
			}
		})
	}
}

func TestOAuthAnimationRejectsUnavailableCredentialsAndConfiguredProxy(t *testing.T) {
	for _, remote := range []string{
		`{"id":1,"type":"oauth","platform":"openai","credentials":{}}`,
		`{"id":2,"type":"oauth","platform":"openai"}`,
		`{"id":1,"type":"apikey","platform":"openai"}`,
		`{"id":1,"type":"oauth","platform":"openai","proxy_id":7}`,
	} {
		t.Run(remote, func(t *testing.T) {
			f, _ := oauthAccountFixture(t, remote)
			f.service.UseOAuthTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) {
				t.Error("invalid remote account reached generation endpoint")
				return oauthResponse(500, "application/json", `{}`), nil
			}))
			if _, err := f.service.EnqueueAnimation(context.Background(), request("1")); err != nil {
				t.Fatal(err)
			}
			if task := finished(t, f); task.Status != "failed" {
				t.Fatalf("unavailable account accepted: %#v", task)
			}
		})
	}
}

func TestOAuthCombinedCancellationStopsAnimationAndKeepsAccountReserved(t *testing.T) {
	f, _ := oauthAccountFixture(t, oauthAnimationAccount)
	started := make(chan struct{}, 1)
	f.service.UseOAuthTransport(oauthTransportFunc(func(r *http.Request) (*http.Response, error) {
		started <- struct{}{}
		<-r.Context().Done()
		return nil, r.Context().Err()
	}))
	input := request("1")
	input.Mode, input.PrecheckQuestions = "both", []string{"candy"}
	queued, err := f.service.EnqueueAnimation(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("OAuth precheck did not start")
	}
	if _, err := f.service.EnqueueAnimation(context.Background(), request("1")); err == nil {
		t.Fatal("same account accepted overlapping detection")
	}
	if !f.runner.CancelTask(queued.ID) {
		t.Fatal("OAuth task could not be cancelled")
	}
	task := finished(t, f)
	rows := task.Result["animations"].([]modelcheck.AnimationResult)
	if task.Status != "cancelled" || len(rows) != 1 || rows[0].Mode != "precheck" {
		t.Fatalf("cancelled combined result: %#v", task)
	}
}
