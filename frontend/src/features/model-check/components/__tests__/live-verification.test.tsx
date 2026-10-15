import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import type { Task } from "@/api";
import { ModelCheckResult } from "../model-check-result";

const firstProbe = {
  id: "arithmetic",
  label: "多步计算",
  round: 1,
  request_id: "request-1",
  status: "passed",
  prompt: "计算 7 × 3",
  expected: "21",
  answer: "**21**",
  protocol: "openai_responses",
  http_status: 200,
  duration_ms: 250,
};
const nextProbe = {
  id: "constraints",
  label: "约束推理",
  round: 1,
  request_id: "request-2",
  status: "running",
  prompt: "计算约束题",
  expected: "42",
};
function runningTask(): Task {
  return {
    id: "live-task",
    skill: "sub2api-model-check",
    operation: "account-model-behavior-check",
    status: "running",
    progress: 11,
    message: "约束推理等待回复",
    created_at: "2026-10-15T00:00:00Z",
    updated_at: "2026-10-15T00:00:01Z",
    result: {
      phase: "testing",
      total: 2,
      completed: 0,
      tests: [],
      active: {
        "0": {
          account_id: "1",
          account_name: "测试账号",
          claimed_model: "test-model",
          completed_probes: 1,
          planned_probes: 7,
          probes: [firstProbe],
          current_probe: nextProbe,
          timeout_seconds: 45,
          probe_plan: [firstProbe, nextProbe, { id: "state", label: "代码状态推演", round: 1 }],
        },
      },
      queued: [{ account_id: "1", account_name: "测试账号", claimed_model: "next-model" }],
    },
  };
}

it("运行中显示当前题目、已返回答案和待执行项，展开后可查看题目", async () => {
  const client = new QueryClient();
  const view = render(
    <QueryClientProvider client={client}>
      <ModelCheckResult task={runningTask()} />
    </QueryClientProvider>,
  );
  const user = userEvent.setup();
  try {
    expect(screen.getByText("**21**", { exact: true })).toBeVisible();
    expect(screen.getByText("等待回复", { exact: true })).toBeVisible();
    expect(screen.getByText("代码状态推演", { exact: true })).toBeVisible();
    await user.click(screen.getByText("等待执行 · 1 个组合", { exact: true }));
    expect(screen.getByText("next-model", { exact: true })).toBeVisible();
    const details = screen.getByText("多步计算", { exact: true }).closest("details")!;
    const summary = details.querySelector("summary")!;
    await user.click(summary);
    expect(details).toHaveAttribute("open");
    expect(within(details).getByText("计算 7 × 3", { exact: true })).toBeVisible();
    expect(within(details).getByText(/预期：21/)).toBeVisible();
    expect(within(details).getByText(/HTTP 200/)).toBeVisible();
    expect(within(details).getByText(/250 ms/)).toBeVisible();
  } finally {
    view.unmount();
    client.clear();
  }
});

it("单项请求失败时立即展示原因并保留之前的回答和展开状态", () => {
  const client = new QueryClient();
  const task = runningTask();
  const view = render(
    <QueryClientProvider client={client}>
      <ModelCheckResult task={task} />
    </QueryClientProvider>,
  );
  try {
    const details = screen.getByText("多步计算", { exact: true }).closest("details")!;
    details.open = true;
    const active = {
      account_id: "1",
      account_name: "测试账号",
      claimed_model: "test-model",
      completed_probes: 2,
      planned_probes: 7,
      probes: [
        firstProbe,
        {
          ...nextProbe,
          status: "error",
          error: "请求超时，请检查接口",
          error_code: "timeout",
          duration_ms: 45000,
        },
      ],
    };
    view.rerender(
      <QueryClientProvider client={client}>
        <ModelCheckResult task={{ ...task, result: { ...task.result, active: { "0": active } } }} />
      </QueryClientProvider>,
    );
    expect(screen.getByText("请求超时，请检查接口", { exact: true })).toBeVisible();
    expect(screen.getByText("请求失败", { exact: true })).toBeVisible();
    expect(screen.getByText("**21**", { exact: true })).toBeVisible();
    expect(details).toHaveAttribute("open");
  } finally {
    view.unmount();
    client.clear();
  }
});

it("旧任务没有逐项数据时明确提示等待进度，不虚构检测题目", () => {
  const client = new QueryClient();
  const task = runningTask();
  const view = render(
    <QueryClientProvider client={client}>
      <ModelCheckResult task={{ ...task, result: { phase: "credentials", tests: [], total: 1 } }} />
    </QueryClientProvider>,
  );
  try {
    expect(
      screen.getByText("正在准备账号凭据，单项检测开始后将在此显示。", { exact: true }),
    ).toBeVisible();
    expect(screen.queryByText("多步计算", { exact: true })).not.toBeInTheDocument();
  } finally {
    view.unmount();
    client.clear();
  }
});
