import { expect, it } from "vitest";
import type { AnimationResult, Task } from "@/api";
import { collectAnimationTasks, selectAnimationTaskResult } from "../animation-task-results";
import { detectionAccountRows } from "../detection-task-results";

const first: AnimationResult = {
  account_id: "1",
  account_name: "账号一",
  model: "first",
  request_id: "r-first",
  status: "succeeded",
  phase: "completed",
  duration_ms: 5,
  completed_at: "2026-10-13T00:00:00Z",
};
const second: AnimationResult = {
  ...first,
  model: "second",
  request_id: "r-second",
  status: "failed",
  error: "模型不可用",
};
const task: Task = {
  id: "batch",
  skill: "sub2api-model-animation",
  operation: "account-model-animation",
  status: "partial",
  progress: 100,
  message: "完成",
  created_at: first.completed_at,
  updated_at: first.completed_at,
  result: {
    account_ids: ["1"],
    targets: [{ account_id: "1", model: "first", models: ["first", "second"] }],
    animations: [first, second],
  },
};

it("同账号的多个模型结果分别保留且人工标记可按请求读取", () => {
  const state = collectAnimationTasks([task], new Set());
  expect(state.modelResults.get("1")?.map((item) => item.model)).toEqual(["first", "second"]);
  expect(selectAnimationTaskResult(task, "1", "r-first").result?.model).toBe("first");
  expect(selectAnimationTaskResult(task, "1", "r-second").result?.model).toBe("second");
  expect(detectionAccountRows(task, state, new Map()).get("1")?.animations).toHaveLength(2);
});
it("一个模型已完成另一个尚未执行时账号仍显示忙碌状态", () => {
  const running = {
    ...task,
    status: "running" as const,
    result: { ...task.result, animations: [first] },
  };
  const state = collectAnimationTasks([running], new Set());
  expect(state.activities.get("1")?.taskID).toBe("batch");
  expect(state.busyIDs.has("1")).toBe(true);
  expect(state.modelResults.get("1")).toHaveLength(1);
});
it("只重测一个模型时保留另一个模型的结果", () => {
  const retry = {
    ...task,
    id: "retry",
    created_at: "2026-10-13T01:00:00Z",
    result: {
      ...task.result,
      targets: [{ account_id: "1", model: "first" }],
      animations: [{ ...first, request_id: "r-retry", completed_at: "2026-10-13T01:00:00Z" }],
    },
  };
  const state = collectAnimationTasks([task, retry], new Set());
  expect(state.modelResults.get("1")?.map((item) => item.request_id)).toEqual([
    "r-retry",
    "r-second",
  ]);
});
