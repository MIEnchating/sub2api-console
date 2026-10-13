import { render, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import type { Task } from "@/api";
import { AccountModelAutomationResults } from "../account-model-automation-results";

it.each(["succeeded", "partial", "failed", "cancelled"] as const)(
  "%s 运营结果独立展示时保留唯一摘要和具体错误",
  (status) => {
    const task: Task = {
      id: "automation",
      skill: "console",
      operation: "account-model-automation",
      status,
      progress: 100,
      message: "本次账号模型处理已结束",
      result: { result_version: 2, items: [], pricing_error: "参考价读取失败" },
      created_at: "2026-10-12T00:00:00Z",
      updated_at: "2026-10-12T00:00:00Z",
    };
    render(<AccountModelAutomationResults task={task} />);
    expect(screen.getByText(task.message)).toBeVisible();
    expect(screen.getByText("价格同步失败：参考价读取失败")).toBeVisible();
    expect(screen.getByText("本次没有渠道上架记录。")).toBeVisible();
  },
);
