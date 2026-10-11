package newapimanagement_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/newapimanagement"
)

func TestChannelLookupLimitsConcurrencyAndStopsQueuedReadsOnCancellation(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		name := "complete"
		if cancelled {
			name = "cancel"
		}
		t.Run(name, func(t *testing.T) {
			var active, maximum, calls atomic.Int32
			entered := make(chan struct{}, 50)
			release := make(chan struct{})
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				n := active.Add(1)
				defer active.Add(-1)
				for old := maximum.Load(); n > old; old = maximum.Load() {
					if maximum.CompareAndSwap(old, n) {
						break
					}
				}
				entered <- struct{}{}
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				id := strings.TrimPrefix(r.URL.Path, "/api/channel/")
				_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"id": id, "name": "选择项", "type": 1, "status": 1, "models": "model-a", "group": "default"}})
			}))
			defer remote.Close()
			store, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			_, err = store.SaveNewAPIPlatform(t.Context(), configstore.NewAPIPlatform{ID: "primary", Name: "测试", BaseURL: remote.URL, UserID: "1", AdminKey: "fixture"})
			if err != nil {
				t.Fatal(err)
			}
			service := newapimanagement.New(store, nil, remote.Client(), nil, nil)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				ids := make([]string, 50)
				for i := range ids {
					ids[i] = strconv.Itoa(50 - i)
				}
				result, err := service.LookupChannels(ctx, "primary", ids)
				if err == nil && (len(result.Items) != 50 || result.Items[0].ID != "50" || result.Items[49].ID != "1") {
					err = errors.New("selection order changed")
				}
				done <- err
			}()
			for range 4 {
				select {
				case <-entered:
				case <-ctx.Done():
					t.Fatal("four reads did not start")
				}
			}
			if cancelled {
				cancel()
			} else {
				close(release)
			}
			err = <-done
			if cancelled {
				if !errors.Is(err, context.Canceled) || calls.Load() != 4 {
					t.Fatalf("cancelled lookup: %v, calls=%d", err, calls.Load())
				}
			} else if err != nil || calls.Load() != 50 {
				t.Fatalf("lookup: %v, calls=%d", err, calls.Load())
			}
			if maximum.Load() != 4 {
				t.Fatalf("concurrency=%d", maximum.Load())
			}
		})
	}
}
