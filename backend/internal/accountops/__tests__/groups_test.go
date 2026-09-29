package accountops_test

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

	"github.com/MIEnchating/sub2api-console/backend/internal/accountops"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

type groupsFixture struct {
	*settingsFixture
	remote       []int64
	writes       int
	reject       bool
	badReadback  bool
	remoteStatus string
	target       *settingsTarget
}

func newGroupsFixture(t *testing.T) *groupsFixture {
	t.Helper()
	store, err := business.Open(filepath.Join(t.TempDir(), "groups.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetMode(t.Context(), runtimepolicy.Full); err != nil {
		t.Fatal(err)
	}
	groupRows := []map[string]any{
		{"id": "7", "name": "原分组", "platform": "openai", "rate_multiplier": "1"},
		{"id": "8", "name": "目标分组", "platform": "openai", "rate_multiplier": "1"},
		{"id": "9", "name": "另一平台", "platform": "anthropic", "rate_multiplier": "1"},
		{"id": "10", "name": "混合平台", "platform": "composite", "rate_multiplier": "1"},
	}
	if _, err := store.SyncManagementSnapshot(t.Context(), []map[string]any{{"id": "41", "name": "账号", "platform": "openai", "priority": 20, "concurrency": 3, "schedulable": true, "group_ids": []any{"7"}}}, groupRows, "test"); err != nil {
		t.Fatal(err)
	}
	f := &groupsFixture{settingsFixture: &settingsFixture{repository: store, runner: &settingsRunner{}, tasks: &settingsTasks{}}, remote: []int64{7}, remoteStatus: "active"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/admin/groups" {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"items": []map[string]any{{"id": 7, "platform": "openai", "status": "active"}, {"id": 8, "platform": "openai", "status": f.remoteStatus}, {"id": 10, "platform": "composite", "status": "active"}}, "total": 3}})
			return
		}
		if r.URL.Path != "/api/v1/admin/accounts/41" {
			http.Error(w, "unexpected endpoint", 400)
			return
		}
		if r.Method == http.MethodPut {
			f.writes++
			if f.reject {
				http.Error(w, "write rejected", 400)
				return
			}
			var body struct {
				GroupIDs []int64 `json:"group_ids"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			if !f.badReadback {
				f.remote = body.GroupIDs
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": 41, "platform": "openai", "group_ids": f.remote}})
	}))
	t.Cleanup(server.Close)
	f.target = &settingsTarget{endpoint: server.URL}
	f.service = accountops.New(f.target, store, f.tasks)
	f.service.UseTaskRunner(f.runner)
	return f
}

func (f *groupsFixture) input(t *testing.T, ids ...string) accountops.AccountGroupsInput {
	t.Helper()
	preview, err := f.service.AccountGroups(t.Context(), "41")
	if err != nil {
		t.Fatal(err)
	}
	return accountops.AccountGroupsInput{GroupIDs: ids, ExpectedGroupIDs: preview.CurrentGroupIDs, TargetVersion: preview.TargetVersion}
}

func TestSwitchGroupsWritesConfirmedMembership(t *testing.T) {
	for _, ids := range [][]string{{"8"}, {"7", "8"}, {"8", "10"}} {
		t.Run(strings.Join(ids, ","), func(t *testing.T) {
			f := newGroupsFixture(t)
			if _, err := f.service.EnqueueGroups(t.Context(), "41", f.input(t, ids...), "operator"); err != nil {
				t.Fatal(err)
			}
			f.runner.run(context.Background())
			if f.tasks.last.Status != "succeeded" || f.writes != 1 {
				t.Fatalf("task=%+v writes=%d", f.tasks.last, f.writes)
			}
			catalog, err := f.repository.PricingCatalog(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(catalog.Accounts[0].GroupIDs, ids) {
				t.Fatalf("groups=%v", catalog.Accounts[0].GroupIDs)
			}
			groups, err := f.repository.Groups(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			for _, group := range groups {
				if group.Name == "目标分组" && group.AccountCount != 1 {
					t.Fatalf("group count=%d", group.AccountCount)
				}
			}
		})
	}
}

func TestSwitchGroupsRejectsInvalidSelectionBeforeRemoteAccess(t *testing.T) {
	for _, ids := range [][]string{nil, {"0"}, {"08"}, {"8", "8"}, {"999"}, {"9"}, {"7"}} {
		t.Run(fmt.Sprint(ids), func(t *testing.T) {
			f := newGroupsFixture(t)
			_, err := f.service.EnqueueGroups(t.Context(), "41", f.input(t, ids...), "operator")
			if err == nil || f.runner.run != nil || f.requests.Load() != 0 {
				t.Fatalf("err=%v requests=%d", err, f.requests.Load())
			}
		})
	}
}

func TestSwitchGroupsRejectsManualControlBeforeQueueAndExecution(t *testing.T) {
	for _, queued := range []bool{false, true} {
		t.Run(fmt.Sprint(queued), func(t *testing.T) {
			f := newGroupsFixture(t)
			input := f.input(t, "8")
			if queued {
				if _, err := f.service.EnqueueGroups(t.Context(), "41", input, "operator"); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := f.repository.AssignManualPriority(t.Context(), "41", 1, "1", 3, true, "test"); err != nil {
				t.Fatal(err)
			}
			if queued {
				f.runner.run(context.Background())
				if f.tasks.last.Status != "failed" || !strings.Contains(f.tasks.last.Message, "手动控制") {
					t.Fatalf("task=%+v", f.tasks.last)
				}
			} else {
				_, err := f.service.EnqueueGroups(t.Context(), "41", input, "operator")
				if err == nil || !strings.Contains(err.Error(), "手动控制") || f.runner.run != nil {
					t.Fatalf("err=%v", err)
				}
			}
			if f.requests.Load() != 0 {
				t.Fatalf("manual account accessed remote: %d", f.requests.Load())
			}
		})
	}
}

func TestQueuedGroupSwitchRejectsChangedPreconditionsWithoutWriting(t *testing.T) {
	for _, change := range []string{"remote groups", "local groups", "target", "mode", "inactive group"} {
		t.Run(change, func(t *testing.T) {
			f := newGroupsFixture(t)
			if _, err := f.service.EnqueueGroups(t.Context(), "41", f.input(t, "8"), "operator"); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "remote groups":
				f.remote = []int64{8}
			case "local groups":
				if _, err := f.repository.SyncPricingAccountGroups(t.Context(), map[string][]string{"41": {"8"}}, "test"); err != nil {
					t.Fatal(err)
				}
			case "target":
				f.target.endpoint += "/changed"
			case "mode":
				if _, err := f.repository.SetMode(t.Context(), runtimepolicy.Monitoring); err != nil {
					t.Fatal(err)
				}
			case "inactive group":
				f.remoteStatus = "disabled"
			}
			f.runner.run(context.Background())
			if f.tasks.last.Status != "failed" || f.writes != 0 {
				t.Fatalf("task=%+v writes=%d", f.tasks.last, f.writes)
			}
		})
	}
}

func TestSwitchGroupsDoesNotCommitUnconfirmedRemoteWrites(t *testing.T) {
	for _, failure := range []string{"rejected", "readback mismatch"} {
		t.Run(failure, func(t *testing.T) {
			f := newGroupsFixture(t)
			f.reject, f.badReadback = failure == "rejected", failure == "readback mismatch"
			if _, err := f.service.EnqueueGroups(t.Context(), "41", f.input(t, "8"), "operator"); err != nil {
				t.Fatal(err)
			}
			f.runner.run(context.Background())
			if f.tasks.last.Status != "failed" {
				t.Fatalf("task=%+v", f.tasks.last)
			}
			catalog, err := f.repository.PricingCatalog(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(catalog.Accounts[0].GroupIDs, []string{"7"}) || !reflect.DeepEqual(f.remote, []int64{7}) {
				t.Fatalf("unconfirmed groups committed: %v remote=%v", catalog.Accounts[0].GroupIDs, f.remote)
			}
			if f.badReadback && f.writes != 2 {
				t.Fatalf("rollback not performed: %d", f.writes)
			}
		})
	}
}

func TestGroupLockBlocksManualSwitchBeforeQueueAndExecution(t *testing.T) {
	for _, queued := range []bool{false, true} {
		t.Run(fmt.Sprint(queued), func(t *testing.T) {
			f := newGroupsFixture(t)
			input := f.input(t, "8")
			if queued {
				if _, err := f.service.EnqueueGroups(t.Context(), "41", input, "test"); err != nil {
					t.Fatal(err)
				}
			}
			if err := f.repository.SetAccountGroupsLocked(t.Context(), "41", true, "test"); err != nil {
				t.Fatal(err)
			}
			if queued {
				f.runner.run(context.Background())
				if f.tasks.last.Status != "failed" || !strings.Contains(f.tasks.last.Message, "分组已锁定") {
					t.Fatalf("task=%+v", f.tasks.last)
				}
			} else {
				if _, err := f.service.EnqueueGroups(t.Context(), "41", input, "test"); err == nil || !strings.Contains(err.Error(), "分组已锁定") {
					t.Fatalf("err=%v", err)
				}
			}
			if f.requests.Load() != 0 {
				t.Fatalf("locked account reached upstream: %d", f.requests.Load())
			}
			if err := f.repository.SetAccountGroupsLocked(t.Context(), "41", false, "test"); err != nil {
				t.Fatal(err)
			}
			if _, err := f.service.EnqueueGroups(t.Context(), "41", f.input(t, "8"), "test"); err != nil {
				t.Fatal(err)
			}
			f.runner.run(context.Background())
			if f.tasks.last.Status != "succeeded" || f.writes != 1 {
				t.Fatalf("unlock failed: %+v writes=%d", f.tasks.last, f.writes)
			}
		})
	}
}
