package targetguard

import (
	"context"
	"errors"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
)

var ErrChanged = errors.New("管理目标在操作排队后已变化，请重新确认并提交")

type Store interface {
	TargetSettings(context.Context) (configstore.TargetSettings, error)
}

type expectedKey struct{}
type pinnedKey struct{}

// Capture records the management target identity that the caller validated.
// Existing expectations are preserved across nested workflows.
func Capture(ctx context.Context, store Store) (context.Context, error) {
	if _, found := expected(ctx); found {
		return ctx, nil
	}
	target, err := Settings(ctx, store)
	if err != nil {
		return nil, err
	}
	return Expect(ctx, target), nil
}

func Expect(ctx context.Context, target configstore.TargetSettings) context.Context {
	return context.WithValue(ctx, expectedKey{}, target)
}

// Expected returns the target identity already carried by an upstream workflow,
// or the current settings when a new workflow is being queued.
func Expected(ctx context.Context, store Store) (configstore.TargetSettings, error) {
	if target, found := expected(ctx); found {
		return target, nil
	}
	return Settings(ctx, store)
}

// Acquire adds the management target to an operation's atomic mutation set.
// Bind must be called after operation-specific preconditions and before remote access.
func Acquire(ctx context.Context, repository any, resources ...string) (context.Context, func() error, error) {
	all := make([]string, 0, len(resources)+1)
	all = append(all, mutationguard.ManagementTarget())
	all = append(all, resources...)
	return mutationguard.Acquire(ctx, repository, all...)
}

// Bind verifies a queued target expectation while its mutation lease is held,
// then pins the verified settings so every client in the operation uses one target.
func Bind(ctx context.Context, store Store) (context.Context, error) {
	return Pin(ctx, store)
}

// Pin validates the queued target identity and keeps that immutable snapshot
// without acquiring the global target mutation lease. Long-running workflows
// use it when they must not block unrelated manual operations.
func Pin(ctx context.Context, store Store) (context.Context, error) {
	current, err := store.TargetSettings(ctx)
	if err != nil {
		return nil, err
	}
	if wanted, found := expected(ctx); found && !sameIdentity(wanted, current) {
		return nil, ErrChanged
	}
	return context.WithValue(ctx, pinnedKey{}, current), nil
}

func Settings(ctx context.Context, store Store) (configstore.TargetSettings, error) {
	if target, found := ctx.Value(pinnedKey{}).(configstore.TargetSettings); found {
		return target, nil
	}
	return store.TargetSettings(ctx)
}

func expected(ctx context.Context) (configstore.TargetSettings, bool) {
	target, found := ctx.Value(expectedKey{}).(configstore.TargetSettings)
	return target, found
}

func sameIdentity(left, right configstore.TargetSettings) bool {
	return normalizeBaseURL(left.BaseURL) == normalizeBaseURL(right.BaseURL) &&
		strings.TrimSpace(left.AdminKey) == strings.TrimSpace(right.AdminKey)
}

func normalizeBaseURL(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), "/")
}
