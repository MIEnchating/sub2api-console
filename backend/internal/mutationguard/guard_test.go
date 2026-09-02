package mutationguard

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

type renewalStoreStub struct {
	mu           sync.Mutex
	calls        int
	acquisitions int
	releases     int
	notify       chan struct{}
}

type blockingRenewalStoreStub struct {
	started chan time.Time
	unblock chan struct{}
}

type cancellableRenewalStoreStub struct {
	started       chan struct{}
	cancelled     chan struct{}
	allowReturn   chan struct{}
	returned      chan struct{}
	releaseCalled chan bool
}

type manualClock struct {
	mu  sync.Mutex
	now time.Time
}

type slowAcquireStoreStub struct {
	mu       sync.Mutex
	clock    *manualClock
	acquires int
	releases int
}

type changingResourceStoreStub struct {
	mu           sync.Mutex
	identity     string
	acquisitions [][]string
	releases     [][]string
}

type overlappingResolverStub struct {
	overlap string
}

type mappedResolverStub map[string]string

type mappedLeaseStoreStub struct {
	mu           sync.Mutex
	resolved     map[string]string
	leases       map[string]string
	acquisitions int
	releases     int
}

type changingNestedResolverStub struct {
	mu      sync.Mutex
	overlap string
}

type nestedLeaseResolverStub struct {
	mu           sync.Mutex
	overlap      string
	acquisitions int
	releases     int
}

func (resolver overlappingResolverStub) ResolveMutationResources(_ context.Context, resources []string) ([]string, error) {
	return append(append([]string{}, resources...), resolver.overlap), nil
}

func (resolver mappedResolverStub) ResolveMutationResources(_ context.Context, resources []string) ([]string, error) {
	resolved := make([]string, 0, len(resources))
	for _, resource := range resources {
		resolved = append(resolved, resolver[resource])
	}
	return resolved, nil
}

func (store *mappedLeaseStoreStub) ResolveMutationResources(_ context.Context, resources []string) ([]string, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	resolved := make([]string, 0, len(resources))
	for _, resource := range resources {
		resolved = append(resolved, store.resolved[resource])
	}
	return resolved, nil
}

func (store *mappedLeaseStoreStub) AcquireMutationLease(
	_ context.Context,
	ownerID string,
	resources []string,
	_ time.Time,
	_ time.Duration,
) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.acquisitions++
	if store.leases == nil {
		store.leases = map[string]string{}
	}
	for _, resource := range resources {
		if owner := store.leases[resource]; owner != "" && owner != ownerID {
			return false, nil
		}
	}
	for _, resource := range resources {
		store.leases[resource] = ownerID
	}
	return true, nil
}

func (*mappedLeaseStoreStub) RenewMutationLease(
	context.Context,
	string,
	[]string,
	time.Time,
	time.Duration,
) (bool, error) {
	return true, nil
}

func (store *mappedLeaseStoreStub) ReleaseMutationLease(_ context.Context, ownerID string, resources []string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.releases++
	for _, resource := range resources {
		if store.leases[resource] == ownerID {
			delete(store.leases, resource)
		}
	}
	return nil
}

func (store *mappedLeaseStoreStub) counts() (int, int) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.acquisitions, store.releases
}

func (resolver *changingNestedResolverStub) ResolveMutationResources(_ context.Context, resources []string) ([]string, error) {
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	return append(append([]string{}, resources...), resolver.overlap), nil
}

func (resolver *changingNestedResolverStub) setOverlap(resource string) {
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	resolver.overlap = resource
}

func (store *nestedLeaseResolverStub) ResolveMutationResources(_ context.Context, resources []string) ([]string, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return append(append([]string{}, resources...), store.overlap), nil
}

func (store *nestedLeaseResolverStub) AcquireMutationLease(context.Context, string, []string, time.Time, time.Duration) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.acquisitions++
	return true, nil
}

func (*nestedLeaseResolverStub) RenewMutationLease(context.Context, string, []string, time.Time, time.Duration) (bool, error) {
	return true, nil
}

func (store *nestedLeaseResolverStub) ReleaseMutationLease(context.Context, string, []string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.releases++
	return nil
}

func (store *nestedLeaseResolverStub) setOverlap(resource string) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.overlap = resource
}

func (store *nestedLeaseResolverStub) counts() (int, int) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.acquisitions, store.releases
}

func (store *changingResourceStoreStub) ResolveMutationResources(_ context.Context, resources []string) ([]string, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	resolved := append([]string{}, resources...)
	resolved = append(resolved, "upstream-identity/"+store.identity)
	return resolved, nil
}

func (store *changingResourceStoreStub) AcquireMutationLease(_ context.Context, _ string, resources []string, _ time.Time, _ time.Duration) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.acquisitions = append(store.acquisitions, append([]string{}, resources...))
	if len(store.acquisitions) == 1 {
		store.identity = "new"
	}
	return true, nil
}

func (*changingResourceStoreStub) RenewMutationLease(context.Context, string, []string, time.Time, time.Duration) (bool, error) {
	return true, nil
}

func (store *changingResourceStoreStub) ReleaseMutationLease(_ context.Context, _ string, resources []string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.releases = append(store.releases, append([]string{}, resources...))
	return nil
}

func (store *changingResourceStoreStub) calls() ([][]string, [][]string) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return append([][]string{}, store.acquisitions...), append([][]string{}, store.releases...)
}

func (*blockingRenewalStoreStub) AcquireMutationLease(context.Context, string, []string, time.Time, time.Duration) (bool, error) {
	return true, nil
}

func (store *blockingRenewalStoreStub) RenewMutationLease(_ context.Context, _ string, _ []string, now time.Time, _ time.Duration) (bool, error) {
	store.started <- now
	<-store.unblock
	return true, nil
}

func (*blockingRenewalStoreStub) ReleaseMutationLease(context.Context, string, []string) error {
	return nil
}

func (*cancellableRenewalStoreStub) AcquireMutationLease(context.Context, string, []string, time.Time, time.Duration) (bool, error) {
	return true, nil
}

func (store *cancellableRenewalStoreStub) RenewMutationLease(ctx context.Context, _ string, _ []string, _ time.Time, _ time.Duration) (bool, error) {
	close(store.started)
	<-ctx.Done()
	close(store.cancelled)
	<-store.allowReturn
	close(store.returned)
	return false, ctx.Err()
}

func (store *cancellableRenewalStoreStub) ReleaseMutationLease(context.Context, string, []string) error {
	renewalExited := false
	select {
	case <-store.returned:
		renewalExited = true
	default:
	}
	store.releaseCalled <- renewalExited
	return nil
}

func (clock *manualClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now
}

func (clock *manualClock) Advance(delta time.Duration) {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	clock.now = clock.now.Add(delta)
}

func (store *slowAcquireStoreStub) AcquireMutationLease(context.Context, string, []string, time.Time, time.Duration) (bool, error) {
	store.mu.Lock()
	store.acquires++
	first := store.acquires == 1
	store.mu.Unlock()
	if first {
		store.clock.Advance(leaseTTL - renewalInterval - leaseCallTimeout + time.Nanosecond)
	}
	return true, nil
}

func (*slowAcquireStoreStub) RenewMutationLease(context.Context, string, []string, time.Time, time.Duration) (bool, error) {
	return true, nil
}

func (store *slowAcquireStoreStub) ReleaseMutationLease(context.Context, string, []string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.releases++
	return nil
}

func (store *slowAcquireStoreStub) counts() (int, int) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.acquires, store.releases
}

func (store *renewalStoreStub) AcquireMutationLease(context.Context, string, []string, time.Time, time.Duration) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.acquisitions++
	return true, nil
}

func (store *renewalStoreStub) RenewMutationLease(context.Context, string, []string, time.Time, time.Duration) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.calls++
	select {
	case store.notify <- struct{}{}:
	default:
	}
	if store.calls == 1 {
		return false, errors.New("temporary busy")
	}
	return true, nil
}

func (store *renewalStoreStub) ReleaseMutationLease(context.Context, string, []string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.releases++
	return nil
}

func (store *renewalStoreStub) leaseCalls() (int, int) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.acquisitions, store.releases
}

func TestFallbackGuardSerializesSharedResourcesAcrossRepositories(t *testing.T) {
	resource := Account("41")
	firstCtx, releaseFirst, err := Acquire(context.Background(), struct{}{}, resource)
	if err != nil || firstCtx.Err() != nil {
		t.Fatalf("first acquire failed: %v", err)
	}
	waiting := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, release, err := Acquire(ctx, struct{ value int }{value: 1}, resource)
		if err == nil {
			_ = release()
		}
		waiting <- err
	}()
	registrationDeadline := time.NewTimer(time.Second)
	defer registrationDeadline.Stop()
	for {
		fallbackLocks.mu.Lock()
		entry := fallbackLocks.entries[resource]
		registered := entry != nil && entry.refs == 2
		fallbackLocks.mu.Unlock()
		if registered {
			break
		}
		select {
		case err := <-waiting:
			t.Fatalf("second acquire did not wait: %v", err)
		case <-registrationDeadline.C:
			t.Fatal("second acquire did not register as a waiter")
		default:
			runtime.Gosched()
		}
	}
	select {
	case err := <-waiting:
		t.Fatalf("registered waiter completed before release: %v", err)
	default:
	}
	if err := releaseFirst(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-waiting:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("second acquire stayed blocked after release")
	}
}

func TestFallbackGuardCancellationDoesNotLeakEarlierResources(t *testing.T) {
	_, releaseHeld, err := Acquire(context.Background(), struct{}{}, Account("42"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if _, _, err := Acquire(ctx, struct{}{}, Account("41"), Account("42")); err == nil {
		t.Fatal("cancelled multi-resource acquire succeeded")
	}
	_, releaseAvailable, err := Acquire(context.Background(), struct{}{}, Account("41"))
	if err != nil {
		t.Fatalf("earlier resource leaked after cancellation: %v", err)
	}
	_ = releaseAvailable()
	_ = releaseHeld()
}

func TestNestedAcquireRejectsPartialResourceOverlap(t *testing.T) {
	first, second, third := Account("41"), Account("42"), Account("43")
	guarded, releaseHeld, err := Acquire(context.Background(), struct{}{}, first, second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = releaseHeld() }()

	waitCtx, cancel := context.WithTimeout(guarded, time.Second)
	defer cancel()
	_, _, err = Acquire(waitCtx, struct{}{}, second, third)
	if !errors.Is(err, ErrPartialResourceOverlap) {
		t.Fatalf("partial-overlap acquire error = %v", err)
	}
	if waitCtx.Err() != nil {
		t.Fatalf("partial-overlap acquire waited for context cancellation: %v", waitCtx.Err())
	}
	assertFallbackResources(t, map[string]int{first: 1, second: 1}, third)
}

func TestNestedAcquireChecksResolverExpansionForPartialOverlap(t *testing.T) {
	first, second, third := Account("51"), Account("52"), Account("53")
	guarded, releaseHeld, err := Acquire(context.Background(), struct{}{}, first, second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = releaseHeld() }()

	_, _, err = Acquire(guarded, overlappingResolverStub{overlap: second}, third)
	if !errors.Is(err, ErrPartialResourceOverlap) {
		t.Fatalf("resolver-expanded partial-overlap acquire error = %v", err)
	}
	assertFallbackResources(t, map[string]int{first: 1, second: 1}, third)
}

func TestNestedAcquireRechecksResolverWhenRequestedResourcesAreHeld(t *testing.T) {
	requested, firstExpansion, changedExpansion := Account("54"), Account("55"), Account("56")
	resolver := &changingNestedResolverStub{overlap: firstExpansion}
	guarded, releaseHeld, err := Acquire(context.Background(), resolver, requested)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = releaseHeld() }()
	resolver.setOverlap(changedExpansion)

	_, _, err = Acquire(guarded, resolver, requested)
	if !errors.Is(err, ErrPartialResourceOverlap) {
		t.Fatalf("changed resolver expansion acquire error = %v", err)
	}
	assertFallbackResources(t, map[string]int{requested: 1, firstExpansion: 1}, changedExpansion)
}

func TestNestedLeaseAcquireRejectsChangedResolverWithoutTouchingParentLease(t *testing.T) {
	requested, firstExpansion, changedExpansion := Account("57"), Account("58"), Account("59")
	store := &nestedLeaseResolverStub{overlap: firstExpansion}
	guarded, releaseHeld, err := Acquire(context.Background(), store, requested)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = releaseHeld() }()
	store.setOverlap(changedExpansion)

	_, _, err = Acquire(guarded, store, requested)
	if !errors.Is(err, ErrPartialResourceOverlap) {
		t.Fatalf("changed resolver lease acquire error = %v", err)
	}
	acquisitions, releases := store.counts()
	if acquisitions != 1 || releases != 0 {
		t.Fatalf("nested rejection touched persisted lease: acquisitions=%d releases=%d", acquisitions, releases)
	}
}

func TestNestedAcquireReusesFullyHeldResources(t *testing.T) {
	first, second := Account("61"), Account("62")
	guarded, releaseHeld, err := Acquire(context.Background(), struct{}{}, first, second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = releaseHeld() }()

	nested, releaseNested, err := Acquire(guarded, struct{}{}, second, first)
	if err != nil {
		t.Fatal(err)
	}
	if nested != guarded {
		t.Fatal("fully held nested acquire replaced the guarded context")
	}
	if err := releaseNested(); err != nil {
		t.Fatal(err)
	}
	assertFallbackResources(t, map[string]int{first: 1, second: 1})
}

func TestNestedFallbackAcquireAllowsForwardDisjointResources(t *testing.T) {
	first, second := Account("71"), Account("72")
	guarded, releaseFirst, err := Acquire(context.Background(), struct{}{}, first)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = releaseFirst() }()

	_, releaseSecond, err := Acquire(guarded, struct{}{}, second)
	if err != nil {
		t.Fatal(err)
	}
	assertFallbackResources(t, map[string]int{first: 1, second: 1})
	if err := releaseSecond(); err != nil {
		t.Fatal(err)
	}
	assertFallbackResources(t, map[string]int{first: 1}, second)
}

func TestNestedFallbackAcquireRejectsReverseDisjointResourceOrder(t *testing.T) {
	first, second := Account("73"), Account("74")
	guarded, releaseSecond, err := Acquire(context.Background(), struct{}{}, second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = releaseSecond() }()

	waitCtx, cancel := context.WithTimeout(guarded, time.Second)
	defer cancel()
	_, _, err = Acquire(waitCtx, struct{}{}, first)
	if !errors.Is(err, ErrResourceOrderViolation) {
		t.Fatalf("reverse disjoint acquire error = %v", err)
	}
	if waitCtx.Err() != nil {
		t.Fatalf("reverse disjoint acquire waited for context cancellation: %v", waitCtx.Err())
	}
	assertFallbackResources(t, map[string]int{second: 1}, first)
}

func TestNestedLeaseAcquireAllowsForwardDisjointResources(t *testing.T) {
	first, second := Account("75"), Account("76")
	store := &renewalStoreStub{}
	guarded, releaseFirst, err := Acquire(context.Background(), store, first)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = releaseFirst() }()

	_, releaseSecond, err := Acquire(guarded, store, second)
	if err != nil {
		t.Fatal(err)
	}
	if acquisitions, releases := store.leaseCalls(); acquisitions != 2 || releases != 0 {
		t.Fatalf("forward disjoint lease calls = acquire:%d release:%d", acquisitions, releases)
	}
	if err := releaseSecond(); err != nil {
		t.Fatal(err)
	}
	if acquisitions, releases := store.leaseCalls(); acquisitions != 2 || releases != 1 {
		t.Fatalf("released nested lease calls = acquire:%d release:%d", acquisitions, releases)
	}
}

func TestNestedLeaseAcquireRejectsReverseDisjointResourceOrder(t *testing.T) {
	first, second := Account("77"), Account("78")
	store := &renewalStoreStub{}
	guarded, releaseSecond, err := Acquire(context.Background(), store, second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = releaseSecond() }()

	waitCtx, cancel := context.WithTimeout(guarded, time.Second)
	defer cancel()
	_, _, err = Acquire(waitCtx, store, first)
	if !errors.Is(err, ErrResourceOrderViolation) {
		t.Fatalf("reverse disjoint lease acquire error = %v", err)
	}
	if waitCtx.Err() != nil {
		t.Fatalf("reverse disjoint lease acquire waited for context cancellation: %v", waitCtx.Err())
	}
	if acquisitions, releases := store.leaseCalls(); acquisitions != 1 || releases != 0 {
		t.Fatalf("rejected reverse lease touched store: acquire:%d release:%d", acquisitions, releases)
	}
}

func TestNestedResourceOrderUsesResolvedLocksInsteadOfRequestedAliases(t *testing.T) {
	resolver := mappedResolverStub{
		"logical/z": "lock/a",
		"logical/a": "lock/b",
	}
	guarded, releaseFirst, err := Acquire(context.Background(), resolver, "logical/z")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = releaseFirst() }()
	reentrant, releaseAlias, err := Acquire(guarded, resolver, "logical/z")
	if err != nil {
		t.Fatalf("stable resolver result was not recognized as already locked: %v", err)
	}
	if reentrant != guarded {
		t.Fatal("fully covered requested alias replaced the guarded context")
	}
	if err := releaseAlias(); err != nil {
		t.Fatal(err)
	}

	_, releaseSecond, err := Acquire(guarded, resolver, "logical/a")
	if err != nil {
		t.Fatalf("forward resolved lock order rejected because of logical aliases: %v", err)
	}
	if err := releaseSecond(); err != nil {
		t.Fatal(err)
	}
}

func TestNestedResourceOrderRejectsReverseResolvedLocksDespiteForwardAliases(t *testing.T) {
	resolver := mappedResolverStub{
		"logical/a": "lock/z",
		"logical/z": "lock/a",
	}
	guarded, release, err := Acquire(context.Background(), resolver, "logical/a")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = release() }()

	waitCtx, cancel := context.WithTimeout(guarded, time.Second)
	defer cancel()
	_, _, err = Acquire(waitCtx, resolver, "logical/z")
	if !errors.Is(err, ErrResourceOrderViolation) {
		t.Fatalf("reverse resolved lock order error = %v", err)
	}
	if waitCtx.Err() != nil {
		t.Fatalf("reverse resolved lock order waited for context cancellation: %v", waitCtx.Err())
	}
}

func TestNestedAcquireRejectsDisjointResolverDriftForHeldLogicalRequest(t *testing.T) {
	resolver := mappedResolverStub{"logical/a": "lock/a"}
	guarded, release, err := Acquire(context.Background(), resolver, "logical/a")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = release() }()
	resolver["logical/a"] = "lock/z"

	_, _, err = Acquire(guarded, resolver, "logical/a")
	if !errors.Is(err, ErrPartialResourceOverlap) {
		t.Fatalf("disjoint resolver drift acquire error = %v", err)
	}
	assertFallbackResources(t, map[string]int{"lock/a": 1}, "lock/z")
}

func TestNonResolverLockNameDoesNotPolluteLogicalRequestCoverage(t *testing.T) {
	guarded, releaseFirst, err := Acquire(context.Background(), struct{}{}, "lock/a")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = releaseFirst() }()
	resolver := mappedResolverStub{"lock/a": "lock/z"}

	_, releaseSecond, err := Acquire(guarded, resolver, "lock/a")
	if err != nil {
		t.Fatalf("actual lock name was mistaken for a held logical request: %v", err)
	}
	if err := releaseSecond(); err != nil {
		t.Fatal(err)
	}
}

func TestResolverAliasCannotBypassCompetingFallbackLock(t *testing.T) {
	resolver := mappedResolverStub{
		"alias-a": "lock-z",
		"alias-b": "alias-a",
	}
	guarded, releaseParent, err := Acquire(context.Background(), resolver, "alias-a")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = releaseParent() }()
	_, releaseCompetitor, err := Acquire(context.Background(), struct{}{}, "alias-a")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = releaseCompetitor() }()

	waitCtx, cancel := context.WithTimeout(guarded, time.Second)
	defer cancel()
	_, _, err = Acquire(waitCtx, resolver, "alias-b")
	if !errors.Is(err, ErrResourceOrderViolation) {
		t.Fatalf("resolver alias acquire error = %v", err)
	}
	if waitCtx.Err() != nil {
		t.Fatalf("resolver alias acquire waited for context cancellation: %v", waitCtx.Err())
	}
	assertFallbackResources(t, map[string]int{"alias-a": 1, "lock-z": 1})
}

func TestResolverAliasCannotBypassCompetingLease(t *testing.T) {
	store := &mappedLeaseStoreStub{resolved: map[string]string{
		"alias-a":    "lock-z",
		"alias-b":    "alias-a",
		"competitor": "alias-a",
	}}
	guarded, releaseParent, err := Acquire(context.Background(), store, "alias-a")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = releaseParent() }()
	_, releaseCompetitor, err := Acquire(context.Background(), store, "competitor")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = releaseCompetitor() }()

	waitCtx, cancel := context.WithTimeout(guarded, time.Second)
	defer cancel()
	_, _, err = Acquire(waitCtx, store, "alias-b")
	if !errors.Is(err, ErrResourceOrderViolation) {
		t.Fatalf("resolver alias lease acquire error = %v", err)
	}
	if waitCtx.Err() != nil {
		t.Fatalf("resolver alias lease acquire waited for context cancellation: %v", waitCtx.Err())
	}
	if acquisitions, releases := store.counts(); acquisitions != 2 || releases != 0 {
		t.Fatalf("resolver alias lease touched store: acquire:%d release:%d", acquisitions, releases)
	}
}

func TestReentrantAcquireRejectsCancelledContext(t *testing.T) {
	guarded, release, err := Acquire(context.Background(), struct{}{}, Account("41"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = release() }()
	cancelled, cancel := context.WithCancel(guarded)
	cancel()

	if _, _, err := Acquire(cancelled, struct{}{}, Account("41")); !errors.Is(err, context.Canceled) {
		t.Fatalf("reentrant acquire error = %v, want context cancellation", err)
	}
}

func assertFallbackResources(t *testing.T, held map[string]int, absent ...string) {
	t.Helper()
	fallbackLocks.mu.Lock()
	defer fallbackLocks.mu.Unlock()
	for resource, refs := range held {
		entry := fallbackLocks.entries[resource]
		if entry == nil || entry.refs != refs {
			t.Fatalf("fallback resource %q = %#v, want refs=%d", resource, entry, refs)
		}
	}
	for _, resource := range absent {
		if entry := fallbackLocks.entries[resource]; entry != nil {
			t.Fatalf("rejected acquisition retained resource %q: %#v", resource, entry)
		}
	}
}

func TestUpstreamResourceCanonicalizesURLAliases(t *testing.T) {
	aliases := []string{"api.example", "HTTPS://API.EXAMPLE/", "https://api.example"}
	for _, alias := range aliases {
		if resource := Upstream(alias); resource != "upstream/api.example" {
			t.Fatalf("Upstream(%q) = %q", alias, resource)
		}
		if resource := UpstreamKeyCatalog(alias); resource != "upstream-keys/api.example" {
			t.Fatalf("UpstreamKeyCatalog(%q) = %q", alias, resource)
		}
	}
}

func TestVaultResourceTrimsEntryName(t *testing.T) {
	if resource := Vault("  Primary  "); resource != "vault/Primary" {
		t.Fatalf("Vault resource = %q", resource)
	}
}

func TestRenewToleratesTransientDatabaseErrorBeforeExpiry(t *testing.T) {
	store := &renewalStoreStub{notify: make(chan struct{}, 2)}
	ctx, cancel := context.WithCancelCause(context.Background())
	done := make(chan struct{})
	ttl := 200 * time.Millisecond
	go renew(ctx, cancel, store, "owner", []string{Account("41")}, time.Now().UTC().Add(ttl), ttl, 20*time.Millisecond, done)
	for call := 0; call < 2; call++ {
		select {
		case <-store.notify:
		case <-time.After(time.Second):
			t.Fatal("renewal call did not run")
		}
	}
	if err := context.Cause(ctx); err != nil {
		t.Fatalf("transient renewal failure cancelled lease: %v", err)
	}
	cancel(errors.New("test complete"))
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("renewal did not stop")
	}
}

func TestRenewCancelsGuardAtExpiryWhileRenewalCallIsBlocked(t *testing.T) {
	store := &blockingRenewalStoreStub{started: make(chan time.Time, 1), unblock: make(chan struct{})}
	ctx, cancel := context.WithCancelCause(context.Background())
	done := make(chan struct{})
	ttl := 60 * time.Millisecond
	go renew(ctx, cancel, store, "owner", []string{Account("41")}, time.Now().UTC().Add(ttl), ttl, 10*time.Millisecond, done)
	select {
	case <-store.started:
	case <-time.After(time.Second):
		t.Fatal("renewal call did not start")
	}
	select {
	case <-ctx.Done():
		if cause := context.Cause(ctx); cause == nil || !strings.Contains(cause.Error(), "到期") {
			t.Fatalf("lease cancellation cause = %v", cause)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked renewal kept the guarded context alive past its lease TTL")
	}
	close(store.unblock)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("renewal did not stop after the blocked call returned")
	}
}

func TestReleaseCancelsInFlightRenewalAndWaitsBeforeDeletingLease(t *testing.T) {
	store := &cancellableRenewalStoreStub{
		started:       make(chan struct{}),
		cancelled:     make(chan struct{}),
		allowReturn:   make(chan struct{}),
		returned:      make(chan struct{}),
		releaseCalled: make(chan bool, 1),
	}
	var allowReturnOnce sync.Once
	allowReturn := func() { allowReturnOnce.Do(func() { close(store.allowReturn) }) }
	t.Cleanup(allowReturn)

	guarded, release := maintainLease(
		context.Background(),
		store,
		"owner",
		[]string{Account("41")},
		time.Now().UTC().Add(time.Minute),
		time.Minute,
		time.Millisecond,
	)
	select {
	case <-store.started:
	case <-time.After(time.Second):
		t.Fatal("renewal call did not start")
	}
	released := make(chan error, 1)
	go func() { released <- release() }()
	select {
	case <-store.cancelled:
	case <-time.After(time.Second):
		t.Fatal("release did not cancel the in-flight renewal")
	}
	select {
	case exited := <-store.releaseCalled:
		t.Fatalf("lease deletion ran before renewal returned: renewal_exited=%t", exited)
	case err := <-released:
		t.Fatalf("release returned before renewal exited: %v", err)
	default:
	}

	allowReturn()
	select {
	case err := <-released:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("release did not finish after renewal exited")
	}
	select {
	case renewalExited := <-store.releaseCalled:
		if !renewalExited {
			t.Fatal("lease deletion observed an active renewal")
		}
	case <-time.After(time.Second):
		t.Fatal("lease deletion was not called")
	}
	if cause := context.Cause(guarded); cause == nil || !strings.Contains(cause.Error(), "已释放") {
		t.Fatalf("guarded context cause = %v", cause)
	}
}

func TestSuccessfulRenewalMakesOldWatchdogTickStale(t *testing.T) {
	initialExpiry := time.Unix(1_800_000_000, 0).UTC()
	deadline := newLeaseDeadline(initialExpiry)
	renewedExpiry := initialExpiry.Add(time.Minute)
	attempt := deadline.beginRenewal()
	attempt.complete(true, nil, initialExpiry.Add(-time.Nanosecond), renewedExpiry)
	ctx, cancel := context.WithCancelCause(context.Background())
	if deadline.cancelIfExpired(initialExpiry.Add(time.Nanosecond), cancel) {
		t.Fatal("old watchdog tick expired the completed renewal before the renewal goroutine finalized it")
	}
	if err := context.Cause(ctx); err != nil {
		t.Fatalf("old watchdog tick cancelled the renewed lease: %v", err)
	}
	if !deadline.finishRenewal(attempt) {
		t.Fatal("watchdog did not preserve the successful renewal result")
	}
	if !deadline.cancelIfExpired(renewedExpiry, cancel) {
		t.Fatal("current watchdog deadline did not expire the lease")
	}
}

func TestAcquireRetriesLeaseWithoutFullFirstRenewalMargin(t *testing.T) {
	clock := &manualClock{now: time.Now().UTC()}
	store := &slowAcquireStoreStub{clock: clock}
	_, release, err := acquire(context.Background(), store, clock.Now, Account("41"))
	if err != nil {
		t.Fatal(err)
	}
	acquires, releases := store.counts()
	if acquires != 2 || releases != 1 {
		_ = release()
		t.Fatalf("before return calls = acquire:%d release:%d, want acquire:2 release:1", acquires, releases)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	acquires, releases = store.counts()
	if acquires != 2 || releases != 2 {
		t.Fatalf("after release calls = acquire:%d release:%d, want acquire:2 release:2", acquires, releases)
	}
}

func TestAcquireReleasesAndRetriesWhenResolvedResourcesChangeWhileWaiting(t *testing.T) {
	store := &changingResourceStoreStub{identity: "old"}
	_, release, err := Acquire(context.Background(), store, Upstream("api.example"))
	if err != nil {
		t.Fatal(err)
	}
	acquisitions, releases := store.calls()
	if len(acquisitions) != 2 {
		_ = release()
		t.Fatalf("acquisitions = %#v, want one stale attempt and one retry", acquisitions)
	}
	if strings.Join(acquisitions[0], "\x00") != "upstream-identity/old\x00upstream/api.example" {
		_ = release()
		t.Fatalf("first acquisition = %#v", acquisitions[0])
	}
	if strings.Join(acquisitions[1], "\x00") != "upstream-identity/new\x00upstream/api.example" {
		_ = release()
		t.Fatalf("second acquisition = %#v", acquisitions[1])
	}
	if len(releases) != 1 || strings.Join(releases[0], "\x00") != strings.Join(acquisitions[0], "\x00") {
		_ = release()
		t.Fatalf("stale releases = %#v, acquisitions = %#v", releases, acquisitions)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	_, releases = store.calls()
	if len(releases) != 2 || strings.Join(releases[1], "\x00") != strings.Join(acquisitions[1], "\x00") {
		t.Fatalf("final releases = %#v, acquisitions = %#v", releases, acquisitions)
	}
}

func TestReleaseDoesNotWaitForLeaseExpiry(t *testing.T) {
	store := &renewalStoreStub{notify: make(chan struct{}, 1)}
	_, release, err := Acquire(context.Background(), store, Account("41"))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- release() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("release waited for the lease TTL")
	}
}
