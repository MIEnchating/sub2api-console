package newapimanagement_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/newapimanagement"
)

var pricingReads = []struct {
	name string
	read func(context.Context, *newapimanagement.Service) error
}{
	{"catalog", func(ctx context.Context, service *newapimanagement.Service) error {
		_, err := service.ModelPriceCatalog(ctx, "test", false)
		return err
	}},
	{"source", func(ctx context.Context, service *newapimanagement.Service) error {
		_, err := service.RemoteModelPricingSource(ctx, "test")
		return err
	}},
}

// Observe when a reader starts selecting on cancellation, without sleeps.
type pricingWaitContext struct {
	context.Context
	listening chan struct{}
	once      sync.Once
}

func (c *pricingWaitContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.listening) })
	return c.Context.Done()
}

func TestPricingReadAlreadyCanceledDoesNotStartRemoteWork(t *testing.T) {
	for _, reader := range pricingReads {
		t.Run(reader.name, func(t *testing.T) {
			var calls atomic.Int32
			page := ""
			service := setupCatalog(t, &page, transport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				return nil, errors.New("canceled read must not reach HTTP")
			}))
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			if err := reader.read(ctx, service); !errors.Is(err, context.Canceled) {
				t.Fatalf("already canceled read returned %v", err)
			}
			if calls.Load() != 0 {
				t.Fatal("already canceled read started remote work")
			}
		})
	}
}

func TestPricingReadCanceledBehindRefreshReturnsBeforeRefreshCompletes(t *testing.T) {
	for _, holder := range pricingReads {
		for _, waiter := range pricingReads {
			t.Run(holder.name+"/"+waiter.name, func(t *testing.T) {
				started := make(chan struct{})
				release := make(chan struct{})
				var releaseOnce sync.Once
				unblock := func() { releaseOnce.Do(func() { close(release) }) }
				var remoteCalls atomic.Int32
				page := ""
				service := setupCatalog(t, &page, transport(func(r *http.Request) (*http.Response, error) {
					body := `{"success":true,"data":[]}`
					if r.URL.Host == "raw.githubusercontent.com" {
						if remoteCalls.Add(1) == 1 {
							close(started)
						}
						select {
						case <-release:
						case <-r.Context().Done():
							return nil, r.Context().Err()
						}
						body = `{"other-model":{"input_cost_per_token":0.000002,"output_cost_per_token":0.000004}}`
					}
					return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
				}))
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				var running sync.WaitGroup
				defer func() { unblock(); running.Wait() }()
				holderDone := make(chan error, 1)
				running.Go(func() { holderDone <- holder.read(ctx, service) })
				select {
				case <-started:
				case <-ctx.Done():
					t.Fatal("first refresh did not reach the isolated transport")
				}
				waitCtx, stop := context.WithCancel(ctx)
				defer stop()
				waiterDone := make(chan error, 1)
				observed := &pricingWaitContext{Context: waitCtx, listening: make(chan struct{})}
				running.Go(func() { waiterDone <- waiter.read(observed, service) })
				guard := time.NewTimer(2 * time.Second)
				defer guard.Stop()
				select {
				case <-observed.listening:
				case <-guard.C:
					t.Fatal("queued read did not listen for cancellation")
				}
				stop()
				select {
				case err := <-waiterDone:
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("canceled read returned %v", err)
					}
				case <-guard.C:
					t.Fatal("canceled read remained blocked behind an unfinished refresh")
				}
				select {
				case err := <-holderDone:
					t.Fatalf("canceling a waiter ended the active refresh: %v", err)
				default:
				}
				unblock()
				if err := <-holderDone; err != nil {
					t.Fatalf("active refresh failed: %v", err)
				}
				for _, next := range pricingReads {
					if err := next.read(ctx, service); err != nil {
						t.Fatalf("read after cancellation (%s): %v", next.name, err)
					}
				}
				if calls := remoteCalls.Load(); calls != 1 {
					t.Fatalf("queued cancellation or cached read caused extra remote requests: %d", calls)
				}
			})
		}
	}
}
