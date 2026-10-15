package modelrequest

import (
	"context"
	"sync"
	"time"
)

type budgetKey struct{}
type executionBudget struct {
	context.Context
	mu       sync.Mutex
	timer    *time.Timer
	deadline time.Time
	duration time.Duration
	stream   bool
	cancel   context.CancelCauseFunc
}

// Start establishes one execution budget, after scheduling and lease queues.
// Nested protocol senders reuse it so an old outer deadline cannot defeat a
// reset. Every valid streamed response event rearms the idle deadline.
func Start(parent context.Context, duration time.Duration, stream bool, resetTimeout ...time.Duration) (context.Context, func()) {
	if parent.Value(budgetKey{}) != nil {
		return parent, func() {}
	}
	window := duration
	if len(resetTimeout) > 0 && resetTimeout[0] > 0 {
		window = resetTimeout[0]
	}
	ctx, cancel := context.WithCancelCause(parent)
	b := &executionBudget{Context: ctx, deadline: time.Now().Add(duration), duration: window, stream: stream, cancel: cancel}
	b.timer = time.AfterFunc(duration, b.expire)
	return context.WithValue(b, budgetKey{}, b), func() { b.mu.Lock(); b.timer.Stop(); b.cancel(context.Canceled); b.mu.Unlock() }
}

func (b *executionBudget) expire() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if remaining := time.Until(b.deadline); remaining > 0 {
		b.timer.Reset(remaining)
		return
	}
	b.cancel(context.DeadlineExceeded)
}

// ExecutionDeadline returns the current execution budget for preparation and
// diagnostics. Context.Deadline itself remains the stable parent-task deadline:
// Go contexts require successive Deadline calls to return the same value.
func ExecutionDeadline(ctx context.Context) (time.Time, bool) {
	b, ok := ctx.Value(budgetKey{}).(*executionBudget)
	if !ok {
		return ctx.Deadline()
	}
	b.mu.Lock()
	deadline := b.deadline
	b.mu.Unlock()
	if parent, ok := ctx.Deadline(); ok && parent.Before(deadline) {
		return parent, true
	}
	return deadline, true
}

func (b *executionBudget) Err() error {
	if b.Context.Err() == nil {
		return nil
	}
	if context.Cause(b.Context) == context.DeadlineExceeded {
		return context.DeadlineExceeded
	}
	return b.Context.Err()
}

func receivedEvent(ctx context.Context) {
	b, ok := ctx.Value(budgetKey{}).(*executionBudget)
	if !ok {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.stream || b.Context.Err() != nil || !time.Now().Before(b.deadline) {
		return
	}
	b.deadline = time.Now().Add(b.duration)
	b.timer.Reset(b.duration)
}
