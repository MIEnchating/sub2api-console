import { expect, it } from "vitest";
import type { Task } from "@/api";
import { detectionMissingResult, detectionTaskMessage } from "../detection-task-outcome";

function task(status: Task["status"], message: string, result: Task["result"] = {}): Task {
  return {
    id: "run",
    skill: "sub2api-model-animation",
    operation: "managed-model-detection",
    status,
    message,
    result,
    progress: 100,
    created_at: "2026-09-24T00:00:00Z",
    updated_at: "2026-09-24T00:30:00Z",
  };
}

it("旧任务仅在 message 保存超时错误时仍显示中文超时原因", () => {
  const history = task("failed", "任务执行失败：context deadline exceeded");
  expect(detectionMissingResult(history, false).label).toBe("任务超时，未取得结果");
  expect(detectionTaskMessage(history)).toContain("检测任务已超时结束");
});

it("单个账号超时但父任务部分完成时不把所有缺失结果归因于任务超时", () => {
  const history = task("partial", "已完成部分账号", {
    animations: [{ error: "context deadline exceeded" }],
  });
  expect(detectionMissingResult(history, false).label).toBe("结果未记录");
  expect(detectionTaskMessage(history)).toBe("已完成部分账号");
});

it("任务失败但没有错误信息时不生成空原因或猜测超时", () => {
  const history = task("failed", "", { error: { detail: "不可信结构" } });
  expect(detectionMissingResult(history, false)).toMatchObject({
    label: "任务失败，未取得结果",
    reason: "任务失败且未记录具体原因；没有结果的阶段无法确认是否执行，请重新检测。",
  });
});

it("运行中没有结果时保留等待状态且不显示任务终止说明", () => {
  expect(detectionMissingResult(task("running", "正在检测"), true)).toEqual({
    label: "等待检测结果",
    tone: "muted",
  });
});
