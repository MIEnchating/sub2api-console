import { QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { Toaster, toast } from "sonner";
import { createConsoleQueryClient } from "@/lib/query-client";
import { MaintenancePanel } from "../components/maintenance-panel";
import { maintenanceKey, workbenchKeys } from "../constants";
import type { WorkbenchMaintenance } from "../types";
import { template } from "./template-fixture";

const settings: WorkbenchMaintenance = {
  revision: 3,
  enabled: false,
  interval_minutes: 5,
  cooldown_minutes: 10,
  check_after_repair: true,
  group_ids: [],
  running: false,
  task_id: "",
  last_check_at: null,
  next_check_at: null,
  message: "",
  results: [],
};
const clients: ReturnType<typeof createConsoleQueryClient>[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  toast.dismiss();
  vi.unstubAllGlobals();
});

async function mount(
  fetcher: typeof fetch,
  accountsLoaded = true,
): Promise<ReturnType<typeof createConsoleQueryClient>> {
  vi.stubGlobal("fetch", fetcher);
  vi.stubGlobal("PointerEvent", MouseEvent);
  const client = createConsoleQueryClient();
  client.setDefaultOptions({ queries: { retry: false, staleTime: Infinity } });
  client.setQueryData(maintenanceKey, settings);
  if (accountsLoaded) client.setQueryData(workbenchKeys.accounts, [template.summary]);
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <MaintenancePanel />
      <Toaster />
    </QueryClientProvider>,
  );
  await userEvent.setup().click(screen.getByRole("button", { name: "维护设置" }));
  return client;
}

it("未编辑维护设置时后台更新同步表单，立即检查提交可见配置版本", async () => {
  const fetcher = vi.fn(async (input: RequestInfo | URL) =>
    String(input).endsWith("/accounts")
      ? Response.json([template.summary])
      : Response.json({ ...settings, revision: 5, interval_minutes: 20, running: true }),
  );
  const client = await mount(fetcher);
  await act(async () => {
    client.setQueryData(maintenanceKey, {
      ...settings,
      revision: 4,
      interval_minutes: 20,
      enabled: true,
      group_ids: ["7"],
    });
  });
  await waitFor(() =>
    expect(screen.getByRole("spinbutton", { name: "检查间隔（分钟）" })).toHaveValue(20),
  );
  expect(screen.getByRole("checkbox", { name: "启用定时检查" })).toBeChecked();
  expect(screen.getByRole("combobox", { name: "维护分组" })).toHaveTextContent("团队");
  await userEvent.setup().click(screen.getByRole("button", { name: "立即检查" }));
  await waitFor(() =>
    expect(fetcher).toHaveBeenCalledWith(
      "/api/account-workbench/maintenance/check",
      expect.objectContaining({ body: JSON.stringify({ revision: 4 }) }),
    ),
  );
});

it("编辑中远端设置变化保留完整草稿，载入最新设置前阻止预览", async () => {
  const fetcher = vi.fn();
  const client = await mount(fetcher);
  const user = userEvent.setup();
  const interval = screen.getByRole("spinbutton", { name: "检查间隔（分钟）" });
  await user.clear(interval);
  await user.type(interval, "12");
  await act(async () => {
    client.setQueryData(maintenanceKey, {
      ...settings,
      revision: 4,
      interval_minutes: 20,
      cooldown_minutes: 30,
    });
  });
  expect(interval).toHaveValue(12);
  expect(screen.getByRole("spinbutton", { name: "失败冷却（分钟）" })).toHaveValue(10);
  await waitFor(() => expect(screen.getByRole("button", { name: "预览并保存" })).toBeDisabled());
  expect(screen.getByRole("button", { name: "立即检查" })).toBeDisabled();
  expect(await screen.findByText("维护设置已更新，请载入最新设置后重新编辑")).toBeVisible();
  await act(async () => {
    client.setQueryData(maintenanceKey, {
      ...settings,
      revision: 5,
      interval_minutes: 20,
      cooldown_minutes: 30,
      message: "状态更新",
    });
  });
  expect(screen.getAllByText("维护设置已更新，请载入最新设置后重新编辑")).toHaveLength(1);
  await user.click(screen.getByRole("button", { name: "载入最新设置" }));
  expect(interval).toHaveValue(20);
  expect(screen.getByRole("spinbutton", { name: "失败冷却（分钟）" })).toHaveValue(30);
  expect(screen.getByRole("button", { name: "预览并保存" })).toBeEnabled();
  expect(screen.queryByRole("button", { name: "载入最新设置" })).not.toBeInTheDocument();
  expect(fetcher).not.toHaveBeenCalled();
});

it("仅维护运行状态和版本更新不覆盖草稿或误报配置冲突", async () => {
  const client = await mount(vi.fn());
  const user = userEvent.setup();
  const interval = screen.getByRole("spinbutton", { name: "检查间隔（分钟）" });
  await user.clear(interval);
  await user.type(interval, "12");
  await act(async () => {
    client.setQueryData(maintenanceKey, { ...settings, revision: 4, message: "检查完成" });
  });
  expect(interval).toHaveValue(12);
  expect(screen.getByRole("button", { name: "预览并保存" })).toBeEnabled();
  expect(screen.queryByRole("button", { name: "载入最新设置" })).not.toBeInTheDocument();
});

it("预览期间配置更新禁止确认旧范围，载入最新配置关闭旧预览", async () => {
  const fetcher = vi.fn(async () =>
    Response.json({
      id: "preview-one",
      revision: 1,
      expires_at: "2099-01-01T00:00:00Z",
      settings,
      accounts: [template.summary],
    }),
  );
  const client = await mount(fetcher);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "预览并保存" }));
  const dialog = within(await screen.findByRole("dialog"));
  await act(async () => {
    client.setQueryData(maintenanceKey, { ...settings, revision: 4, interval_minutes: 20 });
  });
  await waitFor(() => expect(dialog.getByRole("button", { name: "确认保存" })).toBeDisabled());
  expect(dialog.getByRole("button", { name: "返回修改" })).toBeEnabled();
  await user.click(dialog.getByRole("button", { name: "返回修改" }));
  await waitFor(() =>
    expect(screen.getByRole("spinbutton", { name: "检查间隔（分钟）" })).toHaveValue(20),
  );
  expect(fetcher).toHaveBeenCalledOnce();
});

it("维护分组读取失败提供重试且禁止选择空列表，恢复后保留已编辑字段", async () => {
  let finish!: (response: Response) => void;
  let reads = 0;
  const fetcher = vi.fn(() => {
    reads++;
    return new Promise<Response>((resolve) => {
      finish = resolve;
    });
  });
  await mount(fetcher, false);
  expect(screen.getByRole("status", { name: "正在读取维护分组" })).toBeVisible();
  const selector = screen.getByRole("combobox", { name: "维护分组" });
  expect(selector).toHaveAttribute("aria-disabled", "true");
  await act(async () => finish(Response.json({ detail: "账号目录暂不可用" }, { status: 503 })));
  expect(await screen.findByRole("button", { name: "重新读取" })).toBeEnabled();
  expect(screen.getAllByText("账号目录暂不可用")).toHaveLength(1);
  expect(selector).toHaveAttribute("aria-disabled", "true");
  const user = userEvent.setup();
  const interval = screen.getByRole("spinbutton", { name: "检查间隔（分钟）" });
  await user.clear(interval);
  await user.type(interval, "12");
  await user.click(screen.getByRole("button", { name: "重新读取" }));
  await waitFor(() => expect(reads).toBe(2));
  await act(async () => finish(Response.json([template.summary])));
  await waitFor(() => expect(selector).not.toHaveAttribute("aria-disabled", "true"));
  expect(interval).toHaveValue(12);
  await user.click(selector);
  await user.click(await screen.findByRole("option", { name: "团队" }));
  expect(selector).toHaveTextContent("团队");
});

it("维护保存失败保留草稿与预览，重试成功采用确认结果且不退回旧值", async () => {
  let saves = 0;
  const writes: Array<{ path: string; body: unknown }> = [];
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input);
    if (path.endsWith("/accounts")) return Response.json([template.summary]);
    writes.push({ path, body: JSON.parse(String(init?.body)) });
    if (path.endsWith("/preview"))
      return Response.json({
        id: "preview-one",
        revision: 1,
        expires_at: "2099-01-01T00:00:00Z",
        settings: { ...settings, interval_minutes: 12 },
        accounts: [template.summary],
      });
    saves++;
    if (saves === 1) return Response.json({ detail: "保存暂不可用" }, { status: 503 });
    return Response.json({ ...settings, revision: 4, interval_minutes: 12 });
  });
  await mount(fetcher);
  const user = userEvent.setup();
  const interval = screen.getByRole("spinbutton", { name: "检查间隔（分钟）" });
  await user.clear(interval);
  await user.type(interval, "12");
  await user.click(screen.getByRole("button", { name: "预览并保存" }));
  const dialog = within(await screen.findByRole("dialog"));
  await user.click(dialog.getByRole("button", { name: "确认保存" }));
  expect(await screen.findByText("保存暂不可用")).toBeVisible();
  expect(interval).toHaveValue(12);
  await waitFor(() => expect(dialog.getByRole("button", { name: "确认保存" })).toBeEnabled());
  await user.click(dialog.getByRole("button", { name: "确认保存" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(screen.getByRole("button", { name: "立即检查" })).toBeEnabled();
  await user.click(screen.getByRole("button", { name: "维护设置" }));
  expect(screen.getByRole("spinbutton", { name: "检查间隔（分钟）" })).toHaveValue(12);
  expect(screen.queryByRole("button", { name: "载入最新设置" })).not.toBeInTheDocument();
  expect(writes[0]).toEqual({
    path: "/api/account-workbench/maintenance/preview",
    body: {
      enabled: false,
      interval_minutes: 12,
      cooldown_minutes: 10,
      check_after_repair: true,
      group_ids: [],
    },
  });
  expect(writes.slice(1)).toEqual([
    {
      path: "/api/account-workbench/maintenance/configure",
      body: { id: "preview-one", revision: 1 },
    },
    {
      path: "/api/account-workbench/maintenance/configure",
      body: { id: "preview-one", revision: 1 },
    },
  ]);
});

it("维护分组后台刷新失败仍保留已有选项与选中分组", async () => {
  const client = await mount(
    vi.fn(async () => Response.json({ detail: "目录刷新失败" }, { status: 503 })),
  );
  const user = userEvent.setup();
  const selector = screen.getByRole("combobox", { name: "维护分组" });
  await user.click(selector);
  await user.click(await screen.findByRole("option", { name: "团队" }));
  await user.keyboard("{Escape}");
  await act(async () => {
    await client.invalidateQueries({ queryKey: workbenchKeys.accounts });
  });
  expect(await screen.findByText("目录刷新失败")).toBeVisible();
  expect(selector).toHaveTextContent("团队");
  expect(selector).not.toHaveAttribute("aria-disabled", "true");
  expect(screen.queryByRole("button", { name: "重新读取" })).not.toBeInTheDocument();
});
