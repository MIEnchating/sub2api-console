import { seedDictionaries } from "@/test/dictionary-fixtures";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import type { Task, TaskSummary } from "@/api";
import { SystemInfoPage } from "../system-info-page";

const task: TaskSummary = {
  id: "probe-legacy",
  skill: "sub2api-connectivity-test",
  operation: "active-probe",
  status: "succeeded",
  progress: 100,
  message: "探活完成",
  created_at: "2026-09-05T00:00:00Z",
  updated_at: "2026-09-05T00:00:01Z",
  system_info: true,
};

let client: QueryClient;

afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

function renderPage(detail: Task): void {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  seedDictionaries(client);
  client.setQueryData(["tasks"], [task]);
  client.setQueryData(["task", task.id], detail);
  client.setQueryData(["system-metrics"], {
    cpu: { usage_percent: 20, logical_cores: 4 },
    memory: { usage_percent: 30, used_bytes: 1024, total_bytes: 4096 },
    disk: { usage_percent: 40, used_bytes: 1024, total_bytes: 4096 },
  });
  render(
    <QueryClientProvider client={client}>
      <SystemInfoPage />
    </QueryClientProvider>,
  );
}

function probeTask(accountName?: string): Task {
  return {
    ...task,
    result: {
      platform: "openai",
      model: "gpt-5.6-sol",
      results: [
        {
          account_id: "41",
          ...(accountName ? { account_name: accountName } : {}),
          result: "通过",
          request_model: "gpt-5.6-sol",
          actual_model: "gpt-5.6-sol",
          status_code: 200,
        },
      ],
    },
  };
}

it("系统信息首次打开且未查看旧任务时不读取完整账号列表", async () => {
  const fetchRequest = vi.fn(async () => new Response("[]"));
  vi.stubGlobal("fetch", fetchRequest);

  renderPage(probeTask("生产主账号"));
  await userEvent.setup().click(screen.getByRole("tab", { name: "历史任务 1" }));
  await userEvent.setup().click(screen.getByRole("button", { name: "查看任务" }));

  expect(screen.getByText("生产主账号")).toBeVisible();
  expect(fetchRequest).not.toHaveBeenCalledWith(
    expect.stringContaining("/api/accounts"),
    expect.anything(),
  );
});

it("缺少历史及当前名称时保留账号ID和结果，不读取全目录补造名称", async () => {
  const fetchRequest = vi.fn(async () => Response.json([]));
  vi.stubGlobal("fetch", fetchRequest);

  renderPage(probeTask());
  await userEvent.setup().click(screen.getByRole("tab", { name: "历史任务 1" }));
  await userEvent.setup().click(screen.getByRole("button", { name: "查看任务" }));

  expect(await screen.findByText("账号名称不可用")).toBeVisible();
  expect(screen.getByText("ID 41")).toBeVisible();
  expect(fetchRequest).not.toHaveBeenCalledWith(
    expect.stringContaining("/api/accounts"),
    expect.anything(),
  );
});

it("旧探活详情使用后端限定范围的展示名称，不再读取完整账号目录", async () => {
  const fetchRequest = vi.fn(async () => Response.json([]));
  vi.stubGlobal("fetch", fetchRequest);
  renderPage({ ...probeTask(), display_account_names: { "41": "当前目录名称" } });
  await userEvent.setup().click(screen.getByRole("tab", { name: "历史任务 1" }));
  await userEvent.setup().click(screen.getByRole("button", { name: "查看任务" }));
  expect(await screen.findByText("当前目录名称")).toBeVisible();
  expect(fetchRequest).not.toHaveBeenCalledWith(
    expect.stringContaining("/api/accounts"),
    expect.anything(),
  );
});

it("任务保存的历史名称优先于当前目录展示名称", async () => {
  renderPage({ ...probeTask("执行时名称"), display_account_names: { "41": "后来修改的名称" } });
  await userEvent.setup().click(screen.getByRole("tab", { name: "历史任务 1" }));
  await userEvent.setup().click(screen.getByRole("button", { name: "查看任务" }));
  expect(screen.getByText("执行时名称")).toBeVisible();
  expect(screen.queryByText("后来修改的名称")).not.toBeInTheDocument();
});
