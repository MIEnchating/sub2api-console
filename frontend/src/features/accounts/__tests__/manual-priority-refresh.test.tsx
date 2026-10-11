import { seedDictionaries } from "@/test/dictionary-fixtures";
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";

import { AccountsPage } from "@/App";
import type { AccountDetail, AccountStatus, Task } from "@/api";
import { account, policy, task } from "./fixtures";

afterEach(() => vi.restoreAllMocks());

function setup(
  priority: number | null,
  next: number | null,
  options: {
    taskStatus?: Task["status"];
    detailFailure?: boolean;
    detailId?: string;
    deferDetail?: boolean;
  } = {},
) {
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.stubGlobal("scrollTo", vi.fn());
  const current: AccountDetail = {
    ...account,
    manual_priority: priority,
    health: priority === null ? "healthy" : "manual_priority",
    metadata: {},
    group_rates: {},
    group_ids: {},
    bindings: [],
    test_models: [],
  };
  const updated: AccountDetail = {
    ...current,
    id: options.detailId ?? current.id,
    manual_priority: next,
    priority: next ?? 50,
    load_factor: next === null ? "2" : "100",
    concurrency: next === null ? 10 : 100,
    health: next === null ? "healthy" : "manual_priority",
    routing_state: next === null ? "healthy" : "manual_priority",
  };
  const peer = { ...account, id: "42", name: "其他账号", manual_priority: null };
  let resolveList!: (response: Response) => void;
  const listResponse = new Promise<Response>((resolve) => {
    resolveList = resolve;
  });
  let resolveFresh!: (response: Response) => void;
  const freshResponse = new Promise<Response>((resolve) => {
    resolveFresh = resolve;
  });
  let resolveDetail!: (response: Response) => void;
  const detailResponse = new Promise<Response>((resolve) => {
    resolveDetail = resolve;
  });
  let listReads = 0;
  const completed = {
    ...task("account-manual-41", options.taskStatus),
    result: { account_id: "41", readback_confirmed: true, manual_priority: next },
  };
  const fetcher = vi.fn(async (input: RequestInfo | URL) => {
    const path = String(input);
    if (path === "/api/accounts") return ++listReads === 1 ? listResponse : freshResponse;
    if (path === "/api/accounts/41") {
      if (options.detailFailure)
        return Response.json({ error: "账号状态暂时不可用" }, { status: 503 });
      if (options.deferDetail) return detailResponse;
      return Response.json(updated);
    }
    if (path === "/api/accounts/41/manual-priority")
      return Response.json(task(completed.id, "queued"));
    if (path === `/api/tasks/${completed.id}`) return Response.json(completed);
    if (path === "/api/policy") return Response.json(policy);
    if (path.startsWith("/api/dictionaries")) return Response.json({ items: [] });
    return Response.json([]);
  });
  vi.stubGlobal("fetch", fetcher);
  const success = vi.spyOn(toast, "success");
  const error = vi.spyOn(toast, "error");
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  seedDictionaries(client);
  client.setQueryData(["accounts"], [current, peer]);
  client.setQueryData(["account-detail", "41"], current);
  client.setQueryData(["policy"], policy);
  client.setQueryData(["model-check-account-statuses"], []);
  for (const name of ["group", "platform", "account_type"])
    client.setQueryData(["dictionaries", name], { items: [] });
  const router = createRouter({
    routeTree: createRootRoute({ component: AccountsPage }),
    history: createMemoryHistory({ initialEntries: ["/"] }),
  });
  const view = render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  return {
    client,
    current,
    updated,
    peer,
    fetcher,
    success,
    error,
    resolveList,
    resolveDetail,
    resolveFresh,
    dispose: (): void => {
      view.unmount();
      client.clear();
      resolveList(Response.json([]));
      resolveFresh(Response.json([]));
      resolveDetail(Response.json(updated));
    },
  };
}

it.each([
  { action: "设置", before: null, after: 1 },
  { action: "调整", before: 1, after: 2 },
  { action: "取消", before: 1, after: null },
])("$action 手动控制成功后，列表刷新仍等待时立即更新该行参数和状态", async (scenario) => {
  const view = setup(scenario.before, scenario.after);
  const user = userEvent.setup();
  try {
    await user.click(await screen.findByRole("switch", { name: "显示手动控制账号" }));
    const row = screen.getByRole("row", { name: new RegExp(account.name) });
    await user.click(
      within(row).getByRole("button", {
        name: scenario.before === null ? "设置手动控制" : "调整手动控制",
      }),
    );
    const dialog = await screen.findByRole("dialog", { name: "手动控制" });
    if (scenario.after === null) {
      await user.click(within(dialog).getByRole("button", { name: "取消手动控制" }));
    } else {
      await user.click(within(dialog).getByRole("combobox", { name: "选择手动控制位置" }));
      await user.click(
        await screen.findByRole("option", { name: new RegExp(`^${scenario.after} .*可用`) }),
      );
      await user.click(
        within(dialog).getByRole("button", {
          name: scenario.before === null ? "设置手动控制" : "更新手动控制",
        }),
      );
    }
    await waitFor(() => expect(view.success).toHaveBeenCalled());
    if (scenario.before !== null)
      expect(within(row).queryByText(`手动控制 #${scenario.before}`)).not.toBeInTheDocument();
    expect(
      within(row).getByText(scenario.after === null ? "健康" : "手动控制", { exact: true }),
    ).toBeVisible();
    if (scenario.after !== null)
      expect(within(row).getByText(`手动控制 #${scenario.after}`)).toBeVisible();
    expect(view.client.getQueryData<AccountStatus[]>(["accounts"])?.[0]).toMatchObject({
      manual_priority: scenario.after,
      priority: view.updated.priority,
      load_factor: view.updated.load_factor,
      concurrency: view.updated.concurrency,
      health: view.updated.health,
    });
    expect(view.client.getQueryData<AccountStatus[]>(["accounts"])?.[1]).toEqual(view.peer);
    expect(view.client.getQueryData<AccountDetail>(["account-detail", "41"])).toEqual(view.updated);
  } finally {
    view.dispose();
  }
});

it("默认隐藏手动控制账号时，设置成功立即移出列表", async () => {
  const view = setup(null, 1);
  const user = userEvent.setup();
  try {
    const row = await screen.findByRole("row", { name: new RegExp(account.name) });
    await user.click(within(row).getByRole("button", { name: "设置手动控制" }));
    const dialog = await screen.findByRole("dialog", { name: "手动控制" });
    await user.click(within(dialog).getByRole("combobox", { name: "选择手动控制位置" }));
    await user.click(await screen.findByRole("option", { name: /^1 .*可用/ }));
    await user.click(within(dialog).getByRole("button", { name: "设置手动控制" }));
    await waitFor(() => expect(view.success).toHaveBeenCalled());
    expect(screen.queryByRole("row", { name: new RegExp(account.name) })).not.toBeInTheDocument();
    await user.click(screen.getByRole("switch", { name: "显示手动控制账号" }));
    expect(await screen.findByText("手动控制 #1")).toBeVisible();
  } finally {
    view.dispose();
  }
});

it.each(["failed", "cancelled"] as const)(
  "取消手动控制任务 %s 时保留原状态并恢复操作入口",
  async (status) => {
    const view = setup(1, null, { taskStatus: status });
    const user = userEvent.setup();
    try {
      await user.click(await screen.findByRole("switch", { name: "显示手动控制账号" }));
      const row = screen.getByRole("row", { name: new RegExp(account.name) });
      await user.click(within(row).getByRole("button", { name: "调整手动控制" }));
      const dialog = await screen.findByRole("dialog", { name: "手动控制" });
      await user.click(within(dialog).getByRole("button", { name: "取消手动控制" }));
      await waitFor(() =>
        expect(within(row).getByRole("button", { name: "调整手动控制" })).toBeEnabled(),
      );
      expect(within(row).getByText("手动控制 #1")).toBeVisible();
      expect(view.success).not.toHaveBeenCalled();
      expect(view.fetcher).not.toHaveBeenCalledWith("/api/accounts/41", expect.anything());
    } finally {
      view.dispose();
    }
  },
);

it("任务仍在执行时保留原状态且禁用重复操作", async () => {
  const view = setup(1, null, { taskStatus: "running" });
  const user = userEvent.setup();
  try {
    await user.click(await screen.findByRole("switch", { name: "显示手动控制账号" }));
    const row = screen.getByRole("row", { name: new RegExp(account.name) });
    await user.click(within(row).getByRole("button", { name: "调整手动控制" }));
    const dialog = await screen.findByRole("dialog", { name: "手动控制" });
    await user.click(within(dialog).getByRole("button", { name: "取消手动控制" }));
    await waitFor(() =>
      expect(
        view.client.getQueryData<Task>(["account-scheduling", "41", "account-manual-41"])?.status,
      ).toBe("running"),
    );
    expect(within(row).getByRole("button", { name: "调整手动控制" })).toBeDisabled();
    expect(within(row).getByText("手动控制 #1")).toBeVisible();
    expect(view.success).not.toHaveBeenCalled();
    expect(view.fetcher).not.toHaveBeenCalledWith("/api/accounts/41", expect.anything());
  } finally {
    view.dispose();
  }
});

it("任务成功但详情仍在读取时保持操作禁用，读取完成才显示新状态", async () => {
  const view = setup(1, null, { deferDetail: true });
  const user = userEvent.setup();
  try {
    await user.click(await screen.findByRole("switch", { name: "显示手动控制账号" }));
    const row = screen.getByRole("row", { name: new RegExp(account.name) });
    await user.click(within(row).getByRole("button", { name: "调整手动控制" }));
    const dialog = await screen.findByRole("dialog", { name: "手动控制" });
    await user.click(within(dialog).getByRole("button", { name: "取消手动控制" }));
    await waitFor(() =>
      expect(view.fetcher).toHaveBeenCalledWith("/api/accounts/41", expect.anything()),
    );
    expect(within(row).getByRole("button", { name: "调整手动控制" })).toBeDisabled();
    expect(view.success).not.toHaveBeenCalled();
    await act(async () => {
      view.resolveDetail(Response.json(view.updated));
    });
    await waitFor(() =>
      expect(within(row).getByRole("button", { name: "设置手动控制" })).toBeEnabled(),
    );
    expect(within(row).getByText("健康", { exact: true })).toBeVisible();
  } finally {
    view.dispose();
  }
});

it.each([
  { reason: "详情请求失败", options: { detailFailure: true } },
  { reason: "详情返回其他账号", options: { detailId: "99" } },
])("$reason 时保留原状态、提示刷新失败并回退列表刷新", async (scenario) => {
  const view = setup(1, null, scenario.options);
  const user = userEvent.setup();
  try {
    await user.click(await screen.findByRole("switch", { name: "显示手动控制账号" }));
    const row = screen.getByRole("row", { name: new RegExp(account.name) });
    await user.click(within(row).getByRole("button", { name: "调整手动控制" }));
    const dialog = await screen.findByRole("dialog", { name: "手动控制" });
    await user.click(within(dialog).getByRole("button", { name: "取消手动控制" }));
    await waitFor(() => expect(view.error).toHaveBeenCalled());
    expect(within(row).getByText("手动控制 #1")).toBeVisible();
    expect(view.client.getQueryData<AccountDetail>(["account-detail", "41"])?.id).toBe("41");
    await waitFor(() =>
      expect(view.fetcher).toHaveBeenCalledWith("/api/accounts", expect.anything()),
    );
    const recovered = { ...view.updated, id: "41" };
    await act(async () => {
      view.resolveList(Response.json([recovered, view.peer]));
    });
    await waitFor(() =>
      expect(within(row).getByRole("button", { name: "设置手动控制" })).toBeEnabled(),
    );
    expect(within(row).getByText("健康", { exact: true })).toBeVisible();
  } finally {
    view.dispose();
  }
});

it("取消成功后旧列表响应晚到，不恢复已清除的手动控制状态", async () => {
  const view = setup(1, null);
  const user = userEvent.setup();
  try {
    await user.click(await screen.findByRole("switch", { name: "显示手动控制账号" }));
    act(() => {
      void view.client.invalidateQueries({ queryKey: ["accounts"] });
    });
    await waitFor(() =>
      expect(view.fetcher).toHaveBeenCalledWith("/api/accounts", expect.anything()),
    );
    const row = screen.getByRole("row", { name: new RegExp(account.name) });
    await user.click(within(row).getByRole("button", { name: "调整手动控制" }));
    const dialog = await screen.findByRole("dialog", { name: "手动控制" });
    await user.click(within(dialog).getByRole("button", { name: "取消手动控制" }));
    await waitFor(() => expect(view.success).toHaveBeenCalled());
    await act(async () => {
      view.resolveList(Response.json([view.current]));
    });
    expect(within(row).queryByText("手动控制 #1")).not.toBeInTheDocument();
  } finally {
    view.dispose();
  }
});
