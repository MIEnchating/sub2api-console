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

it("历史动画读取手动结论，重测的新结果不继承旧标记", () => {
  const marked = {
    ...result,
    manual_verdict: {
      verdict: "not_passed" as const,
      updated_at: "2026-10-08T01:00:00Z",
      updated_by: "operator",
    },
  };
  const previous = task([marked]);
  expect(collectAnimationTasks([previous], new Set()).results.get("41")?.manual_verdict).toEqual(
    marked.manual_verdict,
  );
  const next = {
    ...task([{ ...result, request_id: "retest-result", completed_at: "2026-10-08T02:00:00Z" }]),
    id: "retest",
    created_at: "2026-10-08T01:59:00Z",
  };
  const latest = collectAnimationTasks([previous, next], new Set()).results.get("41");
  expect(latest?.source_task_id).toBe("retest");
  expect(latest?.manual_verdict).toBeUndefined();
});

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

it("组合任务的旧结果缺少 mode 时仍按前置报告归类", () => {
  const precheck = {
    ...result,
    precheck: {
      verdict: "passed" as const,
      profile_version: "astra-v1",
      questions: [],
    },
    request_id: "precheck-41",
  };
  const animation = {
    ...result,
    svg: "<svg />",
    request_id: "animation-41",
  };
  const combined: Task = {
    ...task([precheck, animation]),
    operation: "managed-model-detection",
    result: {
      mode: "both",
      account_ids: ["41"],
      animations: [precheck, animation],
    },
  };

  const state = collectAnimationTasks([combined], new Set());

  expect(state.precheckResults.get("41")?.request_id).toBe("precheck-41");
  expect(state.results.get("41")?.request_id).toBe("animation-41");
});

it("终态任务中的旧 running 阶段结果仍展示为已记录", () => {
  const legacy: Task = {
    ...task([]),
    operation: "managed-model-detection",
    status: "partial",
    result: {
      mode: "precheck",
      account_ids: ["41"],
      animations: [
        {
          ...result,
          mode: "precheck",
          phase: "running",
          precheck: {
            verdict: "not_passed",
            profile_version: "astra-v2",
            questions: [{ id: "candy", verdict: "not_passed", request_id: "request-candy" }],
          },
        },
      ],
    },
  };

  expect(collectAnimationTasks([legacy], new Set()).precheckResults.get("41")?.phase).toBe(
    "running",
  );
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
  ["2026-09-13T00:02:00.123456790Z", "2026-09-13T00:02:00.123456789Z"],
  ["2026-09-13T00:02:00.120000001Z", "2026-09-13T00:02:00.12Z"],
  ["2026-09-13T01:02:00.123456790+01:00", "2026-09-13T00:02:00.123456789Z"],
])("结果在同一毫秒内完成时选择纳秒时间较新的 %s", (newer, older) => {
  const latest = { ...result, request_id: "latest", completed_at: newer };
  const previous = { ...result, request_id: "older", completed_at: older };
  expect(
    collectAnimationTasks([task([latest, previous])], new Set()).results.get("41")?.request_id,
  ).toBe("latest");
  const first = { ...task([latest]), id: "first" };
  const second = { ...task([previous]), id: "second", created_at: "2026-09-13T00:01:00Z" };
  expect(collectAnimationTasks([first, second], new Set()).results.get("41")?.request_id).toBe(
    "latest",
  );
});

it("同毫秒启动的新前置尝试无结果时仍清除旧通过结论", () => {
  const previous: Task = {
    ...task([{ ...result, mode: "precheck" }]),
    id: "z-previous",
    operation: "account-model-precheck",
    created_at: "2026-09-13T00:02:00.123456789Z",
  };
  const latest: Task = {
    ...task([]),
    id: "a-latest",
    operation: "account-model-precheck",
    status: "cancelled",
    created_at: "2026-09-13T00:02:00.123456790Z",
    result: { account_ids: ["41"] },
  };
  const state = collectAnimationTasks([latest, previous], new Set());
  expect(state.precheckResults.has("41")).toBe(false);
  expect(state.precheckStatuses.get("41")).toBe("cancelled");
});

it("完成与启动时间相同时按稳定任务ID选择结果而不依赖响应顺序", () => {
  const first = { ...task([{ ...result, request_id: "a" }]), id: "a-task" };
  const second = { ...task([{ ...result, request_id: "z" }]), id: "z-task" };
  expect(collectAnimationTasks([second, first], new Set()).results.get("41")?.request_id).toBe("z");
});

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
  expect(state.results.get("41")).toEqual({
    ...result,
    source_task_id: "animation-1",
    source_task_status: "succeeded",
  });
});

it("同一账号返回更新的有效动画时展示最新结果", () => {
  const latest = { ...result, request_id: "request-2", completed_at: "2026-09-13T00:02:00Z" };
  const state = collectAnimationTasks([task([latest, result])], new Set());
  expect(state.results.get("41")).toEqual({
    ...latest,
    source_task_id: "animation-1",
    source_task_status: "succeeded",
  });
});

it("动画结果返回自动重试次数时保留到详情数据", () => {
  const retried = { ...result, retry_count: 2 };
  expect(collectAnimationTasks([task([retried])], new Set()).results.get("41")).toEqual({
    ...retried,
    source_task_id: "animation-1",
    source_task_status: "succeeded",
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
    source_task_status: "succeeded",
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

it("失败动画历史记录保留被拒绝的原文和截断标记供详情展示", () => {
  const failed = {
    ...result,
    status: "failed",
    source: "<svg>未完成的生成内容",
    source_truncated: true,
    error: "生成的 SVG 超过 128 KB，请重试",
  };
  expect(collectAnimationTasks([task([failed])], new Set()).results.get("41")).toMatchObject({
    status: "failed",
    source: "<svg>未完成的生成内容",
    source_truncated: true,
  });
});
