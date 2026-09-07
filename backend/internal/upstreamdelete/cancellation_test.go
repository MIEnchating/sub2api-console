package upstreamdelete

import (
	"context"
	"errors"
	"testing"
)

// Cancellation is injected at the worker's first cancellation check, after the
// producer handed it a job. Done follows the actual context throughout.
type cancelOnWorkerCheck struct {
	context.Context
	cancel context.CancelFunc
}

func (ctx cancelOnWorkerCheck) Err() error {
	ctx.cancel()
	return ctx.Context.Err()
}

type unexpectedAccountDelete struct{ calls int }

func (client *unexpectedAccountDelete) DeleteAccountWithVerification(context.Context, string, bool) (map[string]any, error) {
	client.calls++
	return map[string]any{"confirmed_absent": true}, nil
}

func TestDeleteAccountsMarksDispatchedButCancelledAccountAsFailed(t *testing.T) {
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := cancelOnWorkerCheck{Context: base, cancel: cancel}
	client := &unexpectedAccountDelete{}
	results := deleteAccounts(ctx, client, []string{"41", "42"}, true, 1)
	for index, err := range results {
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("account %d was not deleted but reported success: %v", index, results)
		}
	}
	if client.calls != 0 {
		t.Fatalf("cancelled delete made %d remote calls", client.calls)
	}
}
