package accountops

import (
	"context"
	"sync"
)

type accountOperationLocks struct {
	mu      sync.Mutex
	entries map[string]*accountOperationLock
}

type accountOperationLock struct {
	ready chan struct{}
	refs  int
}

func (locks *accountOperationLocks) acquire(ctx context.Context, accountID string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	locks.mu.Lock()
	if locks.entries == nil {
		locks.entries = make(map[string]*accountOperationLock)
	}
	entry := locks.entries[accountID]
	if entry == nil {
		entry = &accountOperationLock{ready: make(chan struct{}, 1)}
		entry.ready <- struct{}{}
		locks.entries[accountID] = entry
	}
	entry.refs++
	locks.mu.Unlock()

	select {
	case <-ctx.Done():
		locks.releaseReference(accountID, entry)
		return nil, ctx.Err()
	case <-entry.ready:
		if err := ctx.Err(); err != nil {
			entry.ready <- struct{}{}
			locks.releaseReference(accountID, entry)
			return nil, err
		}
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			entry.ready <- struct{}{}
			locks.releaseReference(accountID, entry)
		})
	}, nil
}

func (locks *accountOperationLocks) releaseReference(accountID string, entry *accountOperationLock) {
	locks.mu.Lock()
	defer locks.mu.Unlock()
	entry.refs--
	if entry.refs == 0 && locks.entries[accountID] == entry {
		delete(locks.entries, accountID)
	}
}
