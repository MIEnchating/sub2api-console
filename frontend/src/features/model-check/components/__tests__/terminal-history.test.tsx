import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { Task } from "@/api";
import { DetectionTaskDetails } from "../detection-task-details";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
});

const round = {
  round: 1,
  verdict: "normal",
  request_id: "round-1",
  response: "历史回答",
  duration_ms: 1200,
  completed_at: "2026-09-21T00:00:00Z",
};

function setup(rounds?: unknown): void {
  const client = new QueryClient({
    defaultOptions: { queries: { staleTime: Infinity, retry: false } },
  });
  clients.push(client);
  const task: Task = {
    id: "terminal-history",
    skill: "console",
    operation: "account-terminal-continuity",
    status: "succeeded",
    progress: 100,
    message: "历史检测已完成",
    created_at: round.completed_at,
    updated_at: round.completed_at,
    result: {
      account_ids: ["41"],
      checks: [
        {
          account_id: "41",
          account_name: "历史账号",
          model: "historical-model",
          request_id: "history-41",
          verdict: "normal",
          duration_ms: 1200,
          completed_at: round.completed_at,
          round_results: rounds,
        },
      ],
    },
  };
  client.setQueryData(["task", task.id], task);
  client.setQueryData(["groups"], []);
  render(
    <QueryClientProvider client={client}>
      <DetectionTaskDetails id={task.id} onClose={vi.fn()} />
    </QueryClientProvider>,
  );
}

it("历史逐轮混有空值或无效结论时保留有效轮次与账号结论，并提示部分记录不可读取", () => {
  setup([null, { ...round, verdict: "unknown" }, round]);
  expect(screen.getByRole("group", { name: "终端检测 · 正常" })).toBeVisible();
  const results = screen.getByRole("region", { name: "逐轮检测结果" });
  expect(within(results).getByText(/第 1 轮 · 正常/)).toBeVisible();
  expect(within(results).getByText("部分逐轮结果格式无效，已保留可读取的结果。")).toBeVisible();
});

it.each([
  { label: "字符串", rounds: "invalid" },
  { label: "非数组对象", rounds: { rounds: [] } },
  { label: "包含非文本回答的数组", rounds: [{ ...round, response: { broken: true } }] },
])("历史逐轮数据为 $label 时显示读取说明且不会使详情页崩溃", (scenario) => {
  setup(scenario.rounds);
  expect(screen.getByRole("group", { name: "终端检测 · 正常" })).toBeVisible();
  expect(screen.getByText("部分逐轮结果格式无效，已保留可读取的结果。")).toBeVisible();
  expect(screen.queryByText(/第 1 轮/)).not.toBeInTheDocument();
});

it.each([
  { label: "缺失", rounds: undefined },
  { label: "空数组", rounds: [] },
])("旧记录逐轮数据为 $label 时保留账号结论且不补造错误或轮次", (scenario) => {
  setup(scenario.rounds);
  expect(screen.getByRole("group", { name: "终端检测 · 正常" })).toBeVisible();
  expect(screen.queryByRole("region", { name: "逐轮检测结果" })).not.toBeInTheDocument();
});
