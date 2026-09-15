import { expect, it } from "vitest";
import type { AnimationResult, Task } from "@/api";
import { collectAnimationTasks } from "../animation-task-results";

function task(precheck: boolean, status: Task["status"] = "succeeded"): Task {
  const result: AnimationResult = {
    account_id: "41",
    account_name: "测试账号",
    model: "gpt-6-astra",
    status: "succeeded",
    request_id: "r",
    completed_at: "2026-09-15T00:00:00Z",
    duration_ms: 1,
  };
  if (precheck)
    result.precheck = {
      verdict: "passed",
      profile_version: "astra-v1",
      questions: [{ id: "candy", verdict: "passed", answer: "21", request_id: "r-candy" }],
    };
  else result.svg = "<svg />";
  return {
    id: precheck ? "precheck" : "animation",
    skill: "sub2api-model-animation",
    operation: precheck ? "account-model-precheck" : "account-model-animation",
    status,
    progress: 100,
    message: "完成",
    result: { account_ids: ["41"], animations: status === "running" ? [] : [result] },
    created_at: "2026-09-15T00:00:00Z",
    updated_at: "2026-09-15T00:01:00Z",
  };
}

it("同一账号完成前置检测后保留已有动画并单独返回前置结果", () => {
  const state = collectAnimationTasks([task(false), task(true)], new Set());
  expect(state.results.get("41")?.svg).toBe("<svg />");
  expect(state.precheckResults.get("41")?.precheck?.verdict).toBe("passed");
});

it("前置检测运行时锁定账号并显示前置活动，不覆盖动画完成状态", () => {
  const state = collectAnimationTasks([task(false), task(true, "running")], new Set());
  expect(state.busyIDs.has("41")).toBe(true);
  expect(state.activities.get("41")?.mode).toBe("precheck");
  expect(state.statuses.get("41")).toBe("succeeded");
  expect(state.precheckStatuses.get("41")).toBe("running");
});

it("新一轮前置检测取消且没有结果时清除上一轮通过结果，保留动画", () => {
  const cancelled = task(true, "cancelled");
  cancelled.id = "cancelled-precheck";
  cancelled.created_at = "2026-09-15T00:02:00Z";
  cancelled.result.animations = [];
  const state = collectAnimationTasks([task(false), task(true), cancelled], new Set());
  expect(state.precheckResults.has("41")).toBe(false);
  expect(state.precheckStatuses.get("41")).toBe("cancelled");
  expect(state.results.get("41")?.svg).toBe("<svg />");
});
