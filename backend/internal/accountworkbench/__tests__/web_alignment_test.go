package accountworkbench_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestMaintenanceViewPublishesActualSchedulerDeadlineAndClearsItWhenDisabled(t *testing.T) {
	f := newImportFixture(t, nil)
	config, err := f.service.Maintenance(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	input := config.WorkbenchMaintenance
	input.Enabled = true
	config, err = f.service.SaveMaintenance(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	ticks := make(chan time.Time)
	f.service.UseMaintenanceTicks(ticks)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); f.service.RunScheduler(ctx) }()
	defer func() { cancel(); <-done }()
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	ticks <- now
	ticks <- now.Add(time.Second)
	view, err := f.service.Maintenance(ctx)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(view)
	var fields map[string]any
	_ = json.Unmarshal(raw, &fields)
	expected := now.Add(time.Duration(config.IntervalMinutes) * time.Minute).Format(time.RFC3339)
	if fields["next_run_at"] != expected {
		t.Fatalf("next check: got %v, want %s", fields["next_run_at"], expected)
	}
	input = config.WorkbenchMaintenance
	input.Enabled = false
	view, err = f.service.SaveMaintenance(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(view)
	fields = map[string]any{}
	_ = json.Unmarshal(raw, &fields)
	if value, ok := fields["next_run_at"]; ok && value != "" {
		t.Fatal("disabled maintenance retained scheduled deadline")
	}
}

type proxyCheckerBoundary struct {
	proxy  string
	direct bool
	fail   bool
}

func (c *proxyCheckerBoundary) CheckOAuth(context.Context, string, string, map[string]any, string, int) (map[string]any, error) {
	c.direct = true
	return map[string]any{"verdict": "SOL_CONSISTENT"}, nil
}
func (c *proxyCheckerBoundary) CheckOAuthWithProxy(_ context.Context, _, _ string, _ map[string]any, _ string, _ int, proxy string) (map[string]any, error) {
	c.proxy = proxy
	if c.fail {
		return nil, errors.New("isolated check failure")
	}
	return map[string]any{"verdict": "SOL_CONSISTENT"}, nil
}

func TestRetryRetainsImportProxyWithoutPublishingCredentials(t *testing.T) {
	f := newImportFixture(t, nil)
	checker := &proxyCheckerBoundary{fail: true}
	f.service = accountworkbench.New(f.private, f.tasks, f.business, checker, f.runner)
	const proxy = "http://operator:private-retry-password@proxy.example.com:8080"
	preview, err := f.service.Preview(context.Background(), "owner", accountworkbench.PreviewInput{Content: importedJSON, CheckAfterImport: true, ProxyURL: proxy})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Import(context.Background(), "owner", preview.ID, true); err != nil {
		t.Fatal(err)
	}
	source := f.await(t)
	checker.fail, checker.proxy = false, ""
	retry, err := f.service.RetryPreview(context.Background(), "owner", accountworkbench.RetryPreviewInput{TaskID: source.ID, Indexes: []int{0}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Import(context.Background(), "owner", retry.ID, true); err != nil {
		t.Fatal(err)
	}
	result := f.await(t)
	if checker.direct || checker.proxy != proxy {
		t.Fatal("retry lost original proxy")
	}
	raw, _ := json.Marshal(result)
	if strings.Contains(string(raw), "private-retry-password") {
		t.Fatal("retry exposed proxy credentials")
	}
}

func TestMixedImportUsesSubmittedProxyForBehaviorCheckWithoutPublishingProxy(t *testing.T) {
	f := newImportFixture(t, nil)
	checker := &proxyCheckerBoundary{}
	f.service = accountworkbench.New(f.private, f.tasks, f.business, checker, f.runner)
	const proxy = "http://operator:private-proxy-password@proxy.example.com:8080"
	preview, err := f.service.PreviewWorkbenchRun(context.Background(), "owner", accountworkbench.WorkbenchRunInput{Content: importedJSON, CheckAfterImport: true, Model: "gpt-5.6-sol", ProxyURL: proxy})
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.service.StartWorkbenchRun(context.Background(), "owner", preview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	f.await(t)
	result, err := f.service.PreviewWorkbenchRunResult(context.Background(), "owner", run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Import(context.Background(), "owner", result.ID, true); err != nil {
		t.Fatal(err)
	}
	task := f.await(t)
	if checker.direct || checker.proxy != proxy {
		t.Fatal("submitted batch proxy was not used for behavior check")
	}
	raw, _ := json.Marshal(task)
	if strings.Contains(string(raw), "private-proxy-password") {
		t.Fatal("proxy credentials appeared in public result")
	}
}
