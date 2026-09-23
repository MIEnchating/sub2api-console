import { expect, it } from "vitest";
import type { AnimationResult, Task } from "@/api";
import { collectAnimationTasks } from "../animation-task-results";

const result: AnimationResult = {
  account_id: "41",
  account_name: "测试账号",
  model: "test-model",
  request_id: "request-1",
  status: "succeeded",
  svg: "<svg />",
  duration_ms: 1200,
  completed_at: "2026-09-13T00:00:00Z",
};

function task(animations: unknown[]): Task {
  return {
    id: "animation-1",
    skill: "animation",
    operation: "animation-check",
    status: "succeeded",
    progress: 100,
    message: "完成",
    result: { animations },
    created_at: "2026-09-13T00:00:00Z",
    updated_at: "2026-09-13T00:01:00Z",
  };
}

it.each([
  { completed_at: "invalid" },
  { completed_at: undefined },
  { account_name: { name: "账号" } },
  { duration_ms: "1200" },
  { duration_ms: -1 },
  { svg: {} },
  { error: {} },
  { retry_count: -1 },
  { retry_count: 1.5 },
])("动画结果含无效展示字段 %j 时忽略该记录并保留已有结果", (invalid) => {
  const state = collectAnimationTasks(
    [task([result, { ...result, ...invalid, request_id: "invalid-result" }])],
    new Set(),
  );
  expect(state.results.get("41")).toEqual({ ...result, source_task_id: "animation-1" });
});

it("同一账号返回更新的有效动画时展示最新结果", () => {
  const latest = { ...result, request_id: "request-2", completed_at: "2026-09-13T00:02:00Z" };
  const state = collectAnimationTasks([task([latest, result])], new Set());
  expect(state.results.get("41")).toEqual({ ...latest, source_task_id: "animation-1" });
});

it("动画结果返回自动重试次数时保留到详情数据", () => {
  const retried = { ...result, retry_count: 2 };
  expect(collectAnimationTasks([task([retried])], new Set()).results.get("41")).toEqual({
    ...retried,
    source_task_id: "animation-1",
  });
});

it("批量任务中个别结果无效时继续锁定全部账号并等待有效结果", () => {
  const running = task([result, { ...result, account_id: "42", completed_at: "invalid" }]);
  running.status = "running";
  running.result.account_ids = ["41", "42"];
  const state = collectAnimationTasks([running], new Set());
  expect(state.busyIDs).toEqual(new Set(["41", "42"]));
  expect(state.activities.get("42")).toEqual({
    status: "running",
    taskID: running.id,
    batchSize: 2,
    completed: 1,
  });
});

it("结果声称的来源任务 ID 不可信时使用服务器任务本身的 ID", () => {
  const state = collectAnimationTasks([task([{ ...result, source_task_id: "forged" }])], new Set());
  expect(state.results.get("41")?.source_task_id).toBe("animation-1");
});

it.each(["succeeded", "failed"] as const)(
  "批量任务仍运行但账号已有 %s 结果时，账号显示最终状态并保留批次占用",
  (status) => {
    const running = task([{ ...result, status }]);
    running.status = "running";
    running.result.account_ids = ["41", "42"];
    const state = collectAnimationTasks([running], new Set());
    expect(state.statuses.get("41")).toBe(status);
    expect(state.activities.has("41")).toBe(false);
    expect(state.statuses.get("42")).toBe("running");
    expect(state.busyIDs).toEqual(new Set(["41", "42"]));
  },
);
