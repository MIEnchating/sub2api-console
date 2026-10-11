import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import type { Task } from "@/api";

import { ModelCheckResult } from "../model-check-result";

function interruptedTask(status: "cancelled" | "failed", count: number): Task {
  return {
    id: "interrupted-check",
    skill: "sub2api-model-check",
    operation: "account-model-behavior-check",
    status,
    progress: 100,
    message: status === "cancelled" ? "账号模型检测已取消" : "检测连接中断，请重新提交",
    created_at: "2026-10-10T00:00:00Z",
    updated_at: "2026-10-10T00:00:01Z",
    result: {
      completed: count,
      total: count + 2,
      tests: Array.from({ length: count }, (_, index) => ({
        account_id: String(index + 1),
        account_name: `已完成账号 ${index + 1}`,
        claimed_model: "gpt-6-astra",
        checker: "astra",
        verdict: "MATCH",
        requests: { successful: 3, total: 3 },
      })),
    },
  };
}

describe("普通模型检测中断结果", () => {
  it.each(["cancelled", "failed"] as const)("%s 任务保留已完成明细并展示真实终态", (status) => {
    render(<ModelCheckResult task={interruptedTask(status, 1)} />);

    expect(screen.getByText(status === "cancelled" ? "检测已取消" : "检测失败")).toBeVisible();
    expect(within(screen.getByRole("table")).getByText("已完成账号 1")).toBeInTheDocument();
    expect(within(screen.getByRole("table")).getByText("匹配")).toBeInTheDocument();
    expect(screen.queryByText("检测完成")).not.toBeInTheDocument();
    expect(screen.queryByRole("progressbar", { name: /检测进度/ })).not.toBeInTheDocument();
    if (status === "failed") {
      expect(screen.getByText("检测连接中断，请重新提交")).toBeVisible();
    }
  });

  it.each(["cancelled", "failed"] as const)("%s 任务没有结果时仍展示终态而非空明细表", (status) => {
    render(<ModelCheckResult task={interruptedTask(status, 0)} />);

    expect(screen.getByText(status === "cancelled" ? "检测已取消" : "检测失败")).toBeVisible();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
    expect(screen.queryByText("任务未返回检测明细")).not.toBeInTheDocument();
  });

  it("取消任务的多项已完成结果可通过分页继续查看", async () => {
    const user = userEvent.setup();
    render(<ModelCheckResult task={interruptedTask("cancelled", 21)} />);

    expect(within(screen.getByRole("table")).getByText("已完成账号 20")).toBeInTheDocument();
    expect(screen.queryByText("已完成账号 21")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "转到第 2 页" }));

    expect(within(screen.getByRole("table")).getByText("已完成账号 21")).toBeInTheDocument();
    expect(screen.getByText("检测已取消")).toBeVisible();
  });
});
