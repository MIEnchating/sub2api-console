import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { KumaConfigPage } from "../kuma-config-page";
import { OperationDialogFooter } from "../operation-dialog-footer";
import { UptimeKumaPage } from "../uptime-kuma-page";
import { KumaResourcesPage } from "../resources-page";
import { config, monitor, task } from "./fixtures";
import type { ReactNode } from "react";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  toast.dismiss();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function renderOperation(children: ReactNode): void {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(<QueryClientProvider client={client}>{children}</QueryClientProvider>);
}

it("任务尚未创建时显示忙碌状态，不提前显示进度或取消任务按钮", () => {
  renderOperation(
    <OperationDialogFooter pending starting task={null}>
      <Button disabled>正在保存…</Button>
    </OperationDialogFooter>,
  );
  expect(screen.getByRole("status", { name: "正在创建 Uptime Kuma 任务" })).toBeVisible();
  expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "取消任务" })).not.toBeInTheDocument();
});

it("任务已结束但列表仍在刷新时不重新显示任务启动状态", () => {
  renderOperation(
    <OperationDialogFooter pending starting={false} task={null}>
      <Button disabled>正在保存…</Button>
    </OperationDialogFooter>,
  );
  expect(screen.queryByRole("status")).not.toBeInTheDocument();
  expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
});

it("取消接口失败时保留任务进度与提交保护，并允许再次取消", async () => {
  const error = vi.spyOn(toast, "error");
  vi.stubGlobal("fetch", async () =>
    Response.json({ detail: "取消状态保存失败" }, { status: 503 }),
  );
  renderOperation(
    <OperationDialogFooter pending task={{ ...task, progress: 0 }}>
      <Button disabled>正在保存…</Button>
    </OperationDialogFooter>,
  );
  await userEvent.setup().click(screen.getByRole("button", { name: "取消任务" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "取消任务" })).toBeEnabled());
  expect(screen.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "0");
  expect(screen.getByRole("button", { name: "正在保存…" })).toBeDisabled();
  expect(error).toHaveBeenCalledTimes(1);
  expect(error).toHaveBeenCalledWith("取消状态保存失败", expect.any(Object));
});

it("配置验证取消请求期间防止重复操作，接受取消后刷新配置且只提示一次", async () => {
  let cancelled = false;
  let release!: () => void;
  const accepted = new Promise<void>((resolve) => {
    release = resolve;
  });
  let reads = 0;
  let writes = 0;
  let cancellations = 0;
  const info = vi.spyOn(toast, "info");
  const success = vi.spyOn(toast, "success");
  const error = vi.spyOn(toast, "error");
  vi.stubGlobal("fetch", async (path: string, init?: RequestInit) => {
    if (path === "/api/uptime-kuma/config" && init?.method === "PUT") {
      writes++;
      return Response.json({ ...task, status: "queued", progress: 0 });
    }
    if (path === "/api/uptime-kuma/config") {
      reads++;
      return Response.json(config);
    }
    if (path === `/api/tasks/${task.id}` && init?.method === "DELETE") {
      cancellations++;
      await accepted;
      cancelled = true;
      return Response.json({ cancelled: true });
    }
    if (path === `/api/tasks/${task.id}`)
      return Response.json({
        ...task,
        status: cancelled ? "cancelled" : "running",
        progress: 0,
        message: cancelled ? "任务已手动取消" : "正在验证配置",
      });
    throw new Error(`Unexpected request: ${path}`);
  });
  renderOperation(<KumaConfigPage />);
  const user = userEvent.setup();
  await screen.findByLabelText("API 密钥");
  await user.click(screen.getByRole("button", { name: "验证并保存" }));
  const cancel = await screen.findByRole("button", { name: "取消任务" });
  await user.click(cancel);
  expect(screen.getByRole("button", { name: "取消中" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "正在验证…" })).toBeDisabled();
  expect(screen.getByLabelText("API 密钥")).toBeDisabled();
  await user.click(cancel);
  expect(cancellations).toBe(1);
  await act(async () => release());
  await waitFor(() => expect(screen.getByRole("button", { name: "验证并保存" })).toBeEnabled());
  expect(reads).toBeGreaterThan(1);
  expect(writes).toBe(1);
  expect(info).toHaveBeenCalledTimes(1);
  expect(info).toHaveBeenCalledWith(expect.stringMatching(/不会自动撤销.*核对远端状态/));
  expect(success).not.toHaveBeenCalled();
  expect(error).not.toHaveBeenCalled();
});

it.each(["monitor", "resource"] as const)(
  "%s 的写入确认取消后关闭窗口并刷新列表，不自动重复提交",
  async (kind) => {
    let cancelled = false;
    let writes = 0;
    let reads = 0;
    const info = vi.spyOn(toast, "info");
    vi.stubGlobal("fetch", async (path: string, init?: RequestInit) => {
      if (path === `/api/tasks/${task.id}` && init?.method === "DELETE") {
        cancelled = true;
        return Response.json({ cancelled: true });
      }
      if (path === `/api/tasks/${task.id}`)
        return Response.json({ ...task, status: cancelled ? "cancelled" : "running", progress: 0 });
      if (init?.method === "POST") {
        writes++;
        return Response.json({ ...task, status: "queued", progress: 0 });
      }
      if (path.endsWith("/config")) return Response.json(config);
      if (path.endsWith("/dictionaries")) return Response.json({ items: [] });
      reads++;
      return Response.json({
        config,
        monitors: [monitor],
        items: [
          {
            id: 37,
            name: "维护窗口",
            type: "manual",
            active: true,
            revision: "r1",
            association_revision: "a1",
          },
        ],
      });
    });
    renderOperation(
      kind === "monitor" ? <UptimeKumaPage /> : <KumaResourcesPage kind="maintenance" />,
    );
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "暂停" }));
    const dialog = screen.getByRole("dialog");
    const readsBefore = reads;
    await user.click(within(dialog).getByRole("button", { name: "暂停" }));
    await user.click(await within(dialog).findByRole("button", { name: "取消任务" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    await waitFor(() => expect(reads).toBeGreaterThan(readsBefore));
    expect(writes).toBe(1);
    expect(info).toHaveBeenCalledTimes(1);
  },
);
