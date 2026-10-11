package api

import "context"

func cancelStoredTask(ctx context.Context, tasks TaskRepository, id string) (bool, error) {
	if store, ok := tasks.(interface {
		CancelTask(context.Context, string) (bool, error)
	}); ok {
		return store.CancelTask(ctx, id)
	}
	if store, ok := tasks.(interface {
		CancelRecovery(context.Context, string) (bool, error)
	}); ok {
		return store.CancelRecovery(ctx, id)
	}
	return false, nil
}
