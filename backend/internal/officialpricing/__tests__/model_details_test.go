package officialpricing_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	pricing "github.com/MIEnchating/sub2api-console/backend/internal/officialpricing"
)

func TestInvalidModelDetailsStopRemainingRequests(t *testing.T) {
	for _, provider := range []string{"claude", "openai"} {
		t.Run(provider, func(t *testing.T) {
			var requests atomic.Int32
			client := westernClient(t, provider, func(file, raw string) string {
				if strings.HasPrefix(file, "model-details/") {
					requests.Add(1)
					return "maintenance"
				}
				return raw
			})
			prices, err := pricing.Fetch(context.Background(), client, westernProvider(provider))
			if err == nil || len(prices) != 0 || !strings.Contains(err.Error(), "官方") {
				t.Fatalf("expected original validation failure and no partial catalog: %v %+v", err, prices)
			}
			if got := requests.Load(); got > 4 {
				t.Fatalf("requested %d model pages after validation failed; want at most the 4 active requests", got)
			}
		})
	}
}

func TestModelDetailFailureCancelsActiveReadsAndPreservesOriginalError(t *testing.T) {
	var started atomic.Int32
	var canceled atomic.Int32
	ready := make(chan struct{})
	client := westernClient(t, "claude", nil)
	base := client.Transport
	client.Transport = roundTrip(func(req *http.Request) (*http.Response, error) {
		if !strings.Contains(req.URL.Path, "/models/") {
			return base.RoundTrip(req)
		}
		n := started.Add(1)
		if n == 4 {
			close(ready)
		}
		if n == 1 {
			select {
			case <-ready:
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("maintenance"))}, nil
			case <-req.Context().Done():
				return nil, req.Context().Err()
			}
		}
		<-req.Context().Done()
		canceled.Add(1)
		return nil, req.Context().Err()
	})
	prices, err := pricing.Fetch(t.Context(), client, westernProvider("claude"))
	if err == nil || !strings.Contains(err.Error(), "官方模型 ID 未确认") || len(prices) != 0 {
		t.Fatalf("original failure lost: prices=%+v err=%v", prices, err)
	}
	if started.Load() != 4 || canceled.Load() != 3 {
		t.Fatalf("active reads not canceled: started=%d canceled=%d", started.Load(), canceled.Load())
	}
}

func TestCallerCancellationStopsModelDetailsWithoutPartialCatalog(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	client := westernClient(t, "claude", nil)
	base := client.Transport
	client.Transport = roundTrip(func(req *http.Request) (*http.Response, error) {
		if !strings.Contains(req.URL.Path, "/models/") {
			return base.RoundTrip(req)
		}
		cancel()
		return nil, req.Context().Err()
	})
	prices, err := pricing.Fetch(ctx, client, westernProvider("claude"))
	if !errors.Is(err, context.Canceled) || len(prices) != 0 {
		t.Fatalf("cancellation returned partial prices: prices=%+v err=%v", prices, err)
	}
}

func TestConcurrentModelDetailsKeepPublishedModelAndAliasOrder(t *testing.T) {
	prices, err := pricing.Fetch(t.Context(), westernClient(t, "claude", nil), westernProvider("claude"))
	if err != nil {
		t.Fatal(err)
	}
	if prices[0].Model != "claude-fable-5-1" || prices[1].Model != "claude-mythos-5-1" {
		t.Fatalf("published order changed: %s, %s", prices[0].Model, prices[1].Model)
	}
	for i, p := range prices {
		if p.Model == "claude-sonnet-4-5-20250929" {
			if i+1 == len(prices) || prices[i+1].Model != "claude-sonnet-4-5" {
				t.Fatal("alias no longer follows its published model")
			}
			return
		}
	}
	t.Fatal("published model missing")
}
