import { expect, it } from "vitest";
import type { AnimationResult, Task } from "@/api";
import { collectAnimationTasks } from "../animation-task-results";

const precheck: AnimationResult = {
  account_id: "41",
  account_name: "测试账号",
  model: "gpt-6-astra",
  mode: "precheck",
  status: "succeeded",
  request_id: "check-precheck",
  duration_ms: 10,
  completed_at: "2026-09-15T00:01:00Z",
  precheck: {
    verdict: "passed",
    profile_version: "astra-v1",
    questions: [{ id: "candy", verdict: "passed", answer: "21", request_id: "candy" }],
  },
};
const animation: AnimationResult = {
  ...precheck,
  mode: "animation",
  precheck: undefined,
  svg: "<svg />",
  request_id: "check-animation",
  completed_at: "2026-09-15T00:02:00Z",
};
function task(rows: AnimationResult[], status: Task["status"]): Task {
  return {
    id: "both",
    skill: "sub2api-model-animation",
    operation: "account-model-combined",
    status,
    progress: 50,
    message: "检测中",
    created_at: "2026-09-15T00:00:00Z",
    updated_at: "2026-09-15T00:02:00Z",
    result: { mode: "both", account_ids: ["41"], animations: rows },
  };
}

it("组合检测完成后同一账号的两类结果分别保留", () => {
  const state = collectAnimationTasks([task([precheck, animation], "succeeded")], new Set());
  expect(state.precheckResults.get("41")).toEqual(precheck);
  expect(state.results.get("41")?.svg).toBe("<svg />");
});

it("组合检测在前置完成后继续显示动画运行状态并锁定账号", () => {
  const state = collectAnimationTasks([task([precheck], "running")], new Set());
  expect(state.precheckResults.get("41")).toEqual(precheck);
  expect(state.activities.get("41")).toMatchObject({ status: "running" });
  expect(state.activities.get("41")?.mode).toBeUndefined();
  expect(state.busyIDs.has("41")).toBe(true);
});

it("组合检测尚未返回结果时显示前置检测阶段", () => {
  const state = collectAnimationTasks([task([], "running")], new Set());
  expect(state.activities.get("41")?.mode).toBe("precheck");
});
