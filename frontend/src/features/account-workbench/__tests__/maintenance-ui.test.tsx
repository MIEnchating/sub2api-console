import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { MaintenancePanel } from "../components/maintenance-panel";
import { maintenanceKey, workbenchKeys } from "../constants";
import type { WorkbenchMaintenance } from "../types";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});
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
function mount(value: WorkbenchMaintenance | undefined, fetcher: typeof fetch): void {
  vi.stubGlobal("fetch", fetcher);
  vi.stubGlobal("PointerEvent", MouseEvent);
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(maintenanceKey, value);
  client.setQueryData(workbenchKeys.accounts, []);
  render(
    <QueryClientProvider client={client}>
      <MaintenancePanel />
    </QueryClientProvider>,
  );
}
it("开启维护先展示账号范围，确认后只提交预览标识", async () => {
  const writes: Array<{ path: string; body: unknown }> = [];
  mount(
    settings,
    vi.fn(async (input, init) => {
      const path = String(input);
      if (init?.method === "POST") writes.push({ path, body: JSON.parse(String(init.body)) });
      if (path.endsWith("/preview"))
        return Response.json({
          id: "preview-one",
          revision: 2,
          expires_at: "2099-01-01T00:00:00Z",
          settings: { ...settings, enabled: true },
          accounts: [{ id: "41", email: "owner@example.test", name: "account" }],
        });
      if (path.endsWith("/configure"))
        return Response.json({ ...settings, revision: 4, enabled: true });
      return Response.json([]);
    }),
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "维护设置" }));
  await user.click(screen.getByRole("checkbox", { name: "启用定时检查" }));
  expect(screen.getByRole("button", { name: "立即检查" })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "预览并保存" }));
  const dialog = await screen.findByRole("dialog");
  expect(within(dialog).getByText("owner@example.test · #41")).toBeVisible();
  expect(writes).toHaveLength(1);
  await user.click(within(dialog).getByRole("button", { name: "确认保存" }));
  await waitFor(() =>
    expect(writes[1]).toEqual({
      path: "/api/account-workbench/maintenance/configure",
      body: { id: "preview-one", revision: 2 },
    }),
  );
});
it("维护运行时禁止改设置和重复检查，同时保留停止入口", async () => {
  mount(
    {
      ...settings,
      running: true,
      enabled: true,
      results: [
        {
          account_id: "41",
          email: "owner@example.test",
          action: "none",
          status: "cooldown",
          reason: "rate_limited",
        },
      ],
    },
    vi.fn(),
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "维护设置" }));
  expect(screen.getByRole("spinbutton", { name: "检查间隔（分钟）" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "立即检查" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "预览并保存" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "停止维护" })).toBeEnabled();
  await user.click(screen.getByRole("button", { name: "取消" }));
  expect(await screen.findByRole("status")).toHaveTextContent("正在维护账号");
  expect(screen.getByRole("cell", { name: "上游限流，冷却后再检查" })).toBeVisible();
});
it("检查间隔小于一分钟时阻止预览并显示字段错误", async () => {
  const fetcher = vi.fn();
  mount(settings, fetcher);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "维护设置" }));
  const input = screen.getByRole("spinbutton", { name: "检查间隔（分钟）" });
  await user.clear(input);
  await user.type(input, "0");
  // Submit bypasses native constraints to exercise the shared form schema.
  const form = screen.getByRole("button", { name: "预览并保存" }).closest("form")!;
  form.noValidate = true;
  await user.click(screen.getByRole("button", { name: "预览并保存" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("最少 1 分钟");
  expect(fetcher).not.toHaveBeenCalled();
});

it("首次读取显示表格骨架，结果就绪后通过按钮打开设置弹窗", async () => {
  let complete: (response: Response) => void = () => undefined;
  mount(
    undefined,
    vi.fn(
      () =>
        new Promise<Response>((resolve) => {
          complete = resolve;
        }),
    ),
  );
  const loading = screen.getByLabelText("正在读取维护设置");
  expect(loading).toHaveAttribute("aria-busy", "true");
  expect(loading.lastElementChild).toHaveClass("grid");
  expect(screen.queryByRole("form", { name: "维护设置" })).not.toBeInTheDocument();
  complete(Response.json(settings));
  expect(await screen.findByRole("button", { name: "维护设置" })).toBeVisible();
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(screen.getByRole("table", { name: "维护运行状态" })).toBeVisible();
  expect(screen.getByRole("region", { name: "维护结果" })).toBeVisible();
  expect(screen.queryByLabelText("正在读取维护设置")).not.toBeInTheDocument();
  const user = userEvent.setup();
  const trigger = screen.getByRole("button", { name: "维护设置" });
  trigger.focus();
  await user.keyboard("{Enter}");
  expect(await screen.findByRole("dialog", { name: "维护设置" })).toBeVisible();
  await user.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(screen.getByRole("table", { name: "维护运行状态" })).toBeVisible();
});

it("读取维护范围期间可以取消，迟到响应不会重新打开确认弹窗", async () => {
  let complete!: (response: Response) => void;
  mount(
    settings,
    vi.fn(
      () =>
        new Promise<Response>((resolve) => {
          complete = resolve;
        }),
    ),
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "维护设置" }));
  await user.click(screen.getByRole("button", { name: "预览并保存" }));
  expect(screen.getByRole("status", { name: "正在读取维护范围" })).toBeVisible();
  await user.click(screen.getByRole("button", { name: "取消" }));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  await act(async () =>
    complete(
      Response.json({
        id: "late-preview",
        revision: 1,
        expires_at: "2099-01-01T00:00:00Z",
        settings,
        accounts: [],
      }),
    ),
  );
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(screen.getByRole("table", { name: "维护运行状态" })).toBeVisible();
});
