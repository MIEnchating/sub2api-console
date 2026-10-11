import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import type { Task } from "@/api";
import { OverviewPage } from "../overview-page";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

function renderOverview() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  clients.push(client);
  return render(
    <QueryClientProvider client={client}>
      <OverviewPage onOpenAccounts={() => {}} onOpenGroups={() => {}} onOpenEvents={() => {}} />
    </QueryClientProvider>,
  );
}

it("离开总览时取消未完成的账号、分组和最近事件读取", async () => {
  const signals = new Map<string, AbortSignal | null | undefined>();
  const paths = ["/api/accounts", "/api/groups", "/api/events"];
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const path = new URL(input, "http://localhost").pathname;
    if (!paths.includes(path)) return Promise.resolve(Response.json({ items: [] }));
    signals.set(path, init?.signal);
    return new Promise<Response>(() => {});
  });
  const view = renderOverview();
  await waitFor(() => expect(signals.size).toBe(3));
  view.unmount();
  for (const path of paths) expect(signals.get(path)?.aborted, path).toBe(true);
});

it("同步状态读取失败后可手动重试原任务，读取期间禁用重试且不重复创建同步", async () => {
  const task: Task = {
    id: "overview-sync-test",
    operation: "management-sync",
    skill: "console",
    status: "running",
    progress: 10,
    message: "正在同步",
    result: {},
    created_at: "2026-10-10T00:00:00Z",
    updated_at: "2026-10-10T00:00:00Z",
  };
  let recover = false;
  let starts = 0;
  let finish!: (response: Response) => void;
  vi.stubGlobal("fetch", (input: string) => {
    const path = new URL(input, "http://localhost").pathname;
    if (path === "/api/management/sync") {
      starts++;
      return Promise.resolve(Response.json(task));
    }
    if (path === `/api/tasks/${task.id}`) {
      if (!recover)
        return Promise.resolve(Response.json({ detail: "同步状态暂时不可用" }, { status: 503 }));
      return new Promise<Response>((resolve) => {
        finish = resolve;
      });
    }
    return Promise.resolve(Response.json(path.includes("dictionaries") ? { items: [] } : []));
  });
  renderOverview();
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "立即同步" }));
  const retry = await screen.findByRole("button", { name: "重新读取" });
  expect(screen.getByRole("button", { name: "同步中" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "取消任务" })).toBeEnabled();
  recover = true;
  await user.click(retry);
  await waitFor(() => expect(retry).toBeDisabled());
  await act(async () => finish(Response.json({ ...task, status: "succeeded", progress: 100 })));
  await waitFor(() => expect(screen.getByRole("button", { name: "立即同步" })).toBeEnabled());
  expect(screen.queryByRole("button", { name: "重新读取" })).not.toBeInTheDocument();
  expect(starts).toBe(1);
});
