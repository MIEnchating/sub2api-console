import { QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { Task } from "@/api";
import { createConsoleQueryClient } from "@/lib/query-client";
import { TerminalContinuityPanel } from "../terminal-continuity-panel";

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const completed: Task = {
  id: "terminal-history",
  skill: "sub2api-terminal-continuity",
  operation: "account-terminal-continuity",
  status: "succeeded",
  progress: 100,
  message: "完成",
  created_at: "2026-09-22T00:00:00Z",
  updated_at: "2026-09-22T00:01:00Z",
  result: {
    account_ids: ["41", "42"],
    checks: [
      {
        account_id: "41",
        account_name: "正常账号",
        model: "gpt-6-astra",
        request_id: "normal-request",
        verdict: "normal",
        response: '{"tool":"exec_command","command":"git status --short"}',
        duration_ms: 100,
        completed_at: "2026-09-22T00:01:00Z",
      },
      {
        account_id: "42",
        account_name: "异常账号",
        model: "gpt-6-astra",
        request_id: "suspected-request",
        verdict: "suspected",
        response: "当前会话没有可调用的 exec、git 或文件系统工具入口。",
        duration_ms: 200,
        completed_at: "2026-09-22T00:01:00Z",
      },
    ],
  },
};

function setup(): ReturnType<typeof createConsoleQueryClient> {
  const client = createConsoleQueryClient();
  client.setDefaultOptions({ queries: { retry: false, staleTime: Infinity } });
  client.setQueryData(
    ["accounts"],
    [
      { id: "41", name: "正常账号", groups: [], platform: "openai" },
      { id: "42", name: "异常账号", groups: [], platform: "openai" },
    ],
  );
  client.setQueryData(["terminal-continuity", "history"], [completed]);
  render(
    <QueryClientProvider client={client}>
      <TerminalContinuityPanel />
    </QueryClientProvider>,
  );
  return client;
}

it("独立展示正常与疑似账号，并可查看触发异常的模型原话", async () => {
  const user = userEvent.setup();
  const client = setup();
  expect(screen.getByText("正常", { exact: true })).toBeVisible();
  expect(screen.getByText("疑似无终端权限", { exact: true })).toBeVisible();
  await user.click(screen.getByRole("button", { name: "查看 异常账号 的终端续接详情" }));
  const dialog = screen.getByRole("dialog", { name: "终端续接检测详情" });
  expect(within(dialog).getByText(/没有可调用的 exec/)).toBeVisible();
  expect(within(dialog).getByText("suspected-request")).toBeVisible();
  client.clear();
});

it("选择疑似异常账号后按稳定 ID 和当前模型创建独立检测任务", async () => {
  const user = userEvent.setup();
  const requests: unknown[] = [];
  const queued: Task = {
    ...completed,
    id: "terminal-new",
    status: "queued",
    progress: 0,
    result: { account_ids: ["42"], checks: [] },
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "POST") {
        requests.push(JSON.parse(String(init.body)));
        return Response.json(queued);
      }
      if (String(input).includes("/api/tasks/")) return Response.json(queued);
      return Response.json([completed]);
    }),
  );
  const client = setup();
  await user.click(screen.getByRole("button", { name: "选择疑似异常（1）" }));
  expect(screen.getByRole("checkbox", { name: "检测终端续接 异常账号" })).toBeChecked();
  expect(screen.getByRole("checkbox", { name: "检测终端续接 正常账号" })).not.toBeChecked();
  await user.type(screen.getByRole("combobox", { name: "检测模型" }), "new-model");
  await user.keyboard("{Escape}");
  fireEvent.change(screen.getByRole("spinbutton", { name: "终端检测轮数" }), {
    target: { value: "3" },
  });
  await user.click(screen.getByRole("button", { name: "开始检测（1 个账号）" }));
  const confirm = screen.getByRole("dialog", { name: "确认终端续接检测范围" });
  expect(confirm).toHaveTextContent("结果只用于定位无依据否认终端工具的模型回答");
  await user.click(within(confirm).getByRole("button", { name: "确认并开始检测" }));
  await waitFor(() =>
    expect(requests).toEqual([
      {
        targets: [{ account_id: "42", model: "new-model" }],
        timeout_seconds: 120,
        rounds: 3,
      },
    ]),
  );
  expect(screen.getByRole("button", { name: "取消任务" })).toBeEnabled();
  client.clear();
});
