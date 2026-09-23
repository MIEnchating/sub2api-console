package accountworkbench_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/golang-jwt/jwt/v5"
)

type observedRunner struct {
	*taskrunner.Group
	finished chan struct{}
}

func (r *observedRunner) GoTask(id string, run func(context.Context)) error {
	return r.Group.GoTask(id, func(ctx context.Context) { run(ctx); r.finished <- struct{}{} })
}
func runTasks(t *testing.T, service *accountworkbench.Service) (*observedRunner, *taskstore.Store) {
	t.Helper()
	tasks, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	runner := &observedRunner{Group: taskrunner.NewBounded(context.Background(), 2), finished: make(chan struct{}, 4)}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = runner.Shutdown(ctx)
		_ = tasks.Close()
	})
	service.UseExecution(tasks, runner, nil, t.TempDir())
	return runner, tasks
}
func awaitRun(t *testing.T, runner *observedRunner) {
	t.Helper()
	select {
	case <-runner.finished:
	case <-time.After(10 * time.Second):
		t.Fatal("isolated task did not complete")
	}
}
func signedRunInput(t *testing.T, service *accountworkbench.Service, exchange ...func(*http.Request) error) string {
	return signedRunInputWithProtocol(t, service, nil, exchange...)
}
func signedRunInputWithProtocol(t *testing.T, service *accountworkbench.Service, protocol func(*http.Request) (*http.Response, error), exchange ...func(*http.Request) error) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks := map[string]any{"keys": []any{map[string]any{"kid": "run-key", "kty": "RSA", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())}}}
	var tokenResponse map[string]any
	service.UseOfficialTransport(transportFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() == "https://auth.openai.com/oauth/token" && len(exchange) == 1 {
			if err := exchange[0](request); err != nil {
				return nil, err
			}
			return upstreamResponse(tokenResponse), nil
		}
		if request.URL.String() != "https://auth.openai.com/.well-known/jwks.json" {
			if protocol != nil {
				return protocol(request)
			}
			return nil, fmt.Errorf("unexpected official endpoint")
		}
		return upstreamResponse(jwks), nil
	}))
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"iss": "https://auth.openai.com", "aud": "https://api.openai.com/v1", "exp": time.Now().Add(time.Hour).Unix(), "https://api.openai.com/auth": map[string]any{"chatgpt_user_id": "run-user", "chatgpt_account_id": "run-workspace"}, "https://api.openai.com/profile": map[string]any{"email": "run@example.test"}})
	token.Header["kid"] = "run-key"
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	tokenResponse = map[string]any{"access_token": signed, "refresh_token": "rt_run-private", "token_type": "Bearer"}
	raw, _ := json.Marshal(map[string]any{"access_token": signed, "refresh_token": "rt_run-private"})
	return string(raw)
}
func upstreamResponse(value any) *http.Response {
	raw, _ := json.Marshal(value)
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(raw)))}
}

func TestRunVerifiesAppliedConfigurationBeforeEnabling(t *testing.T) {
	for _, tc := range []struct {
		name, corruptStage  string
		runtimeStatusActive bool
		runtimeAutoPromoted bool
		defaultGroupBound   bool
	}{
		{name: "applied configuration enables scheduling"},
		{name: "active runtime status still permits isolated import", runtimeStatusActive: true},
		{name: "creation ignoring scheduling flag is paused before promotion", runtimeAutoPromoted: true},
		{name: "default group is removed while paused before promotion", defaultGroupBound: true},
		{name: "unexpected created identity prevents all follow-up writes", corruptStage: "identity", runtimeAutoPromoted: true},
		{name: "ignored creation configuration prevents enabling", corruptStage: "create"},
		{name: "extra upstream model mapping prevents enabling", corruptStage: "model-mapping"},
		{name: "model mapping changed on promotion prevents scheduling", corruptStage: "promote-model-mapping"},
		{name: "ignored final configuration prevents scheduling", corruptStage: "promote"},
		{name: "lost creation response reconciles without a duplicate write", corruptStage: "create-response-lost"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, store := fixture(t, `{}`)
			owner, _ := previewOwner(t, store)
			runner, tasks := runTasks(t, service)
			input := signedRunInput(t, service)
			if strings.Contains(tc.corruptStage, "model-mapping") {
				var imported map[string]any
				_ = json.Unmarshal([]byte(input), &imported)
				imported["model_mapping"] = map[string]string{"selected": "selected"}
				raw, _ := json.Marshal(imported)
				input = string(raw)
			}
			var current map[string]any
			var writes []string
			var mu sync.Mutex
			allowReconcile := false
			service.UseTransport(transportFunc(func(request *http.Request) (*http.Response, error) {
				mu.Lock()
				defer mu.Unlock()
				if request.URL.Host != "isolated.invalid" {
					return nil, fmt.Errorf("unexpected host")
				}
				path := strings.TrimPrefix(request.URL.Path, "/api/v1")
				if request.Method == "GET" {
					if tc.corruptStage == "create-response-lost" && current != nil && !allowReconcile {
						return nil, fmt.Errorf("site temporarily unreachable")
					}
					if path == "/admin/accounts" {
						items := []any{}
						if current != nil {
							items = append(items, current)
						}
						return upstreamResponse(map[string]any{"data": map[string]any{"items": items, "total": len(items)}}), nil
					}
					return upstreamResponse(map[string]any{"data": current}), nil
				}
				var body map[string]any
				decoder := json.NewDecoder(request.Body)
				decoder.UseNumber()
				_ = decoder.Decode(&body)
				writes = append(writes, request.Method+" "+path)
				switch path {
				case "/admin/accounts":
					if body["schedulable"] != false || body["status"] != "inactive" || len(body["group_ids"].([]any)) != 0 || body["skip_default_group_bind"] != true {
						return nil, fmt.Errorf("creation was not isolated")
					}
					current = body
					current["id"] = json.Number("41")
					current["notes"] = nil
					if tc.runtimeStatusActive {
						current["status"] = "active"
					}
					if tc.runtimeAutoPromoted {
						current["status"] = "active"
						current["schedulable"] = true
					}
					if tc.defaultGroupBound {
						current["group_ids"] = []any{json.Number("99")}
					}
					if tc.corruptStage == "identity" {
						current["credentials"].(map[string]any)["chatgpt_user_id"] = "different-user"
					}
					if tc.corruptStage == "model-mapping" {
						current["credentials"].(map[string]any)["model_mapping"] = map[string]any{"selected": "selected", "*": "unselected"}
					}
					if tc.corruptStage == "create" {
						current["concurrency"] = json.Number("7")
					}
					if tc.corruptStage == "create-response-lost" {
						return nil, fmt.Errorf("connection lost after write")
					}
				case "/admin/accounts/41":
					for key, value := range body {
						current[key] = value
					}

					if tc.corruptStage == "promote-model-mapping" {
						current["credentials"].(map[string]any)["model_mapping"] = map[string]any{"selected": "selected", "*": "unselected"}
					}
					if tc.corruptStage == "promote" {
						current["concurrency"] = json.Number("7")
					}
				case "/admin/accounts/41/clear-error":
				case "/admin/accounts/41/schedulable":
					current["schedulable"] = body["schedulable"]
				default:
					return nil, fmt.Errorf("unexpected write")
				}
				return upstreamResponse(map[string]any{"data": current}), nil
			}))
			preview, err := service.Preview(context.Background(), owner, accountworkbench.PreviewInput{Action: "import", Content: input, Promote: true})
			if err != nil {
				t.Fatal(err)
			}
			queued, err := service.Start(context.Background(), owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = service.Start(context.Background(), owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision}); err == nil {
				t.Fatal("preview replay accepted")
			}
			awaitRun(t, runner)
			result, err := service.Run(context.Background(), owner, queued.ID)
			if err != nil {
				t.Fatal(err)
			}
			if tc.corruptStage == "create-response-lost" {
				if result.Status == "completed" {
					t.Fatal("lost response was reported successful")
				}
				mu.Lock()
				allowReconcile = true
				mu.Unlock()
				_, err = service.Retry(context.Background(), owner, accountworkbench.RunConfirmation{ID: result.ID, Revision: result.Revision})
				if err != nil {
					t.Fatal(err)
				}
				awaitRun(t, runner)
				result, err = service.Run(context.Background(), owner, result.ID)
				if err != nil || result.Status != "completed" {
					t.Fatalf("reconciliation failed: %v %+v", err, result)
				}
			} else if tc.corruptStage != "" {
				if tc.corruptStage == "identity" {
					if result.Status == "completed" || len(writes) != 1 {
						t.Fatalf("changed identity received follow-up writes: %v", writes)
					}
					return
				}
				if result.Status == "completed" || current["schedulable"] != false {
					t.Fatal("unapplied configuration was enabled")
				}
				for _, write := range writes {
					if strings.HasSuffix(write, "/schedulable") {
						t.Fatal("scheduling enabled before configuration verification")
					}
				}
				return
			}
			if tc.runtimeAutoPromoted {
				if len(writes) < 2 || writes[1] != "POST /admin/accounts/41/schedulable" {
					t.Fatalf("creation was not paused before applying final configuration: %v", writes)
				}
				if result.Status != "completed" || result.Items[0].AccountID != "41" || current["schedulable"] != true {
					t.Fatalf("creation did not finish after isolation: status=%s item=%+v", result.Status, result.Items[0])
				}
				task, err := tasks.Get(context.Background(), result.TaskID)
				if err != nil || task.Status != "succeeded" {
					t.Fatalf("creation task did not finish: %v %+v", err, task)
				}
				return
			}
			if result.Status != "completed" || result.Items[0].AccountID != "41" {
				t.Fatalf("import not completed: %+v", result.Items)
			}
			expected := []string{"POST /admin/accounts", "PUT /admin/accounts/41", "POST /admin/accounts/41/clear-error", "POST /admin/accounts/41/schedulable"}
			if tc.defaultGroupBound {
				expected = []string{"POST /admin/accounts", "PUT /admin/accounts/41", "PUT /admin/accounts/41", "POST /admin/accounts/41/clear-error", "POST /admin/accounts/41/schedulable"}
			}
			raw, _ := json.Marshal(writes)
			want, _ := json.Marshal(expected)
			if string(raw) != string(want) {
				t.Fatalf("writes=%s", raw)
			}
			task, err := tasks.Get(context.Background(), result.TaskID)
			if err != nil || task.Status != "succeeded" {
				t.Fatal("task did not finish")
			}
			public, _ := json.Marshal([]any{result, task})
			if strings.Contains(string(public), "rt_run-private") {
				t.Fatal("public task leaked credentials")
			}
		})
	}
}

func TestRunRejectsChangedTargetBeforeClaimingPreview(t *testing.T) {
	service, store := fixture(t, `{}`)
	owner, _ := previewOwner(t, store)
	runTasks(t, service)
	preview, err := service.Preview(context.Background(), owner, accountworkbench.PreviewInput{Action: "import", Content: "rt_test"})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.ConfigureTarget(context.Background(), "https://other.invalid", "changed", 10); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Start(context.Background(), owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision}); err == nil {
		t.Fatal("changed target accepted")
	}
	records, err := service.Runs(context.Background(), owner)
	if err != nil || len(records) != 0 {
		t.Fatal("target mismatch started a run")
	}
}
