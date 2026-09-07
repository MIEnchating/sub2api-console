package taskrunner

import "context"

// Feed sends work until every item is dispatched or the task context is cancelled.
// It always closes jobs so workers can terminate without a separate cancellation path.
func Feed[T any](ctx context.Context, jobs chan<- T, items []T) error {
	defer close(jobs)
	if ctx == nil {
		ctx = context.Background()
	}
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return err
		}
		select {
		case jobs <- item:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// FeedIndices is the allocation-free variant for index-based worker pools.
func FeedIndices(ctx context.Context, jobs chan<- int, total int) error {
	defer close(jobs)
	if ctx == nil {
		ctx = context.Background()
	}
	for index := 0; index < total; index++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		select {
		case jobs <- index:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
