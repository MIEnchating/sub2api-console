package routingwrite

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type batchCoordinatorAdmin struct {
	mu                 sync.Mutex
	states             map[string]map[string]any
	batchReply         map[string]any
	mutates            int
	lists              int
	active             int
	maxActive          int
	started            chan struct{}
	release            chan struct{}
	onList             func(map[string]map[string]any)
	skipApply          map[string]bool
	ignoreCancellation bool
}

func (a *batchCoordinatorAdmin) Account(_ context.Context, accountID string) (map[string]any, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	state, present := a.states[accountID]
	if !present {
		return nil, errors.New("account missing")
	}
	return copyMap(state), nil
}

func (a *batchCoordinatorAdmin) Accounts(context.Context) ([]map[string]any, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lists++
	if a.onList != nil {
		a.onList(a.states)
	}
	result := make([]map[string]any, 0, len(a.states))
	for _, state := range a.states {
		result = append(result, copyMap(state))
	}
	return result, nil
}

func (a *batchCoordinatorAdmin) DeleteAccount(context.Context, string) (map[string]any, error) {
	return nil, errors.New("unexpected delete")
}

func (a *batchCoordinatorAdmin) Mutate(ctx context.Context, _ string, path string, body map[string]any) (map[string]any, error) {
	a.mu.Lock()
	a.mutates++
	a.active++
	if a.active > a.maxActive {
		a.maxActive = a.active
	}
	a.mu.Unlock()
	if a.started != nil {
		a.started <- struct{}{}
	}
	if a.release != nil {
		if a.ignoreCancellation {
			<-a.release
		} else {
			select {
			case <-a.release:
			case <-ctx.Done():
				a.mu.Lock()
				a.active--
				a.mu.Unlock()
				return nil, ctx.Err()
			}
		}
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	defer func() { a.active-- }()
	if path == "/admin/accounts/bulk-update" {
		results := []any{}
		for _, rawID := range body["account_ids"].([]int64) {
			id := strconv.FormatInt(rawID, 10)
			a.apply(id, body)
			results = append(results, map[string]any{"account_id": rawID, "success": true})
		}
		if a.batchReply != nil {
			return copyMap(a.batchReply), nil
		}
		return map[string]any{"data": map[string]any{"results": results}}, nil
	}
	accountID := strings.TrimPrefix(path, "/admin/accounts/")
	accountID = strings.TrimSuffix(accountID, "/schedulable")
	a.apply(accountID, body)
	return map[string]any{"data": copyMap(a.states[accountID])}, nil
}

func TestBatchWriteCoordinatorRecognizesTopLevelPerAccountFailure(t *testing.T) {
	admin := &batchCoordinatorAdmin{
		states: map[string]map[string]any{
			"41": coordinatorState("41", 20),
			"42": coordinatorState("42", 20),
		},
		batchReply: map[string]any{"success": true, "results": []any{
			map[string]any{"account_id": int64(41), "success": false, "error": "account rejected"},
			map[string]any{"account_id": int64(42), "success": true},
		}},
	}
	coordinator := newBatchWriteCoordinator(context.Background(), admin, 2, false)
	results := make(chan coordinatedWriteOutcome, 2)
	for _, accountID := range []string{"41", "42"} {
		current, err := remoteValues(admin.states[accountID])
		if err != nil {
			t.Fatal(err)
		}
		go func(accountID string, current values) {
			results <- coordinator.Submit(context.Background(), accountID, map[string]any{"priority": int64(10)}, current)
		}(accountID, current)
	}
	outcomes := map[string]coordinatedWriteOutcome{}
	for range 2 {
		outcome := <-results
		accountID := "42"
		if outcome.err != nil {
			accountID = "41"
		}
		outcomes[accountID] = outcome
	}
	if outcomes["41"].err == nil || !strings.Contains(outcomes["41"].err.Error(), "account rejected") {
		t.Fatalf("failed account outcome=%#v", outcomes["41"])
	}
	if outcomes["42"].err != nil || !outcomes["42"].remoteConfirmed {
		t.Fatalf("successful account outcome=%#v", outcomes["42"])
	}
}

func (a *batchCoordinatorAdmin) apply(accountID string, body map[string]any) {
	if a.skipApply[accountID] {
		return
	}
	state := a.states[accountID]
	for field, value := range body {
		if field != "account_ids" {
			state[field] = value
		}
	}
}

func TestBatchWriteCoordinatorDoesNotAssumeOmittedAccountSucceeded(t *testing.T) {
	admin := &batchCoordinatorAdmin{
		states: map[string]map[string]any{
			"41": coordinatorState("41", 20),
			"42": coordinatorState("42", 20),
		},
		batchReply: map[string]any{"success": true, "results": []any{
			map[string]any{"account_id": int64(41), "success": true},
		}},
		skipApply: map[string]bool{"42": true},
	}
	coordinator := newBatchWriteCoordinator(context.Background(), admin, 2, false)
	type result struct {
		accountID string
		outcome   coordinatedWriteOutcome
	}
	results := make(chan result, 2)
	for _, accountID := range []string{"41", "42"} {
		current, err := remoteValues(admin.states[accountID])
		if err != nil {
			t.Fatal(err)
		}
		go func(accountID string, current values) {
			results <- result{accountID: accountID, outcome: coordinator.Submit(
				context.Background(), accountID, map[string]any{"priority": int64(10)}, current,
			)}
		}(accountID, current)
	}
	outcomes := map[string]coordinatedWriteOutcome{}
	for range 2 {
		item := <-results
		outcomes[item.accountID] = item.outcome
	}
	if outcomes["41"].err != nil || !outcomes["41"].remoteConfirmed {
		t.Fatalf("explicitly successful account outcome=%#v", outcomes["41"])
	}
	if outcomes["42"].err == nil || outcomes["42"].remoteConfirmed || outcomes["42"].readbackConfirmed {
		t.Fatalf("omitted account was accepted without matching readback: %#v", outcomes["42"])
	}
}

func coordinatorState(accountID string, priority int64) map[string]any {
	return map[string]any{
		"id": accountID, "schedulable": true, "priority": priority, "load_factor": int64(3),
		"concurrency": int64(4), "status": "active",
	}
}

func TestBatchWriteCoordinatorGroupsIdenticalUpdatesAndConfirmsOnlyChangedFields(t *testing.T) {
	admin := &batchCoordinatorAdmin{states: map[string]map[string]any{
		"41": coordinatorState("41", 20),
		"42": coordinatorState("42", 20),
	}, onList: func(states map[string]map[string]any) {
		states["41"]["status"] = "error"
	}}
	coordinator := newBatchWriteCoordinator(context.Background(), admin, 2, true)
	results := make(chan coordinatedWriteOutcome, 2)
	for _, accountID := range []string{"41", "42"} {
		accountID := accountID
		current, err := remoteValues(admin.states[accountID])
		if err != nil {
			t.Fatal(err)
		}
		go func() {
			results <- coordinator.Submit(context.Background(), accountID, map[string]any{"priority": int64(10)}, current)
		}()
	}
	for range 2 {
		outcome := <-results
		if outcome.err != nil || !outcome.remoteConfirmed || !outcome.readbackConfirmed {
			t.Fatalf("batch outcome=%#v", outcome)
		}
	}
	admin.mu.Lock()
	defer admin.mu.Unlock()
	if admin.mutates != 1 || admin.lists != 1 {
		t.Fatalf("identical updates should use one batch write and one batch confirmation: mutates=%d lists=%d", admin.mutates, admin.lists)
	}
}

func TestBatchWriteCoordinatorSkipsConfirmationWhenNothingChanges(t *testing.T) {
	admin := &batchCoordinatorAdmin{states: map[string]map[string]any{}}
	coordinator := newBatchWriteCoordinator(context.Background(), admin, 2, true)
	coordinator.Skip()
	coordinator.Skip()
	<-coordinator.done
	if admin.mutates != 0 || admin.lists != 0 {
		t.Fatalf("no-op accounts triggered remote work: mutates=%d lists=%d", admin.mutates, admin.lists)
	}
}

func TestBatchWriteCoordinatorRejectsChangedFieldMismatch(t *testing.T) {
	admin := &batchCoordinatorAdmin{
		states: map[string]map[string]any{
			"41": coordinatorState("41", 20),
			"42": coordinatorState("42", 20),
		},
		onList: func(states map[string]map[string]any) {
			states["41"]["priority"] = int64(20)
		},
	}
	coordinator := newBatchWriteCoordinator(context.Background(), admin, 2, true)
	results := map[string]coordinatedWriteOutcome{}
	var mutex sync.Mutex
	var wait sync.WaitGroup
	for _, accountID := range []string{"41", "42"} {
		current, err := remoteValues(admin.states[accountID])
		if err != nil {
			t.Fatal(err)
		}
		wait.Add(1)
		go func(accountID string, current values) {
			defer wait.Done()
			outcome := coordinator.Submit(context.Background(), accountID, map[string]any{"priority": int64(10)}, current)
			mutex.Lock()
			results[accountID] = outcome
			mutex.Unlock()
		}(accountID, current)
	}
	wait.Wait()
	if results["41"].err == nil || results["41"].readbackConfirmed || results["41"].after.priority == nil || *results["41"].after.priority != 20 {
		t.Fatalf("mismatched changed field was accepted: %#v", results["41"])
	}
	if results["42"].err != nil || !results["42"].readbackConfirmed {
		t.Fatalf("matching account was affected by another account mismatch: %#v", results["42"])
	}
}

func TestBatchWriteCoordinatorHonorsAccountConcurrencyLimit(t *testing.T) {
	admin := &batchCoordinatorAdmin{
		states: map[string]map[string]any{
			"41": coordinatorState("41", 20),
			"42": coordinatorState("42", 20),
			"43": coordinatorState("43", 20),
		},
		started: make(chan struct{}, 3), release: make(chan struct{}, 3),
	}
	limited := limitAdmin(admin, 2)
	coordinator := newBatchWriteCoordinator(context.Background(), limited, 3, false)
	var wait sync.WaitGroup
	for index, accountID := range []string{"41", "42", "43"} {
		current, err := remoteValues(admin.states[accountID])
		if err != nil {
			t.Fatal(err)
		}
		wait.Add(1)
		go func(accountID string, priority int64, current values) {
			defer wait.Done()
			outcome := coordinator.Submit(context.Background(), accountID, map[string]any{"priority": priority}, current)
			if outcome.err != nil {
				t.Errorf("write %s failed: %v", accountID, outcome.err)
			}
		}(accountID, int64(10+index), current)
	}
	<-admin.started
	<-admin.started
	admin.release <- struct{}{}
	<-admin.started
	admin.release <- struct{}{}
	admin.release <- struct{}{}
	wait.Wait()
	if admin.maxActive != 2 || admin.lists != 0 {
		t.Fatalf("maximum concurrent writes=%d lists=%d want max=2 and no confirmation", admin.maxActive, admin.lists)
	}
}

func TestBatchWriteCoordinatorCancelsGroupWhenOneAccountLeaseIsLost(t *testing.T) {
	admin := &batchCoordinatorAdmin{
		states: map[string]map[string]any{
			"41": coordinatorState("41", 20),
			"42": coordinatorState("42", 20),
		},
		started: make(chan struct{}, 1), release: make(chan struct{}),
	}
	coordinator := newBatchWriteCoordinator(context.Background(), admin, 2, false)
	firstCtx, loseFirstLease := context.WithCancelCause(context.Background())
	type result struct {
		accountID string
		outcome   coordinatedWriteOutcome
	}
	results := make(chan result, 2)
	for _, accountID := range []string{"41", "42"} {
		accountID := accountID
		requestCtx := context.Background()
		if accountID == "41" {
			requestCtx = firstCtx
		}
		current, err := remoteValues(admin.states[accountID])
		if err != nil {
			t.Fatal(err)
		}
		go func() {
			results <- result{accountID: accountID, outcome: coordinator.Submit(
				requestCtx, accountID, map[string]any{"priority": int64(10)}, current,
			)}
		}()
	}
	select {
	case <-admin.started:
	case <-time.After(3 * time.Second):
		t.Fatal("batch write did not start")
	}
	leaseErr := errors.New("account mutation lease lost")
	loseFirstLease(leaseErr)
	outcomes := map[string]coordinatedWriteOutcome{}
	for range 2 {
		select {
		case item := <-results:
			outcomes[item.accountID] = item.outcome
		case <-time.After(3 * time.Second):
			t.Fatal("batch write did not stop after lease loss")
		}
	}
	if !errors.Is(outcomes["41"].err, leaseErr) {
		t.Fatalf("lost lease outcome = %#v", outcomes["41"])
	}
	if outcomes["42"].err == nil || outcomes["42"].remoteConfirmed {
		t.Fatalf("shared batch continued after a member lease was lost: %#v", outcomes["42"])
	}
	admin.mu.Lock()
	defer admin.mu.Unlock()
	if admin.active != 0 {
		t.Fatalf("cancelled batch still active: %d", admin.active)
	}
}

func TestBatchWriteCoordinatorSubmitWaitsForRemoteCallAfterContextCancellation(t *testing.T) {
	admin := &batchCoordinatorAdmin{
		states: map[string]map[string]any{
			"41": coordinatorState("41", 20),
		},
		started: make(chan struct{}, 1), release: make(chan struct{}), ignoreCancellation: true,
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	coordinator := newBatchWriteCoordinator(ctx, admin, 1, false)
	current, err := remoteValues(admin.states["41"])
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan coordinatedWriteOutcome, 1)
	go func() {
		result <- coordinator.Submit(ctx, "41", map[string]any{"priority": int64(10)}, current)
	}()
	select {
	case <-admin.started:
	case <-time.After(3 * time.Second):
		t.Fatal("remote write did not start")
	}
	leaseErr := errors.New("account mutation lease lost")
	cancel(leaseErr)
	select {
	case outcome := <-result:
		t.Fatalf("submit returned before the remote write completed: %#v", outcome)
	case <-time.After(100 * time.Millisecond):
	}
	close(admin.release)
	select {
	case outcome := <-result:
		if !errors.Is(outcome.err, leaseErr) {
			t.Fatalf("cancelled submit outcome = %#v", outcome)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("submit did not return after the remote write completed")
	}
}

func TestParseWritePolicyDefaultsVerificationToOff(t *testing.T) {
	policy, err := parseWritePolicy(map[string]any{
		"auto_apply": map[string]any{},
		"weights":    map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if policy.maxConcurrency != 4 || policy.verifyAfterWrite {
		t.Fatalf("writeback defaults=%#v", policy)
	}

	policy, err = parseWritePolicy(map[string]any{
		"auto_apply": map[string]any{},
		"weights":    map[string]any{},
		"writeback":  map[string]any{"concurrency": int64(8), "verification": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if policy.maxConcurrency != 8 || !policy.verifyAfterWrite {
		t.Fatalf("configured writeback policy=%#v", policy)
	}
}
