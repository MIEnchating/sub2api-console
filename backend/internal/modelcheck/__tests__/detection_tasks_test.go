package modelcheck_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
)

func (c *catalog) LoadDetectionTasks(context.Context) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.plans), nil
}
func (c *catalog) SaveDetectionTasks(_ context.Context, raw []byte, _, _ string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failSave {
		return errors.New("storage failure")
	}
	c.plans = slices.Clone(raw)
	return nil
}
func (c *catalog) DetectionTaskAccountIDs(_ context.Context, groups []string) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ids := []string{}
	for _, group := range groups {
		members, ok := c.groupMembers[group]
		if !ok {
			return nil, errors.New("group missing")
		}
		for _, id := range members {
			if !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
	}
	return ids, nil
}
func (c *catalog) DetectionTaskScope(ctx context.Context, groups []string) (business.DetectionTaskScope, error) {
	ids, err := c.DetectionTaskAccountIDs(ctx, groups)
	if err != nil {
		return business.DetectionTaskScope{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	scope := business.DetectionTaskScope{AccountIDs: ids, GroupIDsByAccount: map[string][]string{}, GroupNamesByID: map[string]string{}}
	for _, group := range groups {
		scope.GroupNamesByID[group] = "分组 " + group
		for _, id := range c.groupMembers[group] {
			scope.GroupIDsByAccount[id] = append(scope.GroupIDsByAccount[id], group)
		}
	}
	return scope, nil
}
func managedTask() modelcheck.DetectionTask {
	return modelcheck.DetectionTask{Name: "每日组合检测", GroupIDs: []string{"7", "8"}, Model: "test-model", Precheck: true, Terminal: true, TerminalRounds: 3, ScheduleType: "daily", DailyTimes: []string{"09:00", "20:00"}, Timezone: "Asia/Shanghai", TimeoutSeconds: 5}
}

func TestDetectionTaskUsesLatestGroupMembersAndKeepsStageAndRoundResults(t *testing.T) {
	f := setup(t, 2, "openai", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input string `json:"input"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		response := fixtureSVG
		if strings.Contains(body.Input, "exec_command") {
			response = `{"tool":"exec_command","command":"git status --short"}`
		} else if strings.Contains(body.Input, "糖") {
			response = "21"
		}
		fmt.Fprintf(w, `{"output_text":%q}`, response)
	})
	f.catalog.groupMembers = map[string][]string{"7": {"1"}, "8": {"1"}}
	plans, err := f.service.SaveDetectionTask(context.Background(), managedTask(), "test")
	if err != nil {
		t.Fatal(err)
	}
	runner := &deferredRunner{}
	f.service.UseTaskRunner(runner)
	if _, err := f.service.RunDetectionTask(context.Background(), plans[0].ID, plans[0].Version); err != nil {
		t.Fatal(err)
	}
	f.catalog.groupMembers = map[string][]string{"7": {"2"}, "8": {"2"}}
	runner.runs[0](context.Background())
	task := finished(t, f)
	if task.Status != "succeeded" || task.Result["total"] != 3 {
		t.Fatalf("bad result: %#v", task)
	}
	if ids := task.Result["account_ids"].([]string); fmt.Sprint(ids) != "[2]" {
		t.Fatalf("stale or duplicate membership: %v", ids)
	}
	if got := task.Result["group_ids_by_account"].(map[string][]string)["2"]; fmt.Sprint(got) != "[7 8]" {
		t.Fatalf("missing group snapshot: %v", got)
	}
	if task.Result["group_names_by_id"].(map[string]string)["7"] != "分组 7" {
		t.Fatal("group label missing")
	}
	if task.Result["account_names_by_id"].(map[string]string)["2"] != "测试账号 2" {
		t.Fatal("pending account name missing")
	}
	if len(task.Result["animations"].([]modelcheck.AnimationResult)) != 2 || len(task.Result["checks"].([]modelcheck.TerminalContinuityResult)[0].RoundResults) != 3 {
		t.Fatal("stage results missing")
	}
	if statuses, err := f.service.AccountStatuses(context.Background()); err != nil || len(statuses) != 0 {
		t.Fatal("task changed behavior evidence")
	}
	history, err := f.service.TerminalContinuityHistory(context.Background())
	if err != nil || len(history) != 1 || history[0].ID != task.ID {
		t.Fatalf("managed terminal results missing from terminal tab history: %v %v", history, err)
	}
}

func TestDetectionTaskVersionStorageRestartAndAutomaticTiming(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("saving generated") })
	f.catalog.groupMembers = map[string][]string{"7": {"1"}, "8": {"1"}}
	value := managedTask()
	value.Automatic = true
	plans, err := f.service.SaveDetectionTask(context.Background(), value, "test")
	if err != nil {
		t.Fatal(err)
	}
	value = plans[0].DetectionTask
	value.Version--
	if _, err := f.service.SaveDetectionTask(context.Background(), value, "test"); err == nil {
		t.Fatal("stale version accepted")
	}
	value = plans[0].DetectionTask
	value.Name = "changed"
	f.catalog.failSave = true
	if _, err := f.service.SaveDetectionTask(context.Background(), value, "test"); err == nil {
		t.Fatal("storage failure ignored")
	}
	f.catalog.failSave = false
	restarted, err := modelcheck.New(f.tasks, credentials{}, f.catalog, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	if restarted.DetectionTasks()[0].Name != managedTask().Name {
		t.Fatal("failed write changed saved task")
	}
	runner := &deferredRunner{}
	restarted.UseTaskRunner(runner)
	next, _ := time.Parse(time.RFC3339Nano, restarted.DetectionTasks()[0].NextAt)
	restarted.RunDueDetectionTasks(context.Background(), next.Add(-time.Second))
	if len(runner.runs) != 0 {
		t.Fatal("ran early")
	}
	restarted.RunDueDetectionTasks(context.Background(), next)
	restarted.RunDueDetectionTasks(context.Background(), next)
	if len(runner.runs) != 1 {
		t.Fatal("missing or overlapping automatic task")
	}
	if _, err := restarted.DeleteDetectionTask(context.Background(), value.ID, value.Version, "test"); err == nil {
		t.Fatal("deleted running task")
	}
}
