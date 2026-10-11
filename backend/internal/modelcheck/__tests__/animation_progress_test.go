package modelcheck_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type animationProgressStore struct {
	modelcheck.TaskStore
	mu         sync.Mutex
	generating int
	started    chan struct{}
}

func TestOAuthStreamingAnimationPublishesGeneratingOncePerResponse(t *testing.T) {
	t.Parallel()
	f, private := oauthAccountFixture(t, oauthAnimationAccount)
	store := &animationProgressStore{TaskStore: f.tasks, started: make(chan struct{})}
	service, err := modelcheck.New(store, private, f.catalog, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	service.UseTaskRunner(f.runner)
	service.UseOAuthTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) {
		var body strings.Builder
		for _, character := range fixtureSVG {
			_, _ = fmt.Fprintf(&body, "data: {\"type\":\"response.output_text.delta\",\"delta\":%q}\n\n", string(character))
		}
		body.WriteString("data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
		return oauthResponse(http.StatusOK, "text/event-stream", body.String()), nil
	}))
	if _, err := service.EnqueueAnimation(t.Context(), request("1")); err != nil {
		t.Fatal(err)
	}
	final := finished(t, f)
	rows := final.Result["animations"].([]modelcheck.AnimationResult)
	if final.Status != "succeeded" || len(rows) != 1 || rows[0].Source != fixtureSVG || rows[0].SVG == "" {
		t.Fatalf("OAuth stream lost its artifact: %+v", final)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.generating != 1 {
		t.Fatalf("one OAuth generation produced %d redundant generating snapshots", store.generating)
	}
}

func (store *animationProgressStore) Save(ctx context.Context, task taskstore.Task) error {
	if err := store.TaskStore.Save(ctx, task); err != nil {
		return err
	}
	rows, _ := task.Result["animations"].([]modelcheck.AnimationResult)
	if task.Status == "running" && len(rows) == 1 && rows[0].Phase == "generating" {
		store.mu.Lock()
		defer store.mu.Unlock()
		store.generating++
		if store.generating == 1 {
			close(store.started)
		}
	}
	return nil
}

func TestStreamingAnimationPublishesGeneratingOnceAndPreservesEveryTextChunk(t *testing.T) {
	t.Parallel()
	firstPublished := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	f := setup(t, 1, "openai", func(w http.ResponseWriter, request *http.Request) {
		_, _ = io.Copy(io.Discard, request.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		for index, character := range fixtureSVG {
			event, _ := json.Marshal(map[string]any{"type": "response.output_text.delta", "delta": string(character)})
			_, _ = fmt.Fprintf(w, "data: %s\n\n", event)
			if index == 0 {
				w.(http.Flusher).Flush()
				select {
				case <-release:
				case <-request.Context().Done():
					return
				}
			}
		}
		_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
	})
	t.Cleanup(unblock)
	store := &animationProgressStore{TaskStore: f.tasks, started: firstPublished}
	service, err := modelcheck.New(store, credentials{}, f.catalog, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	service.UseTaskRunner(f.runner)
	if _, err := service.EnqueueAnimation(t.Context(), request("1")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-firstPublished:
	case <-time.After(3 * time.Second):
		t.Fatal("first output did not publish generating before the remaining chunks")
	}
	unblock()
	final := finished(t, f)
	rows := final.Result["animations"].([]modelcheck.AnimationResult)
	if final.Status != "succeeded" || len(rows) != 1 || rows[0].Source != fixtureSVG || rows[0].SVG == "" {
		t.Fatalf("streamed artifact was changed or lost: %+v", final)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.generating != 1 {
		t.Fatalf("one generation produced %d redundant generating snapshots", store.generating)
	}
}

func TestCompatibleAnimationStreamsPublishOnlyOneGeneratingTransition(t *testing.T) {
	for _, tc := range []struct {
		name, platform, path, event, end string
	}{
		{"chat fallback", "openai", "/v1/chat/completions", `{"choices":[{"delta":{"content":%q}}]}`, "data: [DONE]\n\n"},
		{"anthropic", "anthropic", "/v1/messages", `{"type":"content_block_delta","delta":{"type":"text_delta","text":%q}}`, "data: {\"type\":\"message_stop\"}\n\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := setup(t, 1, tc.platform, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				for _, character := range fixtureSVG {
					_, _ = fmt.Fprintf(w, "data: "+tc.event+"\n\n", string(character))
				}
				_, _ = io.WriteString(w, tc.end)
			})
			store := &animationProgressStore{TaskStore: f.tasks, started: make(chan struct{})}
			service, err := modelcheck.New(store, credentials{}, f.catalog, credentials{})
			if err != nil {
				t.Fatal(err)
			}
			service.UseTaskRunner(f.runner)
			if _, err := service.EnqueueAnimation(t.Context(), request("1")); err != nil {
				t.Fatal(err)
			}
			final := finished(t, f)
			rows := final.Result["animations"].([]modelcheck.AnimationResult)
			if final.Status != "succeeded" || len(rows) != 1 || rows[0].Source != fixtureSVG || rows[0].SVG == "" {
				t.Fatalf("compatible stream lost its artifact: %+v", final)
			}
			store.mu.Lock()
			defer store.mu.Unlock()
			if store.generating != 1 {
				t.Fatalf("one compatible generation produced %d generating snapshots", store.generating)
			}
		})
	}
}
