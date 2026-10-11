import { expect, it } from "vitest";
import type { AnimationResult, Task } from "@/api";
import { selectAnimationTaskResult } from "../animation-task-results";

const row: AnimationResult = {
  account_id: "41",
  account_name: "生产账号",
  model: "test-model",
  request_id: "animation-41",
  status: "succeeded",
  duration_ms: 100,
  completed_at: "2026-10-09T00:00:00.123456789Z",
  manual_verdict: {
    verdict: "passed",
    updated_at: "2026-10-09T01:00:00Z",
    updated_by: "operator",
  },
};

function task(animations: unknown, overrides: Partial<Task> = {}): Task {
  return {
    id: "batch",
    skill: "sub2api-model-animation",
    operation: "account-model-combined",
    status: "succeeded",
    progress: 100,
    message: "完成",
    created_at: "2026-10-09T00:00:00Z",
    updated_at: "2026-10-09T01:00:00Z",
    result: { animations },
    ...overrides,
  };
}

it("同批多账号结果只返回目标账号，并使用任务自身的来源和状态", () => {
  const current = task([
    { ...row, account_id: "42", request_id: "animation-42", manual_verdict: undefined },
    { ...row, source_task_id: "untrusted", source_task_status: "running" },
  ]);
  expect(selectAnimationTaskResult(current, "41", "animation-41")).toEqual({
    status: "succeeded",
    result: { ...row, source_task_id: "batch", source_task_status: "succeeded" },
  });
});

it.each([undefined, null, {}, [], [null, "invalid", { ...row, account_id: "42" }]])(
  "结果集合 %j 没有目标账号时仍返回任务状态",
  (animations) => {
    expect(
      selectAnimationTaskResult(task(animations, { status: "running" }), "41", "animation-41"),
    ).toEqual({
      status: "running",
      result: undefined,
    });
  },
);

it("组合任务含旧前置报告与非法动画记录时保留最新有效动画", () => {
  const current = task([
    row,
    { ...row, completed_at: "invalid", request_id: "invalid-time" },
    { ...row, manual_verdict: { verdict: "invalid" }, request_id: "invalid-verdict" },
    { ...row, duration_ms: -1, request_id: "invalid-duration" },
    {
      ...row,
      request_id: "precheck-41",
      precheck: { verdict: "passed", profile_version: "v1", questions: [] },
    },
  ]);
  expect(selectAnimationTaskResult(current, "41", "animation-41").result?.manual_verdict).toEqual(
    row.manual_verdict,
  );
});

it("同一账号含纳秒不同的多次结果时只允许读取最新请求的标记", () => {
  const latest = {
    ...row,
    request_id: "latest",
    completed_at: "2026-10-09T00:00:00.123456790Z",
    manual_verdict: undefined,
  };
  const current = task([latest, row]);
  expect(selectAnimationTaskResult(current, "41", "latest").result).toMatchObject({
    request_id: "latest",
    manual_verdict: undefined,
  });
  expect(selectAnimationTaskResult(current, "41", "animation-41").result).toBeUndefined();
});

it("相同完成时间有多条有效动画时使用最后一条", () => {
  const latest = { ...row, request_id: "last" };
  expect(selectAnimationTaskResult(task([row, latest]), "41", "last").result?.request_id).toBe(
    "last",
  );
});

it.each(["queued", "running", "waiting_input"] as const)(
  "任务 %s 时不把生成中的记录当成完成结果",
  (status) => {
    expect(
      selectAnimationTaskResult(
        task([{ ...row, phase: "generating" }], { status }),
        "41",
        "animation-41",
      ),
    ).toEqual({ status, result: undefined });
  },
);

it.each(["succeeded", "partial", "failed", "cancelled"] as const)(
  "任务 %s 时仍兼容旧结果保留生成阶段的格式",
  (status) => {
    expect(
      selectAnimationTaskResult(
        task([{ ...row, phase: "generating" }], { status }),
        "41",
        "animation-41",
      ).result?.source_task_status,
    ).toBe(status);
  },
);

it.each([
  { operation: "account-model-precheck", result: { animations: [row] } },
  { operation: "animation", result: { animations: [row], mode: "precheck" } },
  {
    operation: "managed-model-detection",
    result: { animations: [row], configuration: { animation: false, precheck: true } },
  },
  {
    operation: "managed-model-detection",
    result: {
      animations: [row],
      configuration: { animation: false, precheck: false, terminal: true },
    },
  },
])("任务未执行动画时不返回动画标记：%j", (overrides) => {
  expect(
    selectAnimationTaskResult(task([], overrides), "41", "animation-41").result,
  ).toBeUndefined();
});

it("旧管理任务未配置动画开关时继续展示原动画标记", () => {
  const current = task([row], { operation: "managed-model-detection" });
  current.result.configuration = { precheck: false };
  expect(selectAnimationTaskResult(current, "41", "animation-41").result?.manual_verdict).toEqual(
    row.manual_verdict,
  );
});
