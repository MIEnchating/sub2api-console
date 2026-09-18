package newapimanagement_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/newapimanagement"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type queuedChannelRunner struct{ run func(context.Context) }

func (r *queuedChannelRunner) Go(run func(context.Context)) error               { r.run = run; return nil }
func (r *queuedChannelRunner) GoTask(_ string, run func(context.Context)) error { return r.Go(run) }
func (r *queuedChannelRunner) CancelTask(string) bool                           { return false }

func TestChannelBatchPersistsPartialResultsWithoutReplayingFailedChannel(t *testing.T) {
	writes := []string{}
	models := map[string]string{"41": "gpt-5", "42": "gpt-5"}
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut {
			var body struct {
				ID     int    `json:"id"`
				Models string `json:"models"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			id := fmt.Sprint(body.ID)
			writes = append(writes, id)
			if id == "42" {
				fmt.Fprint(w, `{"success":false,"message":"rejected"}`)
				return
			}
			models[id] = body.Models
			fmt.Fprint(w, `{"success":true}`)
			return
		}
		row := func(id string) map[string]any {
			return map[string]any{"id": json.Number(id), "name": "渠道" + id, "type": 59, "status": 1, "group": "default", "models": models[id]}
		}
		var data any
		if r.URL.Path == "/api/channel/" {
			data = map[string]any{"items": []any{row("41"), row("42")}, "total": 2}
		} else {
			data = row(strings.TrimPrefix(r.URL.Path, "/api/channel/"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data})
	}))
	defer remote.Close()
	private, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer private.Close()
	_, err = private.SaveNewAPIPlatform(context.Background(), configstore.NewAPIPlatform{ID: "primary", Name: "测试", BaseURL: remote.URL, UserID: "1", AdminKey: "test-only"})
	if err != nil {
		t.Fatal(err)
	}
	service := newapimanagement.New(private, nil, remote.Client(), nil, nil)
	page, err := service.Channels(context.Background(), "primary", 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	store, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	runner := &queuedChannelRunner{}
	tasks := newapimanagement.NewChannelTasks(service, store, runner)
	input := newapimanagement.ChannelBatchInput{Action: "add", Models: []string{"gpt-5-mini"}}
	for _, item := range page.Items {
		input.Channels = append(input.Channels, newapimanagement.ChannelTarget{ID: item.ID, Version: item.Version})
	}
	task, err := tasks.Enqueue(context.Background(), "primary", input)
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != "queued" || len(writes) != 0 {
		t.Fatal("writes occurred before background task")
	}
	runner.run(context.Background())
	result, err := store.Get(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "partial" || result.Progress != 100 || !reflect.DeepEqual(writes, []string{"41", "42"}) {
		t.Fatalf("unexpected result: %+v writes=%v", result, writes)
	}
	items := result.Result["items"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["status"] != "succeeded" || items[1].(map[string]any)["status"] != "failed" {
		t.Fatalf("missing item results: %v", items)
	}
	if task.Result["phase"] != "queued" {
		t.Fatal("background task mutated returned task snapshot")
	}
}

func TestChannelBatchRejectsDuplicateTargetsBeforeCreatingTask(t *testing.T) {
	fixture := newChannelFixture(t)
	channel := fixture.channel(t)
	store, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	runner := &queuedChannelRunner{}
	tasks := newapimanagement.NewChannelTasks(fixture.service, store, runner)
	target := newapimanagement.ChannelTarget{ID: channel.ID, Version: channel.Version}
	_, err = tasks.Enqueue(context.Background(), "primary", newapimanagement.ChannelBatchInput{Action: "add", Models: []string{"gpt-5-nano"}, Channels: []newapimanagement.ChannelTarget{target, target}})
	if err == nil || runner.run != nil {
		t.Fatal("duplicate targets enqueued")
	}
}

func TestChannelBatchCancelledBeforeExecutionDoesNotWrite(t *testing.T) {
	fixture := newChannelFixture(t)
	channel := fixture.channel(t)
	store, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	runner := &queuedChannelRunner{}
	tasks := newapimanagement.NewChannelTasks(fixture.service, store, runner)
	task, err := tasks.Enqueue(context.Background(), "primary", newapimanagement.ChannelBatchInput{Action: "add", Models: []string{"gpt-5-nano"}, Channels: []newapimanagement.ChannelTarget{{ID: channel.ID, Version: channel.Version}}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner.run(ctx)
	result, err := store.Get(context.Background(), task.ID)
	if err != nil || result.Status != "cancelled" {
		t.Fatalf("cancellation not persisted: %+v %v", result, err)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if len(fixture.writes) != 0 {
		t.Fatal("cancelled task wrote upstream")
	}
}

type failChannelProgressStore struct {
	store *taskstore.Store
	calls int
}

func (s *failChannelProgressStore) Save(ctx context.Context, task taskstore.Task) error {
	s.calls++
	if s.calls == 2 {
		return fmt.Errorf("isolated storage failure")
	}
	return s.store.Save(ctx, task)
}

func TestChannelBatchProgressFailureStopsWritesAndMarksTaskFailed(t *testing.T) {
	fixture := newChannelFixture(t)
	channel := fixture.channel(t)
	store, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	runner := &queuedChannelRunner{}
	tasks := newapimanagement.NewChannelTasks(fixture.service, &failChannelProgressStore{store: store}, runner)
	task, err := tasks.Enqueue(context.Background(), "primary", newapimanagement.ChannelBatchInput{Action: "add", Models: []string{"gpt-5-nano"}, Channels: []newapimanagement.ChannelTarget{{ID: channel.ID, Version: channel.Version}}})
	if err != nil {
		t.Fatal(err)
	}
	runner.run(context.Background())
	result, err := store.Get(context.Background(), task.ID)
	if err != nil || result.Status != "failed" {
		t.Fatalf("progress failure left active task: %+v %v", result, err)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if len(fixture.writes) != 0 {
		t.Fatal("write proceeded without persistent progress")
	}
}

func TestChannelRemovalSkipsAlreadyEmptyChannelWithoutWriting(t *testing.T) {
	fixture := newChannelFixture(t)
	fixture.mu.Lock()
	fixture.models = ""
	fixture.mu.Unlock()
	channel := fixture.channel(t)
	updated, err := fixture.service.ChangeChannelModels(context.Background(), "primary", channel.ID, newapimanagement.ChannelModelChange{Action: "remove", Models: []string{"gpt-5"}, Version: channel.Version})
	if err != nil || len(updated.Models) != 0 {
		t.Fatalf("no-op removal rejected: %+v %v", updated, err)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if len(fixture.writes) != 0 {
		t.Fatal("no-op removal sent remote write")
	}
}
