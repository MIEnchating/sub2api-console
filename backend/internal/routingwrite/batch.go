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
	accountID string
	desired   map[string]any
	current   values
}

type coordinatedWriteOutcome struct {
	remoteConfirmed   bool
	readbackConfirmed bool
	after             values
	err               error
}

type batchWriteCoordinator struct {
	ctx          context.Context
	admin        Admin
	verification bool
	remaining    int

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

func (c *batchWriteCoordinator) Submit(ctx context.Context, accountID string, desired map[string]any, current values) coordinatedWriteOutcome {
	request := &coordinatedWriteRequest{accountID: accountID, desired: copyMap(desired), current: current}
	c.arrive(request)
	select {
	case <-c.done:
		c.mu.Lock()
		outcome := c.outcomes[accountID]
		c.mu.Unlock()
		return outcome
	case <-ctx.Done():
		return coordinatedWriteOutcome{err: ctx.Err()}
	}
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
	var wait sync.WaitGroup
	for _, group := range groups {
		group := group
		wait.Add(1)
		go func() {
			defer wait.Done()
			c.executeGroup(group)
		}()
	}
	wait.Wait()
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
	body["account_ids"] = accountIDs
	payload, err := c.admin.Mutate(c.ctx, http.MethodPost, "/admin/accounts/bulk-update", body)
	if isMissingBatchRoute(err) {
		var wait sync.WaitGroup
		for _, request := range validGroup {
			request := request
			wait.Add(1)
			go func() {
				defer wait.Done()
				c.executeSingle(request)
			}()
		}
		wait.Wait()
		return
	}
	if err != nil {
		for _, request := range validGroup {
			c.setOutcome(request.accountID, coordinatedWriteOutcome{err: err})
		}
		return
	}
	failures := batchUpdateFailures(payload)
	for _, request := range validGroup {
		if cause, failed := failures[request.accountID]; failed {
			c.setOutcome(request.accountID, coordinatedWriteOutcome{err: cause})
			continue
		}
		c.setOutcome(request.accountID, coordinatedWriteOutcome{
			remoteConfirmed: true,
			after:           valuesWithDesired(request.current, request.desired),
		})
	}
}

func (c *batchWriteCoordinator) executeSingle(request coordinatedWriteRequest) {
	write := writeRoutingValues(c.ctx, c.admin, request.accountID, request.desired)
	outcome := coordinatedWriteOutcome{remoteConfirmed: write.remoteConfirmed, err: write.err}
	if write.err != nil && write.remoteConfirmed {
		payload, err := c.admin.Account(c.ctx, request.accountID)
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
			c.updateVerification(request.accountID, values{}, err)
		}
		return
	}
	for _, request := range requests {
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

func (c *batchWriteCoordinator) successfulRequests() []coordinatedWriteRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := []coordinatedWriteRequest{}
	for accountID, request := range c.pending {
		outcome := c.outcomes[accountID]
		if outcome.remoteConfirmed && outcome.err == nil {
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

func mutationResponseValues(payload map[string]any, accountID string) (values, bool) {
	if payload == nil {
		return values{}, false
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		data = payload
	}
	if strings.TrimSpace(fmt.Sprint(data["id"])) != accountID {
		return values{}, false
	}
	for _, field := range []string{"schedulable", "priority", "load_factor", "concurrency", "status"} {
		if _, present := data[field]; !present {
			return values{}, false
		}
	}
	after, err := remoteValues(data)
	return after, err == nil
}

func batchUpdateFailures(payload map[string]any) map[string]error {
	result := map[string]error{}
	data, _ := payload["data"].(map[string]any)
	failedIDs, _ := data["failed_ids"].([]any)
	for _, rawID := range failedIDs {
		accountID := strings.TrimSpace(fmt.Sprint(rawID))
		if accountID != "" {
			result[accountID] = errors.New("批量更新失败")
		}
	}
	items, _ := data["results"].([]any)
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if item == nil || item["success"] == true {
			continue
		}
		accountID := strings.TrimSpace(fmt.Sprint(item["account_id"]))
		detail := strings.TrimSpace(fmt.Sprint(item["error"]))
		if detail == "" {
			detail = "批量更新失败"
		}
		result[accountID] = errors.New(detail)
	}
	return result
}

func isMissingBatchRoute(err error) bool {
	var httpError *adminclient.HTTPError
	return errors.As(err, &httpError) && (httpError.StatusCode == http.StatusNotFound || httpError.StatusCode == http.StatusMethodNotAllowed)
}
