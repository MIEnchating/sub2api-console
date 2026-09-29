package modelcheck

import (
	"context"
	"sync/atomic"
	"time"
)

// firstOutputTimeout starts only after an animation has acquired its
// concurrency slot. Once output begins, the stream is allowed to finish; the
// task-level timeout remains the upper bound for a stuck or runaway task.
func firstOutputTimeout(parent context.Context, seconds int) (context.Context, context.CancelFunc, func() bool, func()) {
	ctx, cancel := context.WithCancel(parent)
	var timedOut atomic.Bool
	timer := time.AfterFunc(time.Duration(seconds)*time.Second, func() {
		timedOut.Store(true)
		cancel()
	})
	var marked atomic.Bool
	markFirstOutput := func() {
		if marked.CompareAndSwap(false, true) {
			timer.Stop()
		}
	}
	stop := func() {
		markFirstOutput()
		cancel()
	}
	return ctx, stop, timedOut.Load, markFirstOutput
}
