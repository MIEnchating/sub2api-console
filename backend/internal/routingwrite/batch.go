package routingwrite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
)

type accountLister interface {
	Accounts(context.Context) ([]map[string]any, error)
}

type configurableAccountDeleter interface {
	DeleteAccountWithVerification(context.Context, string, bool) (map[string]any, error)
}

type limitedAdmin struct {
	admin Admin
	gate  chan struct{}
}

func limitAdmin(admin Admin, maximum int) Admin {
	if maximum < 1 {
		maximum = 1
	}
	return &limitedAdmin{admin: admin, gate: make(chan struct{}, maximum)}
}

func (a *limitedAdmin) Account(ctx context.Context, accountID string) (map[string]any, error) {
	if err := a.acquire(ctx); err != nil {
		return nil, err
	}
	defer a.release()
	return a.admin.Account(ctx, accountID)
}

func (a *limitedAdmin) Accounts(ctx context.Context) ([]map[string]any, error) {
	lister, ok := a.admin.(accountLister)
	if !ok {
		return nil, errors.New("管理接口不支持批量读取账号")
	}
	if err := a.acquire(ctx); err != nil {
		return nil, err
	}
	defer a.release()
	return lister.Accounts(ctx)
}

func (a *limitedAdmin) Mutate(ctx context.Context, method, path string, body map[string]any) (map[string]any, error) {
	if err := a.acquire(ctx); err != nil {
		return nil, err
	}
	defer a.release()
	if err := adminclient.AuthorizeMutation(ctx); err != nil {
		return nil, err
	}
	return a.admin.Mutate(ctx, method, path, body)
}

func (a *limitedAdmin) DeleteAccount(ctx context.Context, accountID string) (map[string]any, error) {
	return a.DeleteAccountWithVerification(ctx, accountID, true)
}

func (a *limitedAdmin) DeleteAccountWithVerification(ctx context.Context, accountID string, verification bool) (map[string]any, error) {
	if err := a.acquire(ctx); err != nil {
		return nil, err
	}
	defer a.release()
	if err := adminclient.AuthorizeMutation(ctx); err != nil {
		return nil, err
	}
	if configurable, ok := a.admin.(configurableAccountDeleter); ok {
		return configurable.DeleteAccountWithVerification(ctx, accountID, verification)
	}
	if !verification {
		return a.admin.Mutate(ctx, http.MethodDelete, "/admin/accounts/"+accountID, nil)
	}
	return a.admin.DeleteAccount(ctx, accountID)
}

func deleteAccount(ctx context.Context, admin Admin, accountID string, verification bool) (map[string]any, error) {
	if configurable, ok := admin.(configurableAccountDeleter); ok {
		return configurable.DeleteAccountWithVerification(ctx, accountID, verification)
	}
	if !verification {
		return admin.Mutate(ctx, http.MethodDelete, "/admin/accounts/"+accountID, nil)
	}
	return admin.DeleteAccount(ctx, accountID)
}

func (a *limitedAdmin) acquire(ctx context.Context) error {
	select {
	case a.gate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *limitedAdmin) release() { <-a.gate }

type coordinatedWriteRequest struct {
	ctx       context.Context
	accountID string
	desired   map[string]any
	current   values
}

type coordinatedWriteOutcome struct {
	remoteConfirmed   bool
	readbackConfirmed bool
	writePrevented    bool
	after             values
	err               error
}

type batchWriteCoordinator struct {
	ctx          context.Context
	admin        Admin
	verification bool
	remaining    int
	capacity     *upstreamCapacityGuard
	authorize    func() error

	mu       sync.Mutex
	pending  map[string]coordinatedWriteRequest
	outcomes map[string]coordinatedWriteOutcome
	done     chan struct{}
}

func newBatchWriteCoordinator(ctx context.Context, admin Admin, total int, verification bool) *batchWriteCoordinator {
	return &batchWriteCoordinator{
		ctx: ctx, admin: admin, verification: verification, remaining: total,
		pending: map[string]coordinatedWriteRequest{}, outcomes: map[string]coordinatedWriteOutcome{}, done: make(chan struct{}),
	}
}

func (c *batchWriteCoordinator) Skip() {
	c.arrive(nil)
}

// enqueue never waits: every account must finish preparation before writes start.
func (c *batchWriteCoordinator) enqueue(ctx context.Context, accountID string, desired map[string]any, current values) {
	if ctx == nil {
		ctx = context.Background()
	}
	c.arrive(&coordinatedWriteRequest{ctx: ctx, accountID: accountID, desired: copyMap(desired), current: current})
}

func (c *batchWriteCoordinator) Submit(ctx context.Context, accountID string, desired map[string]any, current values) coordinatedWriteOutcome {
	c.enqueue(ctx, accountID, desired, current)
	return c.outcome(ctx, accountID)
}

func (c *batchWriteCoordinator) outcome(ctx context.Context, accountID string) coordinatedWriteOutcome {
	<-c.done
	c.mu.Lock()
	outcome := c.outcomes[accountID]
	c.mu.Unlock()
	if cause := contextCause(ctx); cause != nil {
		outcome.err = errors.Join(outcome.err, cause)
	}
	return outcome
}

func (c *batchWriteCoordinator) arrive(request *coordinatedWriteRequest) {
	c.mu.Lock()
	if request != nil {
		c.pending[request.accountID] = *request
	}
	c.remaining--
	ready := c.remaining == 0
	c.mu.Unlock()
	if ready {
		go c.execute()
	}
}

func (c *batchWriteCoordinator) execute() {
	groups := c.groups()
	if c.authorize != nil {
		if err := c.authorize(); err != nil {
			for _, group := range groups {
				for _, request := range group {
					c.setOutcome(request.accountID, coordinatedWriteOutcome{writePrevented: true, err: err})
				}
			}
			close(c.done)
			return
		}
	}
	runBounded(len(groups), c.workerLimit(), func(index int) { c.executeGroup(groups[index]) })
	if c.verification {
		c.verifySuccessfulWrites()
	}
	close(c.done)
}

func (c *batchWriteCoordinator) groups() [][]coordinatedWriteRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	byPayload := map[string][]coordinatedWriteRequest{}
	keys := []string{}
	for _, request := range c.pending {
		encoded, _ := json.Marshal(request.desired)
		key := string(encoded)
		if _, present := byPayload[key]; !present {
			keys = append(keys, key)
		}
		byPayload[key] = append(byPayload[key], request)
	}
	sort.Strings(keys)
	result := make([][]coordinatedWriteRequest, 0, len(keys))
	for _, key := range keys {
		group := byPayload[key]
		sort.Slice(group, func(left, right int) bool { return group[left].accountID < group[right].accountID })
		result = append(result, group)
	}
	return result
}

func (c *batchWriteCoordinator) executeGroup(group []coordinatedWriteRequest) {
	// Sub2API's bulk service drops a zero load factor before reaching the
	// repository. Clear it through the individual route, which supports NULL.
	// Recovery also needs an individual concurrency read before enabling traffic.
	if clearsLoadFactor(group[0].desired) || requiresConcurrencyConfirmation(group[0].desired) {
		runBounded(len(group), c.workerLimit(), func(index int) { c.executeSingle(group[index]) })
		return
	}
	if len(group) == 1 {
		c.executeSingle(group[0])
		return
	}
	body := copyMap(group[0].desired)
	accountIDs := make([]int64, 0, len(group))
	validGroup := make([]coordinatedWriteRequest, 0, len(group))
	for _, request := range group {
		accountID, err := strconv.ParseInt(request.accountID, 10, 64)
		if err != nil || accountID < 1 {
			c.setOutcome(request.accountID, coordinatedWriteOutcome{err: errors.New("账号 ID 必须是稳定正整数")})
			continue
		}
		accountIDs = append(accountIDs, accountID)
		validGroup = append(validGroup, request)
	}
	if len(validGroup) == 0 {
		return
	}
	groupCtx, cancelGroup := combinedRequestContext(c.ctx, validGroup)
	defer cancelGroup()
	body["account_ids"] = accountIDs
	payload, err := c.admin.Mutate(groupCtx, http.MethodPost, "/admin/accounts/bulk-update", body)
	if isMissingBatchRoute(err) {
		runBounded(len(validGroup), c.workerLimit(), func(index int) { c.executeSingle(validGroup[index]) })
		return
	}
	if err != nil {
		for _, request := range validGroup {
			c.setOutcome(request.accountID, coordinatedWriteOutcome{writePrevented: mutationPrevented(err), err: err})
		}
		return
	}
	requested := make(map[string]struct{}, len(validGroup))
	for _, request := range validGroup {
		requested[request.accountID] = struct{}{}
	}
	acknowledgements, malformed := batchUpdateAcknowledgements(payload, requested)
	ambiguous := make([]coordinatedWriteRequest, 0)
	for _, request := range validGroup {
		if cause := contextCause(request.ctx); cause != nil {
			c.setOutcome(request.accountID, coordinatedWriteOutcome{err: cause})
			continue
		}
		accountAcknowledgements := acknowledgements[request.accountID]
		if malformed || len(accountAcknowledgements) != 1 || !accountAcknowledgements[0].valid {
			ambiguous = append(ambiguous, request)
			continue
		}
		if cause := accountAcknowledgements[0].err; cause != nil {
			c.setOutcome(request.accountID, coordinatedWriteOutcome{err: cause})
			continue
		}
		c.setOutcome(request.accountID, coordinatedWriteOutcome{
			remoteConfirmed: true,
			after:           valuesWithDesired(request.current, request.desired),
		})
	}
	c.confirmAmbiguousBatchWrites(ambiguous)
}

func (c *batchWriteCoordinator) workerLimit() int {
	if limited, ok := c.admin.(*limitedAdmin); ok {
		return max(1, cap(limited.gate))
	}
	return 1
}

func (c *batchWriteCoordinator) confirmAmbiguousBatchWrites(requests []coordinatedWriteRequest) {
	runBounded(len(requests), c.workerLimit(), func(index int) {
		request := requests[index]
		if cause := contextCause(request.ctx); cause != nil {
			c.setOutcome(request.accountID, coordinatedWriteOutcome{err: cause})
			return
		}
		c.confirmAmbiguousBatchWrite(request, errors.New("批量更新响应未唯一确认账号 "+request.accountID))
	})
}

func (c *batchWriteCoordinator) confirmAmbiguousBatchWrite(request coordinatedWriteRequest, ambiguity error) {
	payload, err := c.admin.Account(request.ctx, request.accountID)
	if err != nil {
		c.setOutcome(request.accountID, coordinatedWriteOutcome{err: errors.Join(ambiguity, fmt.Errorf("逐项读回失败：%w", err))})
		return
	}
	after, err := remoteValues(payload)
	if err == nil {
		err = verifyReadback(request.desired, after)
	}
	if err != nil {
		c.setOutcome(request.accountID, coordinatedWriteOutcome{after: after, err: errors.Join(ambiguity, fmt.Errorf("逐项读回未确认目标：%w", err))})
		return
	}
	c.setOutcome(request.accountID, coordinatedWriteOutcome{
		remoteConfirmed: true, readbackConfirmed: true, after: after,
	})
}

func (c *batchWriteCoordinator) executeSingle(request coordinatedWriteRequest) {
	write := writeRoutingValues(request.ctx, c.admin, request.accountID, request.desired)
	outcome := coordinatedWriteOutcome{remoteConfirmed: write.remoteConfirmed, writePrevented: write.writePrevented, err: write.err}
	if write.err == nil && clearsLoadFactor(request.desired) {
		// A reset cannot be inferred from the submitted zero or a success count.
		// Require a fresh account read, accepting the full DTO's omitted nullable
		// field even when optional verification is disabled, before release.
		payload, err := c.admin.Account(request.ctx, request.accountID)
		if err == nil {
			outcome.after, err = remoteValues(payload)
		}
		if err == nil {
			err = verifyReadback(request.desired, outcome.after)
		}
		outcome.err, outcome.readbackConfirmed = err, err == nil
		c.setOutcome(request.accountID, outcome)
		return
	}
	if write.err != nil && write.remoteConfirmed {
		payload, err := c.admin.Account(request.ctx, request.accountID)
		if err != nil {
			outcome.err = errors.Join(write.err, fmt.Errorf("部分写回后的读取失败：%w", err))
			c.setOutcome(request.accountID, outcome)
			return
		}
		after, err := remoteValues(payload)
		if err != nil {
			outcome.err = errors.Join(write.err, err)
			c.setOutcome(request.accountID, outcome)
			return
		}
		outcome.after, outcome.readbackConfirmed = after, true
		c.setOutcome(request.accountID, outcome)
		return
	}
	if write.err == nil {
		if after, trusted := mutationResponseValues(write.payload, request.accountID); trusted {
			outcome.after = after
		} else {
			outcome.after = valuesWithDesired(request.current, request.desired)
		}
	}
	c.setOutcome(request.accountID, outcome)
}

func (c *batchWriteCoordinator) verifySuccessfulWrites() {
	requests := c.successfulRequests()
	if len(requests) == 0 {
		return
	}
	accounts, err := listAccountPayloads(c.ctx, c.admin)
	if err != nil {
		for _, request := range requests {
			cause := contextCause(request.ctx)
			if cause == nil {
				cause = err
			}
			c.updateVerification(request.accountID, values{}, cause)
		}
		return
	}
	for _, request := range requests {
		if cause := contextCause(request.ctx); cause != nil {
			c.updateVerification(request.accountID, values{}, cause)
			continue
		}
		payload, present := accounts[request.accountID]
		if !present {
			c.updateVerification(request.accountID, values{}, errors.New("批量确认结果缺少账号 "+request.accountID))
			continue
		}
		after, err := remoteValues(payload)
		if err == nil {
			err = verifyReadback(request.desired, after)
		}
		c.updateVerification(request.accountID, after, err)
	}
}

func combinedRequestContext(parent context.Context, requests []coordinatedWriteRequest) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)
	// Requests normally inherit the same task context. Watch only distinct
	// cancellation signals; AfterFunc does not park one goroutine per account.
	seen := map[<-chan struct{}]struct{}{parent.Done(): {}}
	stops := make([]func() bool, 0)
	for _, request := range requests {
		if request.ctx == nil || request.ctx.Done() == nil {
			continue
		}
		if request.ctx.Err() != nil {
			cancel()
			break
		}
		signal := request.ctx.Done()
		if _, found := seen[signal]; found {
			continue
		}
		seen[signal] = struct{}{}
		stops = append(stops, context.AfterFunc(request.ctx, cancel))
	}
	return ctx, func() {
		for _, stop := range stops {
			stop()
		}
		cancel()
	}
}

func contextCause(ctx context.Context) error {
	if ctx == nil || ctx.Err() == nil {
		return nil
	}
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	return ctx.Err()
}

func (c *batchWriteCoordinator) successfulRequests() []coordinatedWriteRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := []coordinatedWriteRequest{}
	for accountID, request := range c.pending {
		outcome := c.outcomes[accountID]
		if outcome.remoteConfirmed && !outcome.readbackConfirmed && outcome.err == nil {
			result = append(result, request)
		}
	}
	sort.Slice(result, func(left, right int) bool { return result[left].accountID < result[right].accountID })
	return result
}

func (c *batchWriteCoordinator) updateVerification(accountID string, after values, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	outcome := c.outcomes[accountID]
	if after != (values{}) {
		outcome.after = after
	}
	if err != nil {
		outcome.err = err
	} else {
		outcome.readbackConfirmed = true
	}
	c.outcomes[accountID] = outcome
}

func (c *batchWriteCoordinator) setOutcome(accountID string, outcome coordinatedWriteOutcome) {
	c.mu.Lock()
	c.outcomes[accountID] = outcome
	c.mu.Unlock()
}

func listAccountPayloads(ctx context.Context, admin Admin) (map[string]map[string]any, error) {
	lister, ok := admin.(accountLister)
	if !ok {
		return nil, errors.New("管理接口不支持批量读取账号")
	}
	items, err := lister.Accounts(ctx)
	if err != nil {
		return nil, err
	}
	result := make(map[string]map[string]any, len(items))
	for _, item := range items {
		accountID := strings.TrimSpace(fmt.Sprint(item["id"]))
		if accountID != "" {
			result[accountID] = item
		}
	}
	return result, nil
}

func valuesWithDesired(current values, desired map[string]any) values {
	result := current
	if raw, present := desired["schedulable"]; present {
		value := raw.(bool)
		result.schedulable = &value
	}
	if raw, present := desired["priority"]; present {
		value, _ := integer(raw)
		result.priority = &value
	}
	if raw, present := desired["load_factor"]; present {
		value, _ := optionalNonnegativeIntegerText(raw)
		result.loadFactor = value
		result.loadFactorPresent = true
	}
	if raw, present := desired["concurrency"]; present {
		value, _ := integer(raw)
		result.concurrency = &value
	}
	if raw, present := desired["status"]; present {
		value := strings.TrimSpace(fmt.Sprint(raw))
		result.status = &value
	}
	return result
}

func clearsLoadFactor(desired map[string]any) bool {
	raw, present := desired["load_factor"]
	if !present {
		return false
	}
	value, err := optionalNonnegativeIntegerText(raw)
	return err == nil && value != nil && *value == "0"
}

func requiresConcurrencyConfirmation(desired map[string]any) bool {
	enabled, _ := desired["schedulable"].(bool)
	_, changesConcurrency := desired["concurrency"]
	return enabled && changesConcurrency
}

func mutationResponseValues(payload map[string]any, accountID string) (values, bool) {
	if payload == nil {
		return values{}, false
	}
	for _, data := range mutationResponseObjects(payload) {
		if strings.TrimSpace(fmt.Sprint(data["id"])) != accountID {
			continue
		}
		complete := true
		for _, field := range []string{"schedulable", "priority", "load_factor", "concurrency", "status"} {
			if _, present := data[field]; !present {
				complete = false
				break
			}
		}
		if !complete {
			continue
		}
		after, err := remoteValues(data)
		return after, err == nil
	}
	return values{}, false
}

type batchUpdateAcknowledgement struct {
	valid bool
	err   error
}

func batchUpdateAcknowledgements(payload map[string]any, requested map[string]struct{}) (map[string][]batchUpdateAcknowledgement, bool) {
	result := map[string][]batchUpdateAcknowledgement{}
	sources := map[string]map[string]bool{}
	malformed := false
	appendAcknowledgement := func(source, accountID string, acknowledgement batchUpdateAcknowledgement) {
		if !stableRoutingAccountID(accountID) {
			malformed = true
			return
		}
		if _, expected := requested[accountID]; !expected {
			malformed = true
		}
		if sources[accountID] == nil {
			sources[accountID] = map[string]bool{}
		}
		duplicate := sources[accountID][source]
		sources[accountID][source] = true
		previous := result[accountID]
		// Summary ID lists and detailed results may describe the same outcome.
		// Merge only across different fields; duplicate IDs within a field,
		// invalid acknowledgements and conflicting outcomes remain ambiguous.
		if !duplicate && len(previous) == 1 && previous[0].valid && acknowledgement.valid &&
			(previous[0].err == nil) == (acknowledgement.err == nil) {
			if source == "results" {
				result[accountID][0] = acknowledgement
			}
			return
		}
		result[accountID] = append(result[accountID], acknowledgement)
	}
	for _, data := range mutationResponseObjects(payload) {
		for _, list := range []struct {
			key string
			err error
		}{
			{key: "failed_ids", err: errors.New("批量更新失败")},
			{key: "success_ids"},
			{key: "successful_ids"},
			{key: "updated_ids"},
		} {
			rawIDs, present := data[list.key]
			if !present {
				continue
			}
			ids, ok := rawIDs.([]any)
			if !ok {
				malformed = true
				continue
			}
			for _, rawID := range ids {
				appendAcknowledgement(list.key, capacityAccountID(rawID), batchUpdateAcknowledgement{valid: true, err: list.err})
			}
		}
		rawItems, present := data["results"]
		if !present {
			continue
		}
		items, ok := rawItems.([]any)
		if !ok {
			malformed = true
			continue
		}
		for _, raw := range items {
			item, ok := raw.(map[string]any)
			if !ok || item == nil {
				malformed = true
				continue
			}
			accountID := batchResultAccountID(item)
			success, valid := item["success"].(bool)
			acknowledgement := batchUpdateAcknowledgement{valid: valid}
			if valid && !success {
				detail, _ := item["error"].(string)
				detail = strings.TrimSpace(detail)
				if detail == "" {
					detail = "批量更新失败"
				}
				acknowledgement.err = errors.New(detail)
			}
			appendAcknowledgement("results", accountID, acknowledgement)
		}
	}
	return result, malformed
}

func batchResultAccountID(item map[string]any) string {
	accountID := ""
	for _, field := range []string{"account_id", "id"} {
		raw, present := item[field]
		if !present || raw == nil {
			continue
		}
		id := capacityAccountID(raw)
		if id == "" || accountID != "" && accountID != id {
			return ""
		}
		accountID = id
	}
	return accountID
}

func mutationResponseObjects(payload map[string]any) []map[string]any {
	if payload == nil {
		return nil
	}
	type entry struct {
		value map[string]any
		depth int
	}
	queue := []entry{{value: payload}}
	result := []map[string]any{}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		result = append(result, current.value)
		if current.depth >= 4 {
			continue
		}
		for _, key := range []string{"data", "result", "account", "item", "record"} {
			if nested, ok := current.value[key].(map[string]any); ok {
				queue = append(queue, entry{value: nested, depth: current.depth + 1})
			}
		}
	}
	return result
}

func isMissingBatchRoute(err error) bool {
	var httpError *adminclient.HTTPError
	return errors.As(err, &httpError) && (httpError.StatusCode == http.StatusNotFound || httpError.StatusCode == http.StatusMethodNotAllowed)
}
