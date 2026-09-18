package opstraffic_test

import (
	"context"
	"fmt"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/opstraffic"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type liveTarget struct {
	url, key string
	reads    int
	change   bool
}

func (r *liveTarget) TargetSettings(context.Context) (configstore.TargetSettings, error) {
	r.reads++
	key := r.key
	if r.change && r.reads > 1 {
		key = "changed"
	}
	return configstore.TargetSettings{BaseURL: r.url, AdminKey: key}, nil
}

func TestLiveTrafficCacheIsBoundToManagementTarget(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprintf(w, `{"data":{"enabled":true,"timestamp":%q,"account":{}}}`, time.Now().UTC().Format(time.RFC3339Nano))
	}))
	defer server.Close()
	target := &liveTarget{url: server.URL, key: "first"}
	service := opstraffic.New(target, nil)
	for i := 0; i < 2; i++ {
		if _, err := service.AccountTraffic(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("cache missed: %d", calls.Load())
	}
	target.key = "second"
	if _, err := service.AccountTraffic(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatal("new target inherited old cache")
	}
}

func TestLiveTrafficRejectsStaleSnapshotAndTargetChange(t *testing.T) {
	for _, name := range []string{"stale", "target changed", "upstream failure"} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if name == "upstream failure" {
					w.WriteHeader(503)
					return
				}
				at := time.Now()
				if name == "stale" {
					at = at.Add(-time.Minute)
				}
				fmt.Fprintf(w, `{"data":{"enabled":true,"timestamp":%q,"account":{}}}`, at.UTC().Format(time.RFC3339Nano))
			}))
			defer server.Close()
			service := opstraffic.New(&liveTarget{url: server.URL, key: "fixture", change: name == "target changed"}, nil)
			if _, err := service.AccountTraffic(context.Background()); err == nil {
				t.Fatal("untrustworthy snapshot accepted")
			}
		})
	}
}
