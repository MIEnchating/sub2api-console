package accountworkbench_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
)

type groupMoveProtection struct {
	value      business.AccountMutationProtection
	operations []business.AccountOperation
}

func (p *groupMoveProtection) AccountMutationProtection(context.Context, string) (business.AccountMutationProtection, error) {
	return p.value, nil
}
func (p *groupMoveProtection) RecordAccountOperation(_ context.Context, op business.AccountOperation) error {
	p.operations = append(p.operations, op)
	return nil
}

type groupMoveGate struct {
	taskrunner.TaskRunner
	before func()
}

func (g groupMoveGate) GoTask(id string, run func(context.Context)) error {
	g.before()
	return g.TaskRunner.GoTask(id, run)
}

func TestWorkbenchMoveGroupsRechecksQueuedScopeAndDoesNotReplayUncertainWrites(t *testing.T) {
	for _, scenario := range []string{"account-changed", "groups-locked", "manual-control", "excluded", "target-changed", "group-disabled", "readback-mismatch", "write-response-lost", "success-audit"} {
		t.Run(scenario, func(t *testing.T) {
			service, private := fixture(t, `{}`)
			runner, tasks := runTasks(t, service)
			protection := &groupMoveProtection{}
			service.UseAccountGroupProtection(protection)
			account := map[string]any{"id": 41, "platform": "openai", "type": "oauth", "group_ids": []any{7}}
			groupStatus := "active"
			writes := 0
			service.UseTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host != "isolated.invalid" {
					return nil, errors.New("unexpected host")
				}
				if strings.HasSuffix(r.URL.Path, "/admin/groups") {
					return upstreamResponse(map[string]any{"data": map[string]any{"items": []any{map[string]any{"id": 8, "platform": "openai", "status": groupStatus}}, "total": 1}}), nil
				}
				if r.Method == http.MethodPut {
					writes++
					if scenario == "write-response-lost" {
						return nil, errors.New("response lost")
					}
					if scenario != "readback-mismatch" {
						account["group_ids"] = []any{8}
					}
				}
				return upstreamResponse(map[string]any{"data": account}), nil
			}))
			preview, err := service.AccountGroups(t.Context(), "41")
			if err != nil {
				t.Fatal(err)
			}
			service.UseExecution(tasks, groupMoveGate{TaskRunner: runner, before: func() {
				switch scenario {
				case "account-changed":
					account["group_ids"] = []any{9}
				case "groups-locked":
					protection.value.GroupsLocked = true
				case "manual-control":
					protection.value.ManualPriority = true
				case "excluded":
					protection.value.UpstreamExcluded = true
				case "target-changed":
					if err := private.ConfigureTarget(t.Context(), "https://changed.invalid", "changed", 10); err != nil {
						t.Fatal(err)
					}
				case "group-disabled":
					groupStatus = "inactive"
				}
			}}, nil, t.TempDir())
			task, err := service.MoveAccountGroups(t.Context(), "41", accountworkbench.MoveGroupsInput{GroupIDs: []string{"8"}, ExpectedGroupIDs: preview.CurrentGroupIDs, TargetVersion: preview.TargetVersion, AccountVersion: preview.AccountVersion}, "operator")
			if err != nil {
				t.Fatal(err)
			}
			awaitRun(t, runner)
			result, err := tasks.Get(t.Context(), task.ID)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "success-audit" {
				if result.Status != "succeeded" || writes != 1 || len(protection.operations) != 1 || !protection.operations[0].ReadbackConfirmed || protection.operations[0].Actor != "operator" {
					t.Fatalf("result=%+v audits=%+v writes=%d", result, protection.operations, writes)
				}
				return
			}
			if result.Status != "failed" {
				t.Fatalf("unexpected task result: %+v", result)
			}
			if scenario == "readback-mismatch" || scenario == "write-response-lost" {
				if writes != 1 || result.Result["readback_confirmed"] != false || len(protection.operations) != 1 || protection.operations[0].State != "failed" {
					t.Fatalf("uncertain operation replayed or reported successful: %+v writes=%d audit=%+v", result, writes, protection.operations)
				}
			} else if writes != 0 {
				t.Fatalf("changed scope wrote remotely: %d", writes)
			}
		})
	}
}
