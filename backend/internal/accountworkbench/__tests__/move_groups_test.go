package accountworkbench_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestWorkbenchMoveGroupsUsesRemoteDirectoryAndWritesOnlyGroups(t *testing.T) {
	service, _ := fixture(t, `{}`)
	runner, tasks := runTasks(t, service)
	current := map[string]any{"id": 41, "platform": "openai", "type": "oauth", "group_ids": []any{7}, "credentials": map[string]any{"access_token": "private-secret"}}
	writes := 0
	service.UseTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "isolated.invalid" {
			return nil, fmt.Errorf("unexpected host")
		}
		if strings.HasSuffix(r.URL.Path, "/admin/groups") {
			return upstreamResponse(map[string]any{"data": map[string]any{"items": []any{
				map[string]any{"id": 7, "name": "原组", "platform": "openai", "status": "active"},
				map[string]any{"id": 8, "name": "空目标组", "platform": "openai", "status": "active"},
				map[string]any{"id": 9, "name": "停用组", "platform": "openai", "status": "inactive"},
				map[string]any{"id": 10, "name": "其他平台", "platform": "anthropic", "status": "active"},
			}, "total": 4}}), nil
		}
		if r.URL.Path != "/api/v1/admin/accounts/41" {
			return nil, fmt.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method == http.MethodPut {
			var body map[string]any
			decoder := json.NewDecoder(r.Body)
			decoder.UseNumber()
			if err := decoder.Decode(&body); err != nil {
				return nil, err
			}
			if len(body) != 1 || fmt.Sprint(body["group_ids"]) != "[8]" {
				return nil, fmt.Errorf("unexpected write: %v", body)
			}
			writes++
			current["group_ids"] = body["group_ids"]
		}
		return upstreamResponse(map[string]any{"data": current}), nil
	}))
	preview, err := service.AccountGroups(t.Context(), "41")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Groups) != 2 || preview.Groups[1].ID != "8" || !slices.Equal(preview.CurrentGroupIDs, []string{"7"}) {
		t.Fatalf("preview=%+v", preview)
	}
	task, err := service.MoveAccountGroups(t.Context(), "41", accountworkbench.MoveGroupsInput{GroupIDs: []string{"8"}, ExpectedGroupIDs: preview.CurrentGroupIDs, TargetVersion: preview.TargetVersion, AccountVersion: preview.AccountVersion}, "test")
	if err != nil {
		t.Fatal(err)
	}
	awaitRun(t, runner)
	result, err := tasks.Get(t.Context(), task.ID)
	if err != nil || result.Status != "succeeded" || writes != 1 || result.Result["readback_confirmed"] != true {
		t.Fatalf("result=%+v writes=%d error=%v", result, writes, err)
	}
	raw, _ := json.Marshal([]any{preview, result})
	if strings.Contains(string(raw), "private-secret") {
		t.Fatal("credential exposed")
	}
}

func TestWorkbenchMoveGroupsRejectsChangedOrInvalidSelectionWithoutWriting(t *testing.T) {
	for _, scenario := range []string{"empty", "duplicate", "invalid-id", "unavailable", "unchanged", "stale-groups", "stale-account", "stale-target", "missing-current-groups", "wrong-account"} {
		t.Run(scenario, func(t *testing.T) {
			service, _ := fixture(t, `{}`)
			runTasks(t, service)
			account := map[string]any{"id": 41, "platform": "openai", "type": "oauth", "group_ids": []any{7}}
			service.UseTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method != http.MethodGet {
					t.Error("invalid move wrote upstream")
					return nil, fmt.Errorf("unexpected write")
				}
				if strings.HasSuffix(r.URL.Path, "/admin/groups") {
					return upstreamResponse(map[string]any{"data": map[string]any{"items": []any{map[string]any{"id": 8, "name": "目标", "platform": "openai", "status": "active"}}, "total": 1}}), nil
				}
				return upstreamResponse(map[string]any{"data": account}), nil
			}))
			preview, err := service.AccountGroups(t.Context(), "41")
			if err != nil {
				t.Fatal(err)
			}
			input := accountworkbench.MoveGroupsInput{GroupIDs: []string{"8"}, ExpectedGroupIDs: preview.CurrentGroupIDs, TargetVersion: preview.TargetVersion, AccountVersion: preview.AccountVersion}
			switch scenario {
			case "empty":
				input.GroupIDs = nil
			case "duplicate":
				input.GroupIDs = []string{"8", "8"}
			case "invalid-id":
				input.GroupIDs = []string{"08"}
			case "unavailable":
				input.GroupIDs = []string{"9"}
			case "unchanged":
				input.GroupIDs = []string{"7"}
			case "stale-groups":
				account["group_ids"] = []any{9}
			case "stale-account":
				account["name"] = "modified"
			case "stale-target":
				input.TargetVersion = "old"
			case "missing-current-groups":
				delete(account, "group_ids")
			case "wrong-account":
				account["id"] = 42
			}
			if _, err := service.MoveAccountGroups(t.Context(), "41", input, "test"); err == nil {
				t.Fatal("invalid move accepted")
			}
		})
	}
}
