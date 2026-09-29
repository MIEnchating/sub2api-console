package modelcheck_test

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

type deadlineCredentials struct {
	credentials
	mu       sync.Mutex
	deadline time.Time
	cancel   context.CancelFunc
}

func (c *deadlineCredentials) UpstreamKeySecret(ctx context.Context, host, key, group string) (*configstore.UpstreamKeySecret, error) {
	c.mu.Lock()
	c.deadline, _ = ctx.Deadline()
	c.mu.Unlock()
	c.cancel()
	return c.credentials.UpstreamKeySecret(ctx, host, key, group)
}

func TestDetectionTaskDeadlineCoversSelectedAccountsStagesAndRounds(t *testing.T) {
	for _, tc := range []struct {
		name             string
		accounts         int
		precheck         bool
		terminal         bool
		animation        bool
		legacyAnimation  bool
		rounds           int
		minimumRemaining time.Duration
	}{
		{name: "200 prechecks budget 50 parallel batches", accounts: 200, precheck: true, minimumRemaining: 50 * 120 * time.Second},
		{name: "small batch retains its minimum", accounts: 1, precheck: true, minimumRemaining: 29 * time.Minute},
		{name: "terminal budget includes every round", accounts: 2, terminal: true, rounds: 20, minimumRemaining: 20 * 120 * time.Second},
		{name: "combined stages include animation retries", accounts: 10, precheck: true, terminal: true, animation: true, rounds: 2, minimumRemaining: 3 * 6 * 120 * time.Second},
		{name: "legacy configuration budgets animation", accounts: 10, legacyAnimation: true, minimumRemaining: 29 * time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t, tc.accounts, "openai", func(http.ResponseWriter, *http.Request) {
				t.Error("cancelled batch must not send upstream requests")
			})
			ids := make([]string, tc.accounts)
			for i := range ids {
				ids[i] = strconv.Itoa(i + 1)
			}
			f.catalog.groupMembers = map[string][]string{"7": ids, "8": ids}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			credential := &deadlineCredentials{cancel: cancel}
			service, err := modelcheck.New(f.tasks, credential, f.catalog, credential)
			if err != nil {
				t.Fatal(err)
			}
			runner := &deferredRunner{}
			service.UseTaskRunner(runner)
			value := managedTask()
			value.Precheck, value.Terminal, value.TerminalRounds = tc.precheck, tc.terminal, tc.rounds
			value.TimeoutSeconds = 120
			if !tc.legacyAnimation {
				value.Animation = &tc.animation
			}
			plans, err := service.SaveDetectionTask(context.Background(), value, "test")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.RunDetectionTask(context.Background(), plans[0].ID, plans[0].Version); err != nil {
				t.Fatal(err)
			}
			runner.runs[0](ctx)
			credential.mu.Lock()
			remaining := time.Until(credential.deadline)
			credential.mu.Unlock()
			if remaining < tc.minimumRemaining {
				t.Fatalf("batch remaining time = %s, want at least %s", remaining, tc.minimumRemaining)
			}
			if task := finished(t, f); task.Status != "cancelled" {
				t.Fatalf("batch cancellation = %s, want cancelled", task.Status)
			}
		})
	}
}
