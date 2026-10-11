package newapimanagement_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/newapimanagement"
)

func TestSnapshotIndependentCatalogReadsStartBeforeAnyCompletes(t *testing.T) {
	store, err := configstore.Open(filepath.Join(t.TempDir(), "pricing.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.SaveAuthRecord(context.Background(), configstore.AuthRecord{Host: "upstream.test", BaseURL: "https://upstream.test", UpstreamType: "newapi", AuthMode: "newapi_user_token"}, nil); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	started := make(chan string, 3)
	release := make(chan struct{})
	client := &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		body := `{"success":true,"data":[]}`
		if r.URL.Host == "newapi.test" && r.URL.Path == "/api/option/" {
			body = `{"success":true,"data":{"ModelRatio":"{\"model-a\":1}","CompletionRatio":"{\"model-a\":2}"}}`
		} else {
			started <- r.URL.Host + r.URL.Path
			select {
			case <-release:
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
			if r.URL.Path == "/api/pricing" {
				body = `{"success":true,"data":[{"model_name":"model-a","quota_type":0,"model_ratio":1,"completion_ratio":2}]}`
			}
			if r.URL.Path == "/api/channel/models_enabled" {
				body = `{"success":true,"data":["model-a","unset"]}`
			}
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	service := newapimanagement.New(&catalogStore{store}, nil, client, nil, nil)
	type result struct {
		snapshot newapimanagement.RemoteSnapshot
		err      error
	}
	done := make(chan result, 1)
	go func() { snapshot, err := service.Refresh(ctx, "test"); done <- result{snapshot, err} }()
	seen := map[string]bool{}
	for len(seen) < 3 {
		select {
		case path := <-started:
			seen[path] = true
		case <-ctx.Done():
			close(release)
			<-done
			t.Fatalf("independent reads remained serialized: %v", seen)
		}
	}
	close(release)
	got := <-done
	if got.err != nil || len(got.snapshot.Models) != 1 || len(got.snapshot.UnsetModels) != 1 || len(got.snapshot.UpstreamPrices) != 1 || len(got.snapshot.Differences) != 0 {
		t.Fatalf("snapshot=%+v err=%v", got.snapshot, got.err)
	}
}

func TestSnapshotCancelledDuringOptionalReadsDoesNotPublishPartialSuccess(t *testing.T) {
	page := officialPage
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	service := setupCatalog(t, &page, transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/option/" {
			cancel()
			return nil, context.Canceled
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"success":true,"data":{}}`))}, nil
	}))
	_, err := service.Refresh(ctx, "test")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled snapshot reported success: %v", err)
	}
}
