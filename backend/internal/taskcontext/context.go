package taskcontext

import (
	"context"
	"strings"
)

type taskIDKey struct{}

// WithID associates a background task with the context used by its work.
func WithID(ctx context.Context, id string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, taskIDKey{}, strings.TrimSpace(id))
}

func ID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(taskIDKey{}).(string)
	return strings.TrimSpace(id)
}
