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
	"sync"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/newapimanagement"
)

type channelFixture struct {
	store    *configstore.Store
	mu       sync.Mutex
	models   string
	fail     bool
	mismatch bool
	writes   []map[string]any
	service  *newapimanagement.Service
}

func newChannelFixture(t *testing.T) *channelFixture {
	t.Helper()
	fixture := &channelFixture{models: "gpt-5,gpt-5-mini"}
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fixture.mu.Lock()
		defer fixture.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing authentication")
		}
		row := map[string]any{"id": 42, "name": "测试渠道", "type": 59, "status": 1, "models": fixture.models, "group": "default", "key": "must-not-leak", "setting": "private-proxy"}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			fixture.writes = append(fixture.writes, body)
			if fixture.fail {
				fmt.Fprint(w, `{"success":false,"message":"write rejected"}`)
				return
			}
			if !fixture.mismatch {
				fixture.models = body["models"].(string)
			}
			fmt.Fprint(w, `{"success":true}`)
			return
		}
		var data any = row
		if r.URL.Path == "/api/channel/" {
			data = map[string]any{"items": []any{row}, "total": 1}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data})
	}))
	t.Cleanup(remote.Close)
	store, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	_, err = store.SaveNewAPIPlatform(context.Background(), configstore.NewAPIPlatform{ID: "primary", Name: "测试", BaseURL: remote.URL, AdminKey: "test-key", UserID: "1"})
	if err != nil {
		t.Fatal(err)
	}
	fixture.store = store
	fixture.service = newapimanagement.New(store, nil, remote.Client(), nil, nil)
	return fixture
}

func (f *channelFixture) channel(t *testing.T) newapimanagement.Channel {
	t.Helper()
	page, err := f.service.Channels(context.Background(), "primary", 0, 50)
	if err != nil || len(page.Items) != 1 || page.Total != 1 {
		t.Fatalf("list failed: %+v %v", page, err)
	}
	return page.Items[0]
}

func TestChannelMaintenanceListsOnlyPublicFields(t *testing.T) {
	fixture := newChannelFixture(t)
	row := fixture.channel(t)
	raw, _ := json.Marshal(row)
	if row.ID != "42" || strings.Contains(string(raw), "must-not-leak") || strings.Contains(string(raw), "private-proxy") {
		t.Fatalf("unsafe public result: %s", raw)
	}
}

func TestChannelMaintenanceAddsAndRemovesOnlyRequestedModels(t *testing.T) {
	fixture := newChannelFixture(t)
	initial := fixture.channel(t)
	added, err := fixture.service.ChangeChannelModels(context.Background(), "primary", "42", newapimanagement.ChannelModelChange{Action: "add", Models: []string{"gpt-5", "gpt-5-nano", "gpt-5-nano"}, Version: initial.Version})
	if err != nil || !reflect.DeepEqual(added.Models, []string{"gpt-5", "gpt-5-mini", "gpt-5-nano"}) {
		t.Fatalf("add failed: %+v %v", added, err)
	}
	removed, err := fixture.service.ChangeChannelModels(context.Background(), "primary", "42", newapimanagement.ChannelModelChange{Action: "remove", Models: []string{"gpt-5-mini"}, Version: added.Version})
	if err != nil || !reflect.DeepEqual(removed.Models, []string{"gpt-5", "gpt-5-nano"}) {
		t.Fatalf("remove failed: %+v %v", removed, err)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	for _, body := range fixture.writes {
		if len(body) != 2 || body["id"] != float64(42) {
			t.Fatalf("unrelated fields written: %+v", body)
		}
	}
}

func TestChannelMaintenanceRejectsStaleVersionAndEmptyResultBeforeWrite(t *testing.T) {
	fixture := newChannelFixture(t)
	initial := fixture.channel(t)
	for _, input := range []newapimanagement.ChannelModelChange{
		{Action: "add", Models: []string{"gpt-5-nano"}, Version: strings.Repeat("0", 64)},
		{Action: "remove", Models: initial.Models, Version: initial.Version},
		{Action: "add", Models: []string{"bad,name"}, Version: initial.Version},
	} {
		if _, err := fixture.service.ChangeChannelModels(context.Background(), "primary", "42", input); err == nil {
			t.Fatalf("invalid change accepted: %+v", input)
		}
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if len(fixture.writes) != 0 {
		t.Fatal("invalid change wrote upstream")
	}
}

func TestChannelMaintenanceDoesNotReplayRejectedWriteOrClaimReadbackMismatchSucceeded(t *testing.T) {
	for _, mode := range []string{"business failure", "readback mismatch"} {
		t.Run(mode, func(t *testing.T) {
			fixture := newChannelFixture(t)
			initial := fixture.channel(t)
			fixture.mu.Lock()
			fixture.fail = mode == "business failure"
			fixture.mismatch = mode == "readback mismatch"
			fixture.mu.Unlock()
			_, err := fixture.service.ChangeChannelModels(context.Background(), "primary", "42", newapimanagement.ChannelModelChange{Action: "add", Models: []string{"gpt-5-nano"}, Version: initial.Version})
			if err == nil {
				t.Fatal("unconfirmed write reported success")
			}
			fixture.mu.Lock()
			defer fixture.mu.Unlock()
			if len(fixture.writes) != 1 {
				t.Fatalf("write replayed: %d", len(fixture.writes))
			}
		})
	}
}

func TestChannelMaintenanceRejectsPreviewFromChangedPlatform(t *testing.T) {
	fixture := newChannelFixture(t)
	initial := fixture.channel(t)
	platform, err := fixture.store.NewAPIPlatform(context.Background(), "primary")
	if err != nil {
		t.Fatal(err)
	}
	platform.UserID = "2"
	if _, err := fixture.store.SaveNewAPIPlatform(context.Background(), *platform); err != nil {
		t.Fatal(err)
	}
	_, err = fixture.service.ChangeChannelModels(context.Background(), "primary", "42", newapimanagement.ChannelModelChange{Action: "add", Models: []string{"gpt-5-nano"}, Version: initial.Version})
	if newapimanagement.KindOf(err) != newapimanagement.ErrorConflict {
		t.Fatalf("stale platform accepted: %v", err)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if len(fixture.writes) != 0 {
		t.Fatal("stale platform wrote upstream")
	}
}
