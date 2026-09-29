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

it("前置历史结果保留推理用量及长回答的截断标记", () => {
  const precheck = {
    ...result,
    mode: "precheck",
    usage: { reasoning_tokens: 832 },
    precheck: {
      verdict: "not_passed",
      profile_version: "astra-v1",
      questions: [
        {
          id: "candy",
          verdict: "not_passed",
          answer: "长回答",
          answer_truncated: true,
          request_id: "request-candy",
        },
      ],
    },
  };
  const history = { ...task([precheck]), operation: "account-model-precheck" };
  expect(collectAnimationTasks([history], new Set()).precheckResults.get("41")).toEqual(precheck);
});

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

it("HTML 历史结果保留代码、实际提示词、思考等级和用量", () => {
  const htmlResult = {
    ...result,
    svg: undefined,
    html: "<!DOCTYPE html><html><body><svg></svg></body></html>",
    source: "本次生成原文",
    prompt: "本次实际提示词",
    reasoning_effort: "low",
    generation_duration_ms: 1000,
    usage: { input_tokens: 100, output_tokens: 0, total_tokens: 100 },
  };
  expect(collectAnimationTasks([task([htmlResult])], new Set()).results.get("41")).toEqual({
    ...htmlResult,
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

it.each([
  ["queued", "queued"],
  ["running", "running"],
  ["generating", "generating"],
] as const)("阶段 %s 的动画结果显示为 %s 活动状态", (phase, status) => {
  const running = task([
    { ...result, phase, status: "failed", completed_at: "2026-09-13T00:00:01Z" },
  ]);
  running.status = "running";
  running.result.account_ids = ["41"];
  const state = collectAnimationTasks([running], new Set());
  expect(state.results.has("41")).toBe(false);
  expect(state.activities.get("41")?.status).toBe(status);
});

it("结果声称的来源任务 ID 不可信时使用服务器任务本身的 ID", () => {
  const state = collectAnimationTasks([task([{ ...result, source_task_id: "forged" }])], new Set());
  expect(state.results.get("41")?.source_task_id).toBe("animation-1");
});

it("只执行终端检测时保留已有动画和前置结果且不显示重新检测，账号仍被占用", () => {
  const previous = task([result, { ...result, mode: "precheck" }]);
  previous.result.mode = "both";
  const running: Task = {
    ...task([]),
    id: "terminal-only",
    operation: "managed-model-detection",
    status: "running",
    created_at: "2026-09-13T00:02:00Z",
    result: {
      account_ids: ["41"],
      configuration: { animation: false, precheck: false, terminal: true },
    },
  };
  const state = collectAnimationTasks([previous, running], new Set());
  expect(state.results.get("41")?.request_id).toBe("request-1");
  expect(state.precheckResults.get("41")?.request_id).toBe("request-1");
  expect(state.statuses.get("41")).toBe("succeeded");
  expect(state.precheckStatuses.get("41")).toBe("succeeded");
  expect(state.activities.has("41")).toBe(false);
  expect(state.busyIDs.has("41")).toBe(true);
});

it("只执行前置检测时只更新前置进度，已完成动画保留成功状态", () => {
  const running: Task = {
    ...task([]),
    id: "precheck-only",
    operation: "managed-model-detection",
    status: "running",
    created_at: "2026-09-13T00:02:00Z",
    result: {
      account_ids: ["41"],
      configuration: { animation: false, precheck: true, terminal: false },
    },
  };
  const state = collectAnimationTasks([task([result]), running], new Set());
  expect(state.statuses.get("41")).toBe("succeeded");
  expect(state.precheckStatuses.get("41")).toBe("running");
  expect(state.activities.get("41")?.mode).toBe("precheck");
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
