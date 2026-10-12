import { render, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { OnboardingTaskProgress } from "@/App";
import type { Task } from "@/api";

function task(status: Task["status"], message: string, result: Task["result"] = {}): Task {
  return {
    id: "onboard",
    skill: "console",
    operation: "onboard",
    status,
    progress: 100,
    message,
    result,
    created_at: "2026-10-12T00:00:00Z",
    updated_at: "2026-10-12T00:00:00Z",
  };
}

it("单项绑定取消时只显示一次取消原因", () => {
  render(<OnboardingTaskProgress task={task("cancelled", "用户已取消绑定")} />);
  expect(screen.getByText("用户已取消绑定")).toBeVisible();
});

it.each([{}, { error: "上游分组不可用" }])(
  "单项绑定失败且错误为 %j 时只显示一次失败原因",
  (result) => {
    render(<OnboardingTaskProgress task={task("failed", "上游分组不可用", result)} />);
    expect(screen.getByText("上游分组不可用")).toBeVisible();
  },
);

it("失败摘要与具体原因不同时保留两者及重试指引", () => {
  render(
    <OnboardingTaskProgress
      task={task("failed", "开户未完成", { error: "续办参数与首次冻结的开户意图不一致" })}
    />,
  );
  expect(screen.getByText("开户未完成")).toBeVisible();
  expect(screen.getByText("续办参数与首次冻结的开户意图不一致")).toBeVisible();
  expect(screen.getByText(/请按原参数重试/)).toBeVisible();
});
