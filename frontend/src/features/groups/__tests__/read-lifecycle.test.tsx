import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { GroupsPage } from "@/App";
import type { GroupStatus } from "@/api";
import { policy } from "@/features/accounts/__tests__/fixtures";
import { allocationFixture } from "../components/__tests__/allocation-fixture";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

const group: GroupStatus = {
  id: "6",
  name: "codex",
  account_count: 1,
  scheduling_open: 1,
  scheduling_closed: 0,
  scheduling_unknown: 0,
  strategy: "balanced",
  strategy_source: "global_default",
  participation_status: "participating",
  participation_reason: null,
  status: "healthy",
  override: null,
};
const models = {
  group_id: "6",
  group_name: "codex",
  models: ["current-model"],
  account_count: 1,
  accounts_with_models: 1,
  complete: true,
};

for (const target of [
  {
    path: "allocation",
    trigger: "查看分组账号调度状态",
    dialog: "codex 调度状态",
    result: allocationFixture(1),
  },
  { path: "models", trigger: "编辑分组", dialog: "编辑分组策略", result: models },
]) {
  function renderPage(): QueryClient {
    const client = new QueryClient({
      defaultOptions: { queries: { enabled: false, retry: false, staleTime: 15_000 } },
    });
    clients.push(client);
    client.setQueryData(["groups"], [group]);
    client.setQueryData(["policy"], policy);
    render(
      <QueryClientProvider client={client}>
        <GroupsPage />
      </QueryClientProvider>,
    );
    return client;
  }

  it(`${target.path}关闭弹窗取消未完成读取，不把晚到结果写入缓存`, async () => {
    let signal: AbortSignal | null | undefined;
    let finish!: (response: Response) => void;
    vi.stubGlobal(
      "fetch",
      vi.fn((input: string, init?: RequestInit) => {
        if (!input.endsWith(`/groups/6/${target.path}`)) return Promise.resolve(Response.json([]));
        signal = init?.signal;
        return new Promise<Response>((resolve) => {
          finish = resolve;
        });
      }),
    );
    const client = renderPage();
    fireEvent.click(screen.getByRole("button", { name: target.trigger }));
    await waitFor(() => expect(finish).toBeDefined());
    const dialog = within(screen.getByRole("dialog", { name: target.dialog }));
    if (target.path === "models") fireEvent.click(dialog.getByRole("button", { name: "取消" }));
    else fireEvent.click(dialog.getByRole("button", { name: "关闭" }));
    await act(async () => finish(Response.json(target.result)));
    expect(signal?.aborted).toBe(true);
    const key = target.path === "models" ? "group-probe-models" : "group-allocation";
    expect(client.getQueryData([key, "6"])).toBeUndefined();
  });

  it(`${target.path}短时间关闭重开仍刷新当前数据`, async () => {
    let reads = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn((input: string) => {
        if (input.endsWith(`/groups/6/${target.path}`)) reads++;
        return Promise.resolve(Response.json(target.result));
      }),
    );
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: target.trigger }));
    const key = target.path === "models" ? "group-probe-models" : "group-allocation";
    await waitFor(() => expect(clients[0].getQueryData([key, "6"])).toEqual(target.result));
    const dialog = within(screen.getByRole("dialog", { name: target.dialog }));
    if (target.path === "models") fireEvent.click(dialog.getByRole("button", { name: "取消" }));
    else fireEvent.click(dialog.getByRole("button", { name: "关闭" }));
    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: target.dialog })).not.toBeInTheDocument(),
    );
    fireEvent.click(screen.getByRole("button", { name: target.trigger }));
    await waitFor(() => expect(reads).toBe(2));
  });
}
