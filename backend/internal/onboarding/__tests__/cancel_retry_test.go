package onboarding_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/onboarding"
)

type cancelAfterKeyRepository struct {
	*business.Store
	cancel          context.CancelFunc
	corruptSnapshot bool
}

func (r *cancelAfterKeyRepository) SavePendingOnboarding(ctx context.Context, pending business.PendingOnboarding) error {
	if r.corruptSnapshot {
		pending.FrozenIntentJSON = strings.Replace(pending.FrozenIntentJSON, `"priority":`, `"changed_priority":`, 1)
	}
	if err := r.Store.SavePendingOnboarding(ctx, pending); err != nil {
		return err
	}
	if pending.UpstreamKeyID != "" && r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
	return nil
}

func TestCancelledOnboardingRejectsChangedRequestBeforeKeyReuse(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*onboarding.Request)
	}{
		{"priority", func(r *onboarding.Request) { value := int64(999); r.Priority = &value }},
		{"concurrency", func(r *onboarding.Request) { value := int64(999); r.Concurrency = &value }},
		{"notes", func(r *onboarding.Request) { value := "changed"; r.Notes = &value }},
		{"models", func(r *onboarding.Request) { r.ModelMapping = map[string]string{"alias": "gemini-2.5-flash"} }},
		{"scheduling", func(r *onboarding.Request) { r.Schedulable = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			service, _, keys, request := newService(t, "http://127.0.0.1:1", "http://127.0.0.1:1", func(store *business.Store) onboarding.Repository {
				return &cancelAfterKeyRepository{Store: store, cancel: cancel}
			})
			if _, err := service.Onboard(ctx, request); err == nil || ctx.Err() == nil {
				t.Fatalf("expected cancellation: %v", err)
			}
			tc.change(&request)
			if _, err := service.Onboard(t.Context(), request); err == nil || !strings.Contains(err.Error(), "首次冻结的开户意图不一致") {
				t.Fatalf("changed request accepted: %v", err)
			}
			if keys.creates != 1 || keys.reveals != 0 {
				t.Fatalf("changed request reached key client: %+v", keys)
			}
		})
	}
}

func TestCorruptFrozenSnapshotRejectsRetryAndPreview(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	service, _, keys, request := newService(t, "http://127.0.0.1:1", "http://127.0.0.1:1", func(store *business.Store) onboarding.Repository {
		return &cancelAfterKeyRepository{Store: store, cancel: cancel, corruptSnapshot: true}
	})
	if _, err := service.Onboard(ctx, request); err == nil || ctx.Err() == nil {
		t.Fatalf("expected cancellation: %v", err)
	}
	if _, err := service.PreviewConcurrency(t.Context(), []onboarding.Request{request}); err == nil || !strings.Contains(err.Error(), "冻结参数校验失败") {
		t.Fatalf("corrupt snapshot accepted in preview: %v", err)
	}
	if _, err := service.Onboard(t.Context(), request); err == nil || !strings.Contains(err.Error(), "冻结参数校验失败") {
		t.Fatalf("corrupt snapshot accepted in retry: %v", err)
	}
	if keys.creates != 1 || keys.reveals != 0 {
		t.Fatalf("corrupt snapshot reached key client: %+v", keys)
	}
}

func TestCancelledOnboardingReusesFrozenAllocationAfterCapacityChanges(t *testing.T) {
	for _, tc := range []struct {
		name    string
		limit   int64
		blocked bool
	}{
		{"increased capacity keeps original allocation", 9, false},
		{"decreased capacity rejects before remote writes", 5, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var posts atomic.Int64
			admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/api/v1/admin/accounts/models/sync-upstream-preview" {
					_, _ = w.Write([]byte(`{"data":{"models":["gemini-2.5-flash"]}}`))
					return
				}
				if r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/accounts" {
					posts.Add(1)
					var body map[string]any
					decoder := json.NewDecoder(r.Body)
					decoder.UseNumber()
					if err := decoder.Decode(&body); err != nil {
						t.Error(err)
						return
					}
					if body["concurrency"] != json.Number("7") {
						t.Errorf("original concurrency lost: %v", body["concurrency"])
					}
					body["id"] = 77
					_ = json.NewEncoder(w).Encode(map[string]any{"data": body})
					return
				}
				http.NotFound(w, r)
			}))
			t.Cleanup(admin.Close)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			service, repo, keys, request := newService(t, admin.URL, admin.URL, func(store *business.Store) onboarding.Repository {
				return &cancelAfterKeyRepository{Store: store, cancel: cancel}
			})
			setLimit := func(limit int64) {
				t.Helper()
				if _, err := repo.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{Host: "upstream.test", Balance: &business.UpstreamBalanceObservation{ConcurrencyLimit: &limit, ProfileUserID: "17"}}); err != nil {
					t.Fatal(err)
				}
			}
			setLimit(7)
			if _, err := service.Onboard(ctx, request); err == nil || ctx.Err() == nil {
				t.Fatalf("expected cancellation after key confirmation: %v", err)
			}
			pending, err := repo.PendingOnboarding(t.Context(), "upstream.test", "6", []string{"3"})
			if err != nil || pending == nil || pending.UpstreamKeyID != "91" || pending.FrozenIntentJSON == "" {
				t.Fatalf("lost pending intent: %+v %v", pending, err)
			}
			setLimit(tc.limit)
			preview, previewErr := service.PreviewConcurrency(t.Context(), []onboarding.Request{request})
			if tc.blocked {
				if previewErr == nil {
					t.Error("preview silently shrank frozen allocation")
				}
			} else if previewErr != nil || len(preview) != 1 || preview[0].Concurrency == nil || *preview[0].Concurrency != 7 {
				t.Errorf("preview must retain frozen concurrency 7: %+v %v", preview, previewErr)
			}
			result, err := service.Onboard(t.Context(), request)
			if tc.blocked {
				if err == nil || !strings.Contains(err.Error(), "剩余额度") || posts.Load() != 0 || keys.reveals != 0 {
					t.Fatalf("retry bypassed current capacity: result=%v err=%v posts=%d reveals=%d", result, err, posts.Load(), keys.reveals)
				}
			} else if err != nil || result["account_id"] != "77" || posts.Load() != 1 {
				t.Fatalf("retry failed: result=%v err=%v posts=%d", result, err, posts.Load())
			}
			if keys.creates != 1 {
				t.Fatalf("retry duplicated key creation: %d", keys.creates)
			}
		})
	}
}
