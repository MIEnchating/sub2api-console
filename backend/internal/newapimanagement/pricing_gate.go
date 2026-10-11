package newapimanagement

import (
	"context"
	"sync"
)

// pricingReadGate serializes cache refreshes without retaining canceled readers.
// Its zero value is ready to use, including services constructed without New.
type pricingReadGate struct {
	once sync.Once
	held chan struct{}
}

func (g *pricingReadGate) Lock(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	g.once.Do(func() { g.held = make(chan struct{}, 1) })
	select {
	case g.held <- struct{}{}:
		// Cancellation and release may become ready together. Do not start a
		// refresh for a reader that was canceled while waiting.
		if err := ctx.Err(); err != nil {
			g.Unlock()
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (g *pricingReadGate) Unlock() {
	<-g.held
}
