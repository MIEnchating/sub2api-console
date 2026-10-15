package modelcheck_test

import (
	"context"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelrequest"
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
	mu           sync.Mutex
	deadline     time.Time
	taskDeadline time.Time
	cancel       context.CancelFunc
}

func (c *deadlineCredentials) UpstreamKeySecret(ctx context.Context, host, key, group string) (*configstore.UpstreamKeySecret, error) {
	c.mu.Lock()
	c.deadline, _ = modelrequest.ExecutionDeadline(ctx)
	c.taskDeadline, _ = ctx.Deadline()
	c.mu.Unlock()
	c.cancel()
	return c.credentials.UpstreamKeySecret(ctx, host, key, group)
}

func TestDetectionExecutionDeadlineUsesItemBudgetRegardlessOfBatchSize(t *testing.T) {
	for _, tc := range []struct {
		name            string
		accounts        int
		precheck        bool
		terminal        bool
		animation       bool
		legacyAnimation bool
		rounds          int
	}{
		{name: "large precheck batch", accounts: 200, precheck: true},
		{name: "single precheck", accounts: 1, precheck: true},
		{name: "combined stages", accounts: 10, precheck: true, animation: true},
		{name: "legacy animation configuration", accounts: 10, legacyAnimation: true},
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
			if remaining < 119*time.Second || remaining > 120*time.Second {
				t.Fatalf("execution remaining time = %s, want configured 120s budget", remaining)
			}
			if task := finished(t, f); task.Status != "cancelled" {
				t.Fatalf("batch cancellation = %s, want cancelled", task.Status)
			}
		})
	}
}
