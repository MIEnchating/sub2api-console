package routingwrite

import (
	"sync"
	"sync/atomic"
)

// Preparation must not wait for the batch outcome: writes begin only after
// every account has either queued its desired fields or explicitly skipped.
// Keep continuations, rather than parked goroutines, until that barrier closes.
func runAccountPreparations(count, maximum int, prepare func(int) (AccountResult, func() AccountResult)) []AccountResult {
	results := make([]AccountResult, count)
	finish := make([]func() AccountResult, count)
	runBounded(count, maximum, func(index int) {
		results[index], finish[index] = prepare(index)
	})
	runBounded(count, maximum, func(index int) {
		if finish[index] != nil {
			results[index] = finish[index]()
			finish[index] = nil
		}
	})
	return results
}

func runBounded(count, maximum int, work func(int)) {
	maximum = max(1, min(maximum, count))
	var wait sync.WaitGroup
	var next atomic.Int64
	for range min(maximum, count) {
		wait.Go(func() {
			for {
				index := int(next.Add(1) - 1)
				if index >= count {
					return
				}
				work(index)
			}
		})
	}
	wait.Wait()
}
