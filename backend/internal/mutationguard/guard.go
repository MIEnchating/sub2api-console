package mutationguard

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

const (
	leaseTTL         = 2 * time.Minute
	renewalInterval  = 20 * time.Second
	retryInterval    = 25 * time.Millisecond
	leaseCallTimeout = 5 * time.Second
)

var (
	// ErrPartialResourceOverlap means a nested acquisition overlaps an existing
	// lease without being fully covered by it.
	ErrPartialResourceOverlap = errors.New("部分变更资源已由当前 context 持有，请一次性获取全部变更资源")
	// ErrResourceOrderViolation means a nested acquisition tried to lock a new
	// resource before one already held by the current context.
	ErrResourceOrderViolation = errors.New("嵌套变更资源违反全局词典序锁顺序，请按顺序获取资源")
)

type leaseStore interface {
	AcquireMutationLease(context.Context, string, []string, time.Time, time.Duration) (bool, error)
	RenewMutationLease(context.Context, string, []string, time.Time, time.Duration) (bool, error)
	ReleaseMutationLease(context.Context, string, []string) error
}

type resourceResolver interface {
	ResolveMutationResources(context.Context, []string) ([]string, error)
}

type heldResourcesKey struct{}

type heldResourceState struct {
	parent    *heldResourceState
	requested map[string]struct{}
	locked    map[string]struct{}
	active    atomic.Bool
}

type localLockSet struct {
	mu      sync.Mutex
	entries map[string]*localLock
}

type localLock struct {
	ready chan struct{}
	refs  int
}

type leaseDeadline struct {
	mu        sync.Mutex
	expiresAt time.Time
	changed   chan struct{}
	attempt   *renewalAttempt
	expired   bool
}

type renewalAttempt struct {
	ready    chan struct{}
	result   renewalResult
	accepted bool
}

type renewalResult struct {
	renewed     bool
	err         error
	completedAt time.Time
	expiresAt   time.Time
}

var fallbackLocks localLockSet

func Account(accountID string) string { return "account/" + strings.TrimSpace(accountID) }

func AccountCatalog() string { return "account-catalog" }

func ManagementTarget() string { return "management-target" }

func UpstreamCatalog() string { return "upstream-catalog" }

func Vault(entry string) string { return "vault/" + strings.TrimSpace(entry) }

func Upstream(host string) string {
	host = configstore.CanonicalHost(host)
	if host == "" {
		return ""
	}
	return "upstream/" + host
}

// Acquire returns a context cancelled on lease loss and an idempotent release.
func Acquire(ctx context.Context, repository any, resources ...string) (context.Context, func() error, error) {
	return acquire(ctx, repository, time.Now, resources...)
}

func acquire(
	ctx context.Context,
	repository any,
	now func() time.Time,
	resources ...string,
) (context.Context, func() error, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	requested, err := normalizeResources(resources)
	if err != nil {
		return nil, nil, err
	}
	resolver, resolves := repository.(resourceResolver)
	if !resolves && holdsLockedResources(ctx, requested) {
		return ctx, func() error { return nil }, nil
	}
	store, backed := repository.(leaseStore)
	if !backed {
		for {
			resolved, err := resolveResources(ctx, resolver, resolves, requested)
			if err != nil {
				return nil, nil, err
			}
			if held, err := resolvedResourcesHeld(ctx, requested, resolved, resolves); err != nil {
				return nil, nil, err
			} else if held {
				return ctx, func() error { return nil }, nil
			}
			release, err := fallbackLocks.acquire(ctx, resolved)
			if err != nil {
				return nil, nil, err
			}
			confirmed, err := resolveResources(ctx, resolver, resolves, requested)
			if err != nil {
				release()
				return nil, nil, err
			}
			if !slices.Equal(resolved, confirmed) {
				release()
				continue
			}
			guarded, held := withHeldResources(ctx, requested, resolved, resolves)
			return guarded, func() error {
				held.active.Store(false)
				release()
				return nil
			}, nil
		}
	}
	ownerID, err := randomOwnerID()
	if err != nil {
		return nil, nil, err
	}
	for {
		resolved, err := resolveResources(ctx, resolver, resolves, requested)
		if err != nil {
			return nil, nil, err
		}
		if held, err := resolvedResourcesHeld(ctx, requested, resolved, resolves); err != nil {
			return nil, nil, err
		} else if held {
			return ctx, func() error { return nil }, nil
		}
		var expiresAt time.Time
		for {
			attemptedAt := now().UTC()
			acquired, acquireErr := store.AcquireMutationLease(ctx, ownerID, resolved, attemptedAt, leaseTTL)
			if acquireErr != nil {
				if contextErr := ctx.Err(); contextErr != nil {
					return nil, nil, contextErr
				}
				return nil, nil, acquireErr
			}
			if acquired {
				expiresAt = attemptedAt.Add(leaseTTL)
				if now().UTC().Add(renewalInterval + leaseCallTimeout).Before(expiresAt) {
					break
				}
				if releaseErr := releaseLease(store, ownerID, resolved); releaseErr != nil {
					return nil, nil, fmt.Errorf("获取变更租约后剩余时间不足且释放失败：%w", releaseErr)
				}
			}
			timer := time.NewTimer(retryInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, nil, ctx.Err()
			case <-timer.C:
			}
		}
		confirmed, err := resolveResources(ctx, resolver, resolves, requested)
		if err != nil {
			return nil, nil, errors.Join(err, releaseLease(store, ownerID, resolved))
		}
		if !slices.Equal(resolved, confirmed) {
			if err := releaseLease(store, ownerID, resolved); err != nil {
				return nil, nil, fmt.Errorf("变更资源已变化且旧租约释放失败：%w", err)
			}
			continue
		}
		guarded, release := maintainLease(ctx, store, ownerID, resolved, expiresAt, leaseTTL, renewalInterval)
		guarded, held := withHeldResources(guarded, requested, resolved, resolves)
		return guarded, func() error {
			held.active.Store(false)
			return release()
		}, nil
	}
}

func resolveResources(ctx context.Context, resolver resourceResolver, resolves bool, requested []string) ([]string, error) {
	if !resolves {
		return append([]string{}, requested...), nil
	}
	resolved, err := resolver.ResolveMutationResources(ctx, append([]string{}, requested...))
	if err != nil {
		return nil, err
	}
	return normalizeResources(resolved)
}

func releaseLease(store leaseStore, ownerID string, resources []string) error {
	releaseCtx, releaseCancel := context.WithTimeout(context.Background(), leaseCallTimeout)
	defer releaseCancel()
	return store.ReleaseMutationLease(releaseCtx, ownerID, resources)
}

func holdsLockedResources(ctx context.Context, resources []string) bool {
	held, missing := heldResourceCoverage(ctx, resources, func(state *heldResourceState) map[string]struct{} {
		return state.locked
	})
	return held && !missing
}

func resolvedResourcesHeld(
	ctx context.Context,
	requested []string,
	resolved []string,
	resolves bool,
) (bool, error) {
	held, missing := heldResourceCoverage(ctx, resolved, func(state *heldResourceState) map[string]struct{} {
		return state.locked
	})
	if held && missing {
		return false, ErrPartialResourceOverlap
	}
	if held {
		return true, nil
	}
	if resolves {
		requestedHeld, _ := heldResourceCoverage(ctx, requested, func(state *heldResourceState) map[string]struct{} {
			return state.requested
		})
		if requestedHeld {
			return false, ErrPartialResourceOverlap
		}
	}
	if missing && nestedResourceOrderViolated(ctx, resolved) {
		return false, ErrResourceOrderViolation
	}
	return false, nil
}

func heldResourceCoverage(
	ctx context.Context,
	resources []string,
	namespace func(*heldResourceState) map[string]struct{},
) (held, missing bool) {
	for _, resource := range resources {
		found := false
		for held, _ := ctx.Value(heldResourcesKey{}).(*heldResourceState); held != nil; held = held.parent {
			if !held.active.Load() {
				continue
			}
			if _, present := namespace(held)[resource]; present {
				found = true
				break
			}
		}
		if !found {
			missing = true
		} else {
			held = true
		}
	}
	return held, missing
}

func nestedResourceOrderViolated(ctx context.Context, resources []string) bool {
	var lastLocked string
	hasLocked := false
	for held, _ := ctx.Value(heldResourcesKey{}).(*heldResourceState); held != nil; held = held.parent {
		if !held.active.Load() {
			continue
		}
		for resource := range held.locked {
			if !hasLocked || resource > lastLocked {
				lastLocked = resource
				hasLocked = true
			}
		}
	}
	return hasLocked && resources[0] <= lastLocked
}

func withHeldResources(
	ctx context.Context,
	requested []string,
	locked []string,
	resolves bool,
) (context.Context, *heldResourceState) {
	previous, _ := ctx.Value(heldResourcesKey{}).(*heldResourceState)
	held := &heldResourceState{
		parent:    previous,
		requested: make(map[string]struct{}, len(requested)),
		locked:    make(map[string]struct{}, len(locked)),
	}
	if resolves {
		for _, resource := range requested {
			held.requested[resource] = struct{}{}
		}
	}
	for _, resource := range locked {
		held.locked[resource] = struct{}{}
	}
	held.active.Store(true)
	return context.WithValue(ctx, heldResourcesKey{}, held), held
}

func maintainLease(
	ctx context.Context,
	store leaseStore,
	ownerID string,
	resources []string,
	expiresAt time.Time,
	ttl time.Duration,
	interval time.Duration,
) (context.Context, func() error) {
	guarded, cancel := context.WithCancelCause(ctx)
	renewalCtx, stopRenewal := context.WithCancel(guarded)
	done := make(chan struct{})
	go renew(renewalCtx, cancel, store, ownerID, resources, expiresAt, ttl, interval, done)
	var once sync.Once
	var releaseErr error
	release := func() error {
		once.Do(func() {
			stopRenewal()
			<-done
			cancel(errors.New("变更租约已释放"))
			releaseCtx, releaseCancel := context.WithTimeout(context.Background(), leaseCallTimeout)
			defer releaseCancel()
			releaseErr = store.ReleaseMutationLease(releaseCtx, ownerID, resources)
		})
		return releaseErr
	}
	return guarded, release
}

func renew(
	ctx context.Context,
	cancel context.CancelCauseFunc,
	store leaseStore,
	ownerID string,
	resources []string,
	expiresAt time.Time,
	ttl time.Duration,
	interval time.Duration,
	done chan<- struct{},
) {
	defer close(done)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	deadline := newLeaseDeadline(expiresAt)
	watchdogDone := make(chan struct{})
	go watchLeaseExpiry(ctx, cancel, deadline, watchdogDone)
	defer func() { <-watchdogDone }()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if ctx.Err() != nil {
				return
			}
			now := time.Now().UTC()
			expiresAt = deadline.current()
			if !now.Before(expiresAt) {
				cancel(errors.New("变更租约已到期"))
				return
			}
			attemptDeadline := minTime(now.Add(leaseCallTimeout), expiresAt)
			renewCtx, renewCancel := context.WithDeadline(ctx, attemptDeadline)
			attempt := deadline.beginRenewal()
			renewed, err := store.RenewMutationLease(renewCtx, ownerID, resources, now, ttl)
			renewCancel()
			completedAt := time.Now().UTC()
			// Publish before taking the deadline lock so a stale timer can adopt this result.
			attempt.complete(renewed, err, completedAt, now.Add(ttl))
			accepted := deadline.finishRenewal(attempt)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				if !completedAt.Before(expiresAt) {
					cancel(errors.New("变更租约在续期确认前已到期"))
					return
				}
				if completedAt.Before(expiresAt.Add(-interval)) {
					continue
				}
				cancel(fmt.Errorf("变更租约在到期前无法续期：%w", err))
				return
			}
			if !renewed {
				cancel(errors.New("变更租约已失效"))
				return
			}
			if !accepted {
				cancel(errors.New("变更租约在续期确认前已到期"))
				return
			}
		}
	}
}

func watchLeaseExpiry(
	ctx context.Context,
	cancel context.CancelCauseFunc,
	deadline *leaseDeadline,
	done chan<- struct{},
) {
	defer close(done)
	timer := time.NewTimer(time.Until(deadline.current()))
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-deadline.changed:
		case <-timer.C:
		}
		now := time.Now().UTC()
		if deadline.cancelIfExpired(now, cancel) {
			return
		}
		resetTimer(timer, time.Until(deadline.current()))
	}
}

func newLeaseDeadline(expiresAt time.Time) *leaseDeadline {
	return &leaseDeadline{expiresAt: expiresAt, changed: make(chan struct{}, 1)}
}

func (deadline *leaseDeadline) current() time.Time {
	deadline.mu.Lock()
	defer deadline.mu.Unlock()
	return deadline.expiresAt
}

func (deadline *leaseDeadline) beginRenewal() *renewalAttempt {
	deadline.mu.Lock()
	defer deadline.mu.Unlock()
	attempt := &renewalAttempt{ready: make(chan struct{})}
	deadline.attempt = attempt
	return attempt
}

func (attempt *renewalAttempt) complete(renewed bool, err error, completedAt, expiresAt time.Time) {
	attempt.result = renewalResult{
		renewed:     renewed,
		err:         err,
		completedAt: completedAt,
		expiresAt:   expiresAt,
	}
	close(attempt.ready)
}

func (deadline *leaseDeadline) finishRenewal(attempt *renewalAttempt) bool {
	deadline.mu.Lock()
	defer deadline.mu.Unlock()
	deadline.applyRenewalLocked(attempt)
	return attempt.accepted
}

func (deadline *leaseDeadline) applyRenewalLocked(attempt *renewalAttempt) {
	if deadline.attempt != attempt {
		return
	}
	select {
	case <-attempt.ready:
		deadline.attempt = nil
	default:
		return
	}
	result := attempt.result
	if deadline.expired || result.err != nil || !result.renewed || !result.completedAt.Before(deadline.expiresAt) {
		return
	}
	deadline.expiresAt = result.expiresAt
	attempt.accepted = true
	select {
	case deadline.changed <- struct{}{}:
	default:
	}
}

func (deadline *leaseDeadline) cancelIfExpired(now time.Time, cancel context.CancelCauseFunc) bool {
	deadline.mu.Lock()
	defer deadline.mu.Unlock()
	if deadline.attempt != nil {
		deadline.applyRenewalLocked(deadline.attempt)
	}
	if now.Before(deadline.expiresAt) {
		return false
	}
	deadline.expired = true
	cancel(errors.New("变更租约已到期"))
	return true
}

func resetTimer(timer *time.Timer, delay time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(max(delay, 0))
}

func minTime(left, right time.Time) time.Time {
	if left.Before(right) {
		return left
	}
	return right
}

func normalizeResources(resources []string) ([]string, error) {
	unique := make(map[string]struct{}, len(resources))
	for _, resource := range resources {
		resource = strings.TrimSpace(resource)
		if resource == "" {
			return nil, errors.New("变更资源不能为空")
		}
		unique[resource] = struct{}{}
	}
	if len(unique) == 0 {
		return nil, errors.New("至少需要一个变更资源")
	}
	result := make([]string, 0, len(unique))
	for resource := range unique {
		result = append(result, resource)
	}
	sort.Strings(result)
	return result, nil
}

func randomOwnerID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func (locks *localLockSet) acquire(ctx context.Context, resources []string) (func(), error) {
	releases := make([]func(), 0, len(resources))
	for _, resource := range resources {
		release, err := locks.acquireOne(ctx, resource)
		if err != nil {
			for index := len(releases) - 1; index >= 0; index-- {
				releases[index]()
			}
			return nil, err
		}
		releases = append(releases, release)
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			for index := len(releases) - 1; index >= 0; index-- {
				releases[index]()
			}
		})
	}, nil
}

func (locks *localLockSet) acquireOne(ctx context.Context, resource string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	locks.mu.Lock()
	if locks.entries == nil {
		locks.entries = make(map[string]*localLock)
	}
	entry := locks.entries[resource]
	if entry == nil {
		entry = &localLock{ready: make(chan struct{}, 1)}
		entry.ready <- struct{}{}
		locks.entries[resource] = entry
	}
	entry.refs++
	locks.mu.Unlock()
	select {
	case <-ctx.Done():
		locks.releaseReference(resource, entry)
		return nil, ctx.Err()
	case <-entry.ready:
		if err := ctx.Err(); err != nil {
			entry.ready <- struct{}{}
			locks.releaseReference(resource, entry)
			return nil, err
		}
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			entry.ready <- struct{}{}
			locks.releaseReference(resource, entry)
		})
	}, nil
}

func (locks *localLockSet) releaseReference(resource string, entry *localLock) {
	locks.mu.Lock()
	defer locks.mu.Unlock()
	entry.refs--
	if entry.refs == 0 && locks.entries[resource] == entry {
		delete(locks.entries, resource)
	}
}
