package main

import (
	"context"
	"errors"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskcontext"
)

func newServiceContext(parent context.Context) (context.Context, context.CancelFunc) {
	// Database instance loss stops the process just like a shutdown signal.
	// Preserve its cause, but classify running tasks as recoverable interrupts.
	ctx, cancel := context.WithCancelCause(context.WithoutCancel(parent))
	interrupt := func() { cancel(errors.Join(taskcontext.ErrInterrupted, context.Cause(parent))) }
	if parent.Err() != nil {
		interrupt()
	}
	stopForwarding := context.AfterFunc(parent, interrupt)
	return ctx, func() {
		stopForwarding()
		interrupt()
	}
}
