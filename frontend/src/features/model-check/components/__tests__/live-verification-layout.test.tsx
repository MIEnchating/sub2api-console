import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import type { Task } from "@/api";
import { ModelCheckResult } from "../model-check-result";

function task(): Task {
  return {
    id: "layout-task",
    skill: "sub2api-model-check",
    operation: "account-model-behavior-check",
    status: "running",
    progress: 20,
    message: "正在执行检测",
    created_at: "",
    updated_at: "",
    result: {
      total: 2,
      completed: 0,
      phase: "testing",
      tests: [],
      active: {
        first: {
          account_id: "1",
          account_name: "账号甲",
          claimed_model: "model-a",
          planned_probes: 3,
          probes: [
            {
              id: "math",
              label: "计算题",
              round: 1,
              status: "passed",
              answer: "21",
              prompt: "7 × 3",
              protocol: "openai_responses",
              http_status: 200,
              duration_ms: 100,
            },
          ],
          current_probe: {
            id: "fingerprint",
            label: "中文采样",
            category: "fingerprint",
            round: 1,
            status: "running",
          },
        },
        second: {
          account_id: "2",
          account_name: "账号乙",
          claimed_model: "model-b",
          probes: [
            { id: "other", label: "第二账号题目", round: 1, status: "error", error: "请求超时" },
          ],
        },
      },
    },
  };
}

it("并行检测时只展开选中组合并按能力与指纹分组，切换不影响总进度与取消入口", async () => {
  const client = new QueryClient();
  const view = render(
    <QueryClientProvider client={client}>
      <ModelCheckResult task={task()} />
    </QueryClientProvider>,
  );
  try {
    const navigation = screen.getByRole("navigation", { name: "检测组合" });
    expect(navigation).toHaveClass("overflow-auto", "md:w-60");
    const first = within(navigation).getByRole("button", { name: /账号甲/ });
    expect(first).toHaveAttribute("aria-pressed", "true");
    const process = screen.getByRole("region", { name: "逐项检测过程" });
    expect(process).toHaveClass("min-h-0", "overflow-auto");
    expect(within(process).getByRole("heading", { name: "能力检测" })).toBeVisible();
    expect(within(process).getByRole("heading", { name: "分词指纹" })).toBeVisible();
    expect(within(process).queryByText("第二账号题目")).not.toBeInTheDocument();
    await userEvent.setup().click(within(navigation).getByRole("button", { name: /账号乙/ }));
    expect(within(process).getByText("请求超时")).toBeVisible();
    expect(first).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "20");
    expect(screen.getByRole("button", { name: "取消任务" })).toBeVisible();
  } finally {
    view.unmount();
    client.clear();
  }
});

it("默认仅显示回答摘要，展开检测项后才显示协议与请求明细", async () => {
  const client = new QueryClient();
  const view = render(
    <QueryClientProvider client={client}>
      <ModelCheckResult task={task()} />
    </QueryClientProvider>,
  );
  try {
    expect(screen.getByText("21", { exact: true })).toHaveClass("line-clamp-2");
    expect(screen.getByText(/HTTP 200/)).not.toBeVisible();
    await userEvent.setup().click(screen.getByText("计算题", { exact: true }));
    expect(screen.getByText(/HTTP 200/)).toBeVisible();
    expect(screen.getByText("7 × 3", { exact: true })).toBeVisible();
  } finally {
    view.unmount();
    client.clear();
  }
});

it("选中的组合结束后继续展示该组合，不被其他账号实时进度切换", async () => {
  const client = new QueryClient();
  const current = task();
  const view = render(
    <QueryClientProvider client={client}>
      <ModelCheckResult task={current} />
    </QueryClientProvider>,
  );
  try {
    await userEvent.setup().click(
      within(screen.getByRole("navigation", { name: "检测组合" })).getByRole("button", {
        name: /账号乙/,
      }),
    );
    view.rerender(
      <QueryClientProvider client={client}>
        <ModelCheckResult
          task={{
            ...current,
            result: {
              ...current.result,
              completed: 1,
              active: {
                first: { account_id: "1", account_name: "账号甲", claimed_model: "model-a" },
              },
              tests: [
                { account_id: "2", account_name: "账号乙", claimed_model: "model-b", probes: [] },
              ],
            },
          }}
        />
      </QueryClientProvider>,
    );
    const process = screen.getByRole("region", { name: "逐项检测过程" });
    expect(within(process).getByRole("heading", { name: "账号乙" })).toBeVisible();
    expect(within(process).getByText("已结束")).toBeVisible();
  } finally {
    view.unmount();
    client.clear();
  }
});
