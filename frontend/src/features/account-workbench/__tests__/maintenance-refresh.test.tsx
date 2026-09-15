import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import type { Task, WorkbenchMaintenance } from "@/api";
import { WorkbenchMaintenancePanel } from "../components/workbench-maintenance";
import { workbenchKeys } from "../constants";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

const initial: WorkbenchMaintenance = {
  enabled: false,
  reauthorize_with_profiles: false,
  revision: 2,
  interval_minutes: 5,
  cooldown_minutes: 10,
  group_ids: [],
  check_after_repair: false,
  model: "test-model",
};
const updated: WorkbenchMaintenance = { ...initial, revision: 3, interval_minutes: 10 };

function mount(): (config: WorkbenchMaintenance) => void {
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(workbenchKeys.maintenance, initial);
  client.setQueryData(["groups"], []);
  const view = render(
    <QueryClientProvider client={client}>
      <WorkbenchMaintenancePanel />
    </QueryClientProvider>,
  );
  return (config) => {
    act(() => {
      client.setQueryData(workbenchKeys.maintenance, config);
    });
    view.rerender(
      <QueryClientProvider client={client}>
        <WorkbenchMaintenancePanel />
      </QueryClientProvider>,
    );
  };
}

it("维护设置有未保存修改时后台版本变化保留草稿及其原始提交版本", async () => {
  let submitted: WorkbenchMaintenance | undefined;
  vi.stubGlobal("fetch", async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "PUT") {
      submitted = JSON.parse(String(init.body)) as WorkbenchMaintenance;
      return Response.json({ detail: "维护配置已变化" }, { status: 409 });
    }
    return Response.json(updated);
  });
  const refresh = mount();
  fireEvent.change(screen.getByRole("spinbutton", { name: "检查间隔（分钟）" }), {
    target: { value: "15" },
  });

  refresh(updated);

  expect(screen.getByRole("spinbutton", { name: "检查间隔（分钟）" })).toHaveValue(15);
  fireEvent.click(screen.getByRole("button", { name: "保存维护设置" }));
  await waitFor(() => expect(submitted?.revision).toBe(2));
  expect(submitted?.interval_minutes).toBe(15);
});

it("维护设置尚未修改时后台新版本同步到表单", () => {
  const refresh = mount();

  refresh(updated);

  expect(screen.getByRole("spinbutton", { name: "检查间隔（分钟）" })).toHaveValue(10);
});

it("维护草稿与后台版本冲突后通过键盘重置修改会采用最新配置", async () => {
  const refresh = mount();
  fireEvent.change(screen.getByRole("spinbutton", { name: "检查间隔（分钟）" }), {
    target: { value: "15" },
  });
  refresh(updated);

  screen.getByRole("button", { name: "重置修改" }).focus();
  await userEvent.setup().keyboard("{Enter}");

  expect(screen.getByRole("spinbutton", { name: "检查间隔（分钟）" })).toHaveValue(10);
  expect(screen.getByRole("button", { name: "立即检查并修复" })).toBeEnabled();
});

it("维护任务创建后后台配置更新仍保留任务状态和取消入口", async () => {
  const task: Task = {
    id: "maintenance-running",
    skill: "account-workbench",
    operation: "account-workbench-maintenance",
    status: "running",
    progress: 10,
    message: "正在维护账号",
    result: {},
    created_at: "2026-09-14T00:00:00Z",
    updated_at: "2026-09-14T00:00:00Z",
  };
  vi.stubGlobal("fetch", async () => Response.json(task));
  const refresh = mount();
  fireEvent.click(screen.getByRole("button", { name: "立即检查并修复" }));
  fireEvent.click(screen.getByRole("button", { name: "创建维护任务" }));
  await screen.findByRole("article", { name: "处理任务 maintenance-running" });

  refresh(updated);

  expect(screen.getByRole("article", { name: "处理任务 maintenance-running" })).toBeVisible();
  expect(screen.getByRole("button", { name: "取消任务" })).toBeEnabled();
});

it("维护设置保存期间后台更新版本时继续禁用重复提交直到原请求完成", async () => {
  let resolve!: (response: Response) => void;
  vi.stubGlobal(
    "fetch",
    () =>
      new Promise<Response>((done) => {
        resolve = done;
      }),
  );
  const refresh = mount();
  fireEvent.change(screen.getByRole("spinbutton", { name: "检查间隔（分钟）" }), {
    target: { value: "15" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存维护设置" }));
  await screen.findByRole("button", { name: "正在保存…" });

  refresh(updated);

  expect(screen.getByRole("button", { name: "正在保存…" })).toBeDisabled();
  expect(screen.getByRole("spinbutton", { name: "检查间隔（分钟）" })).toBeDisabled();
  await act(async () => resolve(Response.json({ ...initial, revision: 4, interval_minutes: 15 })));
  await waitFor(() => expect(screen.getByRole("button", { name: "保存维护设置" })).toBeEnabled());
});
