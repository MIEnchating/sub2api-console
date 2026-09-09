package evidence

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
)

// Live shares short polling between viewers. It never runs probes or routing writes.
type Live struct {
	poll     func(context.Context, string) error
	runner   taskrunner.Runner
	mu       sync.Mutex
	accounts map[string]*liveAccount
	slots    chan struct{}
}
type LiveUpdate struct {
	AccountID string
	Failed    bool
}
type liveAccount struct {
	cancel      context.CancelFunc
	subscribers map[chan LiveUpdate]struct{}
}

func NewLive(poll func(context.Context, string) error, runner taskrunner.Runner) *Live {
	return &Live{poll: poll, runner: runner, accounts: map[string]*liveAccount{}, slots: make(chan struct{}, 4)}
}
func (s *Live) Subscribe(ids []string) (<-chan LiveUpdate, func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	newIDs := map[string]struct{}{}
	for _, id := range ids {
		if s.accounts[id] == nil {
			newIDs[id] = struct{}{}
		}
	}
	if len(s.accounts)+len(newIDs) > 500 {
		return nil, nil, errors.New("实时采集账号数量已达上限")
	}
	updates := make(chan LiveUpdate, len(ids)*2)
	remove := func() {
		for _, id := range ids {
			account := s.accounts[id]
			if account == nil {
				continue
			}
			delete(account.subscribers, updates)
			if len(account.subscribers) == 0 {
				account.cancel()
				delete(s.accounts, id)
			}
		}
	}
	for _, id := range ids {
		account := s.accounts[id]
		if account == nil {
			ctx, cancel := context.WithCancel(context.Background())
			account = &liveAccount{cancel: cancel, subscribers: map[chan LiveUpdate]struct{}{updates: {}}}
			s.accounts[id] = account
			if err := s.runner.Go(func(parent context.Context) {
				stop := context.AfterFunc(parent, cancel)
				defer stop()
				defer cancel()
				s.run(ctx, id, account)
			}); err != nil {
				remove()
				return nil, nil, err
			}
		} else {
			account.subscribers[updates] = struct{}{}
		}
	}
	var once sync.Once
	return updates, func() { once.Do(func() { s.mu.Lock(); defer s.mu.Unlock(); remove() }) }, nil
}
func (s *Live) run(ctx context.Context, id string, account *liveAccount) {
	for ctx.Err() == nil {
		select {
		case s.slots <- struct{}{}:
		case <-ctx.Done():
			return
		}
		pollCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err := s.poll(pollCtx, id)
		cancel()
		<-s.slots
		if ctx.Err() != nil {
			return
		}
		s.mu.Lock()
		for subscriber := range account.subscribers {
			select {
			case subscriber <- LiveUpdate{AccountID: id, Failed: err != nil}:
			default:
			}
		}
		s.mu.Unlock()
		delay := 5 * time.Second
		if err != nil {
			delay = 30 * time.Second
		}
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return
		}
	}
}
