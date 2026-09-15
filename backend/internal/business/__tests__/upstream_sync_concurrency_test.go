package business_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestConcurrentUpstreamSyncAndInspectionPreserveEveryCatalog(t *testing.T) {
	store, err := business.Open(filepath.Join(t.TempDir(), "concurrent-sync.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	upstreams := []struct{ host, rate string }{
		{"primary.example.test", "0.2"},
		{"secondary.example.test", "1.5"},
		{"fallback.example.test", "2"},
	}
	for _, upstream := range upstreams {
		if _, err := store.CreateUpstreamConfiguration(ctx, business.UpstreamConfigurationWrite{
			Host: upstream.host, BaseURL: "https://" + upstream.host,
			UpstreamType: "sub2api", AuthMode: "access_token", RechargeRate: "1",
		}); err != nil {
			t.Fatal(err)
		}
	}
	start := make(chan struct{})
	completed := make(chan error, len(upstreams)+1)
	for _, upstream := range upstreams {
		go func() {
			<-start
			_, err := store.ApplyUpstreamSync(ctx, business.UpstreamSyncWrite{
				Host: upstream.host, AuthenticationOK: true,
				Catalog: &business.UpstreamCatalogSnapshot{Groups: []business.UpstreamCatalogGroup{
					{GroupID: "7", Name: "standard", RawRate: &upstream.rate},
				}},
			})
			completed <- err
		}()
	}
	go func() {
		<-start
		completed <- store.RecordInspectionHeartbeat(ctx, business.InspectionHeartbeat{
			CheckedAt: "2026-09-14T07:57:52Z", Status: "running",
		})
	}()
	close(start)
	for range len(upstreams) + 1 {
		if err := <-completed; err != nil {
			t.Errorf("concurrent local write failed: %v", err)
		}
	}
	for _, upstream := range upstreams {
		groups, err := store.UpstreamGroups(ctx, upstream.host, true)
		if err != nil {
			t.Fatal(err)
		}
		if len(groups) != 1 || groups[0].RawRate == nil || *groups[0].RawRate != upstream.rate {
			t.Fatalf("catalog for %s was not committed: %#v", upstream.host, groups)
		}
	}
	heartbeats, err := store.InspectionHeartbeats(ctx, 1)
	if err != nil || len(heartbeats) != 1 || heartbeats[0].Status != "running" {
		t.Fatalf("inspection heartbeat was not committed: %#v, %v", heartbeats, err)
	}
}
