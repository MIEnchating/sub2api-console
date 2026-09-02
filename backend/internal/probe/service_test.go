package probe

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestResolveModelPrefersAccountOverrideBeforeGroupPolicy(t *testing.T) {
	groupID := "7"
	model, reason := resolveModel(map[string]any{
		"account_test_models":   map[string]any{"41": "account-model"},
		"group_policy_bindings": map[string]any{"7": map[string]any{"probe_model": "group-model"}},
	}, "41", &groupID, nil)
	if reason != nil || model == nil || *model != "account-model" {
		t.Fatalf("model=%v reason=%v", model, reason)
	}
}

func TestResolveModelFallsBackToFirstKnownAccountModel(t *testing.T) {
	groupID := "7"
	model, reason := resolveModel(map[string]any{}, "41", &groupID, []string{"", "cached-model", "second"})
	if reason != nil || model == nil || *model != "cached-model" {
		t.Fatalf("model=%v reason=%v", model, reason)
	}
}

func TestResolveModelUsesCurrentOverrideOrderWithoutLegacyPolicyProfiles(t *testing.T) {
	groupID := "7"
	policy := map[string]any{
		"probe": map[string]any{"model": "global-model"},
		"group_policy_bindings": map[string]any{
			"7": map[string]any{"probe_model": "group-model", "policy_id": "legacy-profile"},
		},
		"default_policy_id": "legacy-default",
		"policies": map[string]any{
			"legacy-profile": map[string]any{"probe": map[string]any{"model": "legacy-group-model"}},
			"legacy-default": map[string]any{"probe": map[string]any{"model": "legacy-default-model"}},
		},
	}
	model, reason := resolveModel(policy, "41", &groupID, []string{"known-model"})
	if reason != nil || model == nil || *model != "group-model" {
		t.Fatalf("model=%v reason=%v", model, reason)
	}

	policy["group_policy_bindings"] = map[string]any{"7": map[string]any{"policy_id": "legacy-profile"}}
	model, reason = resolveModel(policy, "41", &groupID, []string{"known-model"})
	if reason != nil || model == nil || *model != "global-model" {
		t.Fatalf("legacy profile changed current resolution: model=%v reason=%v", model, reason)
	}
}

func TestResolveModelDoesNotRequireGroupIDForGlobalOrKnownModel(t *testing.T) {
	model, reason := resolveModel(
		map[string]any{"probe": map[string]any{"model": "global-model"}},
		"41", nil, []string{"known-model"},
	)
	if reason != nil || model == nil || *model != "global-model" {
		t.Fatalf("global model=%v reason=%v", model, reason)
	}

	model, reason = resolveModel(map[string]any{}, "41", nil, []string{"known-model"})
	if reason != nil || model == nil || *model != "known-model" {
		t.Fatalf("known model=%v reason=%v", model, reason)
	}
}

type fakeRepository struct {
	policy      map[string]any
	policyErr   error
	policyCalls int
	candidates  []business.ProbeCandidate
	samples     []business.ProbeSample
	mu          sync.Mutex
}

func (repository *fakeRepository) ControlPolicy(context.Context) (map[string]any, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.policyCalls++
	return repository.policy, repository.policyErr
}

func (repository *fakeRepository) ProbeCandidates(context.Context, *string, *string) ([]business.ProbeCandidate, error) {
	return repository.candidates, nil
}

func (repository *fakeRepository) PersistProbeSamples(_ context.Context, samples []business.ProbeSample) (int, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.samples = append([]business.ProbeSample{}, samples...)
	return len(samples), nil
}

type fakeSettings struct {
	target configstore.TargetSettings
}

func (settings fakeSettings) TargetSettings(context.Context) (configstore.TargetSettings, error) {
	return settings.target, nil
}

type mutableSettings struct {
	mu     sync.Mutex
	target configstore.TargetSettings
}

func (settings *mutableSettings) TargetSettings(context.Context) (configstore.TargetSettings, error) {
	settings.mu.Lock()
	defer settings.mu.Unlock()
	return settings.target, nil
}

func (settings *mutableSettings) Set(target configstore.TargetSettings) {
	settings.mu.Lock()
	defer settings.mu.Unlock()
	settings.target = target
}

type deferredProbeRunner struct {
	run func(context.Context)
}

func (runner *deferredProbeRunner) Go(run func(context.Context)) error {
	runner.run = run
	return nil
}

func (runner *deferredProbeRunner) Run(ctx context.Context) {
	if runner.run == nil {
		panic("probe task was not scheduled")
	}
	runner.run(ctx)
}

type observingTasks struct {
	terminal chan taskstore.Task
}

func (tasks *observingTasks) Save(_ context.Context, task taskstore.Task) error {
	if task.Status == "succeeded" || task.Status == "failed" {
		select {
		case tasks.terminal <- task:
		default:
		}
	}
	return nil
}

func TestQueuedProbeRejectsManagementTargetChangeBeforeRemoteAccess(t *testing.T) {
	var targetARequests, targetBRequests atomic.Int32
	targetA := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		targetARequests.Add(1)
		http.Error(response, "unexpected obsolete target access", http.StatusInternalServerError)
	}))
	defer targetA.Close()
	targetB := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		targetBRequests.Add(1)
		http.Error(response, "unexpected replacement target access", http.StatusInternalServerError)
	}))
	defer targetB.Close()

	repository := &fakeRepository{
		policy: map[string]any{"probe": map[string]any{}},
		candidates: []business.ProbeCandidate{{
			AccountID: "41", GroupName: "codex", KnownModels: []string{"gpt-test"}, Metadata: map[string]any{},
		}},
	}
	settings := &mutableSettings{target: configstore.TargetSettings{BaseURL: targetA.URL, AdminKey: "target-a", TimeoutSeconds: 5}}
	tasks := &observingTasks{terminal: make(chan taskstore.Task, 1)}
	runner := &deferredProbeRunner{}
	service := New(repository, settings, tasks)
	service.UseTaskRunner(runner)

	if _, err := service.Enqueue(context.Background(), Request{}, "operator"); err != nil {
		t.Fatal(err)
	}
	settings.Set(configstore.TargetSettings{BaseURL: targetB.URL, AdminKey: "target-b", TimeoutSeconds: 5})
	runner.Run(context.Background())

	terminal := <-tasks.terminal
	if terminal.Status != "failed" || !strings.Contains(terminal.Message, "管理目标") || terminal.Result["remote_write"] != false {
		t.Fatalf("target-drift probe task=%#v", terminal)
	}
	if targetARequests.Load() != 0 || targetBRequests.Load() != 0 {
		t.Fatalf("queued probe accessed remote targets: target-a=%d target-b=%d", targetARequests.Load(), targetBRequests.Load())
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if len(repository.samples) != 0 {
		t.Fatalf("target-drift probe persisted samples: %#v", repository.samples)
	}
}

func TestActiveProbeUsesOfficialStreamAndPersistsConfirmedSample(t *testing.T) {
	requestCount := 0
	streamRelease := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requestCount++
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/admin/accounts/41/test" || request.Header.Get("X-API-Key") != "secret" {
			t.Fatalf("unexpected request: %s %s headers=%#v", request.Method, request.URL.Path, request.Header)
		}
		response.Header().Set("Content-Type", "text/event-stream")
		_, _ = response.Write([]byte("data: {\"type\":\"test_start\",\"model\":\"mapped-model\"}\n\n"))
		_, _ = response.Write([]byte("data: {\"type\":\"status\",\"text\":\"正在请求上游\"}\n\n"))
		_, _ = response.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"pong\"}}]}\n\n"))
		if flusher, ok := response.(http.Flusher); ok {
			flusher.Flush()
		}
		<-streamRelease
		_, _ = response.Write([]byte("data: {\"type\":\"test_complete\",\"success\":true}\n\n"))
	}))
	defer server.Close()
	defer close(streamRelease)
	repository := &fakeRepository{
		policy: map[string]any{
			"probe":                 map[string]any{"enabled": true, "model": "gpt-test", "timeout_seconds": int64(5), "concurrency": int64(2)},
			"scope":                 map[string]any{},
			"group_policy_bindings": map[string]any{"7": map[string]any{"enabled": true}},
		},
		candidates: []business.ProbeCandidate{
			{AccountID: "41", GroupName: "codex", GroupID: textPointer("7"), KnownModels: []string{"gpt-test"}, Metadata: map[string]any{}},
			{AccountID: "42", GroupName: "codex", GroupID: textPointer("7"), MetadataErr: errors.New("invalid metadata")},
		},
	}
	tasks := &observingTasks{terminal: make(chan taskstore.Task, 1)}
	service := New(repository, fakeSettings{
		target: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 5},
	}, tasks)

	queued, err := service.Enqueue(context.Background(), Request{}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if queued.Status != "queued" || queued.Operation != "active-probe" {
		t.Fatalf("unexpected queued task: %#v", queued)
	}
	select {
	case terminal := <-tasks.terminal:
		if terminal.Status != "succeeded" || terminal.Result["passed"] != 1 || terminal.Result["failed"] != 0 || terminal.Result["skipped"] != 1 || terminal.Result["persisted"] != 1 {
			t.Fatalf("unexpected terminal task: %#v", terminal)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("active probe task did not finish")
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if requestCount != 1 || len(repository.samples) != 1 || repository.samples[0].Result != "通过" || repository.samples[0].SampleCount != 1 ||
		repository.samples[0].RequestModel != "gpt-test" || repository.samples[0].ActualModel != "mapped-model" ||
		repository.samples[0].LatencyP95 == nil {
		t.Fatalf("requests=%d samples=%#v", requestCount, repository.samples)
	}
}

func TestFixedRetryRecoversAfterConfiguredStatus(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requestCount++
		if request.URL.Path != "/api/v1/admin/accounts/41/test" {
			t.Fatalf("unexpected request path %s", request.URL.Path)
		}
		if requestCount < 3 {
			response.WriteHeader(http.StatusServiceUnavailable)
			_, _ = response.Write([]byte(`{"error":"temporary"}`))
			return
		}
		response.Header().Set("Content-Type", "text/event-stream")
		_, _ = response.Write([]byte("data: {\"type\":\"content\",\"text\":\"pong\"}\n\n"))
	}))
	defer server.Close()
	repository := &fakeRepository{
		policy: map[string]any{"probe": map[string]any{
			"retry_enabled": true, "retry_source": "fixed", "retry_count": int64(2),
			"retry_status_codes": []any{int64(503)},
		}},
		candidates: []business.ProbeCandidate{{AccountID: "41", GroupName: "codex", KnownModels: []string{"gpt-test"}, Metadata: map[string]any{}}},
	}
	service := New(repository, fakeSettings{target: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret"}}, &observingTasks{})

	summary, err := service.RunNow(context.Background(), Request{})
	if err != nil {
		t.Fatal(err)
	}
	if requestCount != 3 || summary.Passed != 1 || len(summary.Results) != 1 || summary.Results[0].Attempts != 3 {
		t.Fatalf("requests=%d summary=%#v", requestCount, summary)
	}
}

func TestActiveProbeRejectsRedirectBodyThatLooksLikeAValidStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "text/event-stream")
		response.WriteHeader(http.StatusFound)
		_, _ = response.Write([]byte("data: {\"type\":\"content\",\"text\":\"pong\"}\n\n"))
	}))
	defer server.Close()
	repository := &fakeRepository{
		policy: map[string]any{"probe": map[string]any{}},
		candidates: []business.ProbeCandidate{{
			AccountID: "41", GroupName: "codex", KnownModels: []string{"gpt-test"}, Metadata: map[string]any{},
		}},
	}
	service := New(repository, fakeSettings{target: configstore.TargetSettings{
		BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 5,
	}}, &observingTasks{})

	summary, err := service.RunNow(context.Background(), Request{})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Passed != 0 || summary.Failed != 1 || len(summary.Results) != 1 ||
		summary.Results[0].StatusCode == nil || *summary.Results[0].StatusCode != http.StatusFound {
		t.Fatalf("redirect response was accepted as a successful probe: %#v", summary)
	}
}

func TestFixedRetryDoesNotRunForUnconfiguredStatusOrDisabledSwitch(t *testing.T) {
	for name, retryEnabled := range map[string]bool{"unconfigured status": true, "disabled": false} {
		t.Run(name, func(t *testing.T) {
			requestCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				requestCount++
				response.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()
			repository := &fakeRepository{
				policy: map[string]any{"probe": map[string]any{
					"retry_enabled": retryEnabled, "retry_source": "fixed", "retry_count": int64(3),
					"retry_status_codes": []any{int64(503)},
				}},
				candidates: []business.ProbeCandidate{{AccountID: "41", GroupName: "codex", KnownModels: []string{"gpt-test"}, Metadata: map[string]any{}}},
			}
			service := New(repository, fakeSettings{target: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret"}}, &observingTasks{})

			summary, err := service.RunNow(context.Background(), Request{})
			if err != nil {
				t.Fatal(err)
			}
			if requestCount != 1 || summary.Failed != 1 || summary.Results[0].Attempts != 1 {
				t.Fatalf("requests=%d summary=%#v", requestCount, summary)
			}
		})
	}
}

func TestSub2APIPoolRetryLoadsDirectoryOnceAndAppliesPerAccountRules(t *testing.T) {
	var mutex sync.Mutex
	directoryRequests := 0
	probeRequests := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		mutex.Lock()
		defer mutex.Unlock()
		switch request.URL.Path {
		case "/api/v1/admin/accounts":
			directoryRequests++
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"success":true,"data":{"items":[{"id":41,"credentials":{"pool_mode":true,"pool_mode_retry_count":1,"pool_mode_retry_status_codes":[502]}},{"id":42,"credentials":{"pool_mode":false}}],"total":2}}`))
		case "/api/v1/admin/accounts/41/test", "/api/v1/admin/accounts/42/test":
			accountID := strings.Split(request.URL.Path, "/")[5]
			probeRequests[accountID]++
			if accountID == "41" && probeRequests[accountID] > 1 {
				response.Header().Set("Content-Type", "text/event-stream")
				_, _ = response.Write([]byte("data: {\"type\":\"content\",\"text\":\"pong\"}\n\n"))
				return
			}
			response.WriteHeader(http.StatusBadGateway)
		default:
			t.Fatalf("unexpected request path %s", request.URL.Path)
		}
	}))
	defer server.Close()
	repository := &fakeRepository{
		policy: map[string]any{"probe": map[string]any{
			"retry_enabled": true, "retry_source": "sub2api_pool", "retry_count": int64(10),
			"retry_status_codes": []any{int64(500)},
		}},
		candidates: []business.ProbeCandidate{
			{AccountID: "41", GroupName: "codex", KnownModels: []string{"gpt-test"}, Metadata: map[string]any{}},
			{AccountID: "42", GroupName: "codex", KnownModels: []string{"gpt-test"}, Metadata: map[string]any{}},
		},
	}
	service := New(repository, fakeSettings{target: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret"}}, &observingTasks{})

	summary, err := service.RunNow(context.Background(), Request{})
	if err != nil {
		t.Fatal(err)
	}
	mutex.Lock()
	defer mutex.Unlock()
	if directoryRequests != 1 || probeRequests["41"] != 2 || probeRequests["42"] != 1 {
		t.Fatalf("directory=%d probes=%#v", directoryRequests, probeRequests)
	}
	if summary.Passed != 1 || summary.Failed != 1 || summary.Results[0].Attempts != 2 || summary.Results[1].Attempts != 1 {
		t.Fatalf("summary=%#v", summary)
	}
}

func TestPoolRetryConfigUsesSub2APIDefaultsAndHonorsExplicitEmptyCodes(t *testing.T) {
	defaults := poolRetryConfig(map[string]any{"credentials": map[string]any{"pool_mode": true}})
	if defaults.Count != 3 {
		t.Fatalf("default retry count=%d", defaults.Count)
	}
	for _, code := range []int{401, 403, 429} {
		if _, found := defaults.StatusCodes[code]; !found {
			t.Fatalf("default status codes=%#v", defaults.StatusCodes)
		}
	}
	empty := poolRetryConfig(map[string]any{"credentials": map[string]any{
		"pool_mode": true, "pool_mode_retry_count": "99", "pool_mode_retry_status_codes": []any{},
	}})
	if empty.Count != 10 || len(empty.StatusCodes) != 0 {
		t.Fatalf("explicit pool config=%#v", empty)
	}
}

func TestAutomaticProbeRejectsDisabledConfigurationBeforeCreatingTask(t *testing.T) {
	repository := &fakeRepository{policy: map[string]any{"probe": map[string]any{"enabled": false}}, candidates: []business.ProbeCandidate{}}
	tasks := &observingTasks{terminal: make(chan taskstore.Task, 1)}
	service := New(repository, fakeSettings{}, tasks)

	_, err := service.Enqueue(context.Background(), Request{Automatic: true}, "operator")
	if err == nil || err.Error() != "主动探测已关闭，请在调度策略中开启" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestManualProbeIgnoresAutomaticSchedulingFiltersButUsesRetryPolicy(t *testing.T) {
	groupID := "7"
	repository := &fakeRepository{
		policy: map[string]any{
			"probe": map[string]any{
				"enabled": false, "timeout_seconds": int64(1), "concurrency": int64(32), "prompt": "automatic",
				"retry_enabled": true, "retry_source": "fixed", "retry_count": int64(2), "retry_status_codes": []any{int64(503)},
			},
			"scope": map[string]any{"excluded_account_ids": []any{"41"}},
			"group_policy_bindings": map[string]any{
				"7": map[string]any{"enabled": false, "probe_enabled": false, "probe_model": "group-model"},
			},
		},
		candidates: []business.ProbeCandidate{{
			AccountID: "41", GroupName: "codex", GroupID: &groupID,
			KnownModels: []string{"manual-model"},
			Metadata:    map[string]any{"automatic_operation_excluded": true},
		}},
	}
	service := New(repository, fakeSettings{target: configstore.TargetSettings{
		BaseURL: "http://127.0.0.1:1", AdminKey: "test", TimeoutSeconds: 1,
	}}, &observingTasks{terminal: make(chan taskstore.Task, 1)})

	accountID := "41"
	prepared, err := service.prepare(context.Background(), Request{AccountID: &accountID})
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.targets) != 1 || prepared.targets[0].Model == nil || *prepared.targets[0].Model != "manual-model" {
		t.Fatalf("manual probe was filtered or used an automatic model: %#v", prepared.targets)
	}
	if prepared.config.Timeout != 60*time.Second || prepared.config.MaxConcurrency != 4 || prepared.config.Prompt != "hi" {
		t.Fatalf("manual probe inherited automatic execution settings: %#v", prepared.config)
	}
	if !prepared.config.RetryEnabled || prepared.config.RetrySource != "fixed" || prepared.config.Retry.Count != 2 {
		t.Fatalf("manual probe did not inherit retry settings: %#v", prepared.config)
	}
	if _, found := prepared.config.Retry.StatusCodes[503]; !found {
		t.Fatalf("manual probe retry status codes=%#v", prepared.config.Retry.StatusCodes)
	}
	if repository.policyCalls != 1 {
		t.Fatalf("manual probe should read retry policy once, got %d", repository.policyCalls)
	}
}

func TestBuildTargetsAppliesScopeAndProbesMultiGroupAccountOnce(t *testing.T) {
	policy := map[string]any{
		"probe": map[string]any{"model": "default-model"},
		"scope": map[string]any{"excluded_account_ids": []any{"42"}},
		"group_policy_bindings": map[string]any{
			"7": map[string]any{"probe_model": "model-a"},
			"9": map[string]any{"probe_model": "model-b"},
		},
	}
	targets, err := buildTargets([]business.ProbeCandidate{
		{AccountID: "41", GroupName: "codex", GroupID: textPointer("7"), Metadata: map[string]any{}},
		{AccountID: "41", GroupName: "pro", GroupID: textPointer("9"), Metadata: map[string]any{}},
		{AccountID: "42", GroupName: "codex", GroupID: textPointer("7"), Metadata: map[string]any{}},
	}, policy, targetOptions{applySchedulingPolicy: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 {
		t.Fatalf("excluded account remained in targets: %#v", targets)
	}
	if targets[0].AccountID != "41" || targets[0].GroupID == nil || *targets[0].GroupID != "7" || targets[0].Model == nil || *targets[0].Model != "model-a" || targets[0].SkipReason != nil {
		t.Fatalf("多分组账号没有由稳定主分组决定单次探测：%#v", targets[0])
	}
}

func TestBuildTargetsDropsDisabledMembershipBeforeChoosingPrimaryGroup(t *testing.T) {
	policy := map[string]any{
		"probe": map[string]any{"model": "default-model"},
		"group_policy_bindings": map[string]any{
			"7": map[string]any{"enabled": false, "probe_model": "disabled-model"},
			"9": map[string]any{"enabled": true, "probe_model": "managed-model"},
		},
	}
	targets, err := buildTargets([]business.ProbeCandidate{
		{AccountID: "41", GroupName: "disabled", GroupID: textPointer("7"), Metadata: map[string]any{}},
		{AccountID: "41", GroupName: "managed", GroupID: textPointer("9"), Metadata: map[string]any{}},
	}, policy, targetOptions{applySchedulingPolicy: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].GroupID == nil || *targets[0].GroupID != "9" ||
		targets[0].Model == nil || *targets[0].Model != "managed-model" {
		t.Fatalf("关闭分组重新成为探测主分组：%#v", targets)
	}
}

func TestRecoverySelectionRunsWhenOrdinaryProbeIsDisabled(t *testing.T) {
	groupID := "7"
	repository := &fakeRepository{
		policy: map[string]any{
			"probe": map[string]any{
				"enabled": false, "model": "recovery-model",
				"timeout_seconds": int64(60), "concurrency": int64(4),
			},
			"scope": map[string]any{},
			"group_policy_bindings": map[string]any{
				"7": map[string]any{"enabled": true, "probe_enabled": false},
			},
		},
		candidates: []business.ProbeCandidate{{
			AccountID: "41", GroupName: "codex", GroupID: &groupID, Metadata: map[string]any{},
		}},
	}
	service := New(repository, fakeSettings{target: configstore.TargetSettings{
		BaseURL: "http://127.0.0.1:1", AdminKey: "test", TimeoutSeconds: 1,
	}}, &observingTasks{terminal: make(chan taskstore.Task, 1)})

	prepared, err := service.prepare(context.Background(), Request{AccountIDs: []string{"41"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.targets) != 1 || prepared.targets[0].Model == nil || *prepared.targets[0].Model != "recovery-model" {
		t.Fatalf("recovery probe did not bypass only the ordinary probe switch: %#v", prepared.targets)
	}
}

func TestConfigFromPolicyUsesCurrentProbeTimeoutContract(t *testing.T) {
	configured, err := configFromPolicy(map[string]any{
		"probe": map[string]any{"timeout_seconds": int64(240), "concurrency": int64(32)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if configured.Timeout != 240*time.Second || configured.MaxConcurrency != 32 || configured.Prompt != "hi" {
		t.Fatalf("当前探测参数没有进入执行配置：%#v", configured)
	}
}

func TestConfigFromPolicyNormalizesProbePromptLikeGuardian(t *testing.T) {
	for name, input := range map[string]string{"blank": "   ", "configured": "  hello  "} {
		t.Run(name, func(t *testing.T) {
			configured, err := configFromPolicy(map[string]any{"probe": map[string]any{"prompt": input}})
			if err != nil {
				t.Fatal(err)
			}
			want := "hi"
			if name == "configured" {
				want = "hello"
			}
			if configured.Prompt != want {
				t.Fatalf("Prompt=%q, want %q", configured.Prompt, want)
			}
		})
	}
}

func TestConfigFromPolicyUsesOneCurrentContract(t *testing.T) {
	configured, err := configFromPolicy(map[string]any{
		"probe": map[string]any{"request_timeout_seconds": int64(15), "ordinary": map[string]any{"concurrency": int64(9)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if configured.Timeout != 60*time.Second || configured.MaxConcurrency != 4 {
		t.Fatalf("运行时仍读取未定义字段：%#v", configured)
	}
}

func TestConfigFromPolicyRejectsConcurrencyAboveExecutionLimit(t *testing.T) {
	_, err := configFromPolicy(map[string]any{
		"probe": map[string]any{"concurrency": int64(33)},
	})
	if err == nil || !strings.Contains(err.Error(), "probe.concurrency") {
		t.Fatalf("超过执行上限的并发应被拒绝，实际 %v", err)
	}
}

func TestEventParserDoesNotTreatMetadataOnlyEventAsFirstResponse(t *testing.T) {
	if eventHasText(map[string]any{"type": "response.created", "model": "gpt-test"}) {
		t.Fatal("metadata-only event was treated as first response")
	}
	if !eventHasText(map[string]any{"output": []any{map[string]any{"content": []any{map[string]any{"text": "ok"}}}}}) {
		t.Fatal("nested output text was not detected")
	}
}

func TestEventParserUsesContentEventsAndReadsActualModel(t *testing.T) {
	start := map[string]any{"type": "test_start", "model": "mapped-model"}
	status := map[string]any{"type": "status", "text": "正在请求上游"}
	content := map[string]any{"type": "content", "text": "pong"}
	if eventModel(start) != "mapped-model" {
		t.Fatalf("actual model=%q", eventModel(start))
	}
	if eventHasContent(start) || eventHasContent(status) {
		t.Fatal("test_start/status must not be treated as first content")
	}
	if !eventHasContent(content) {
		t.Fatal("content event was not detected")
	}
}
