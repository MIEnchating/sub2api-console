package modelcheck

import "context"

type managedDetectionSlotsKey struct{}

func (s *Service) reserveAnimationSlot(ctx context.Context) (func(), error) {
	managed, _ := ctx.Value(managedDetectionSlotsKey{}).(bool)
	releaseStandalone := func() {}
	if !managed {
		select {
		case s.animation.standaloneSlots <- struct{}{}:
			releaseStandalone = func() { <-s.animation.standaloneSlots }
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	select {
	case s.animation.slots <- struct{}{}:
		return func() { <-s.animation.slots; releaseStandalone() }, nil
	case <-ctx.Done():
		releaseStandalone()
		return nil, ctx.Err()
	}
}
