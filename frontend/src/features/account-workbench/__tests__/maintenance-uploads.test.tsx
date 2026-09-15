import type { ReactElement } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Task, WorkbenchPendingUpload } from "@/api";
import { WorkbenchMaintenancePanel } from "../components/workbench-maintenance";
import { WorkbenchMaintenanceUploads } from "../components/workbench-maintenance-uploads";

const clients: QueryClient[] = [];
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const upload: WorkbenchPendingUpload = {
  id: "source-upload:0",
  source_task_id: "source-upload",
  account_id: "101",
  email: "pending@example.test",
  status: "cooldown",
  message: "新授权已保存，冷却后核对原上传",
  next_retry_at: "2026-09-15T12:30:00Z",
  expires_at: "2026-09-16T12:00:00Z",
};
const task: Task = {
  id: "source-upload",
  skill: "account-workbench",
  operation: "account-workbench-retry",
  status: "failed",
  progress: 1,
  message: "凭据上传被拒绝，已保留待上传结果",
  result: {},
  created_at: "2026-09-15T12:00:00Z",
  updated_at: "2026-09-15T12:00:00Z",
};

function mount(element: ReactElement): void {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  clients.push(client);
  render(<QueryClientProvider client={client}>{element}</QueryClientProvider>);
}

describe("维护待上传账号", () => {
  it("待上传为空时显示空状态且不读取任务", () => {
    const fetcher = vi.fn<typeof fetch>();
    vi.stubGlobal("fetch", fetcher);
    mount(<WorkbenchMaintenanceUploads items={[]} refreshing={false} onRefresh={() => {}} />);
    expect(screen.getByText("暂无待上传账号")).toBeVisible();
    expect(screen.queryByRole("button", { name: /来源任务/ })).not.toBeInTheDocument();
    expect(fetcher).not.toHaveBeenCalled();
  });

  it.each([
    ["pending", "等待上传"],
    ["running", "正在处理"],
    ["cooldown", "等待冷却"],
    ["waiting_session", "等待连接会话"],
    ["review", "待人工核对"],
  ] as const)("后端返回 %s 时显示对应状态 %s", (status, label) => {
    mount(
      <WorkbenchMaintenanceUploads
        items={[{ ...upload, status }]}
        refreshing={false}
        onRefresh={() => {}}
      />,
    );
    const list = screen.getByRole("list", { name: "待上传账号列表" });
    expect(list).toHaveTextContent(label);
    expect(list).toHaveTextContent("账号 ID：101");
    expect(list).toHaveTextContent(upload.next_retry_at!);
    expect(list).toHaveTextContent(upload.expires_at);
  });

  it("来源任务读取中仍可关闭且关闭后没有写请求", async () => {
    const fetcher = vi.fn<typeof fetch>(() => new Promise<Response>(() => {}));
    vi.stubGlobal("fetch", fetcher);
    mount(<WorkbenchMaintenanceUploads items={[upload]} refreshing={false} onRefresh={() => {}} />);
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "查看账号 101 的来源任务" }));
    const dialog = screen.getByRole("dialog", { name: "待上传来源任务" });
    expect(within(dialog).getByText("正在读取待上传来源任务")).toBeVisible();
    await user.click(within(dialog).getByRole("button", { name: "关闭" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(fetcher.mock.calls[0]?.[1]?.method ?? "GET").toBe("GET");
  });

  it("来源任务读取失败时重试后显示原任务且不提供重复上传操作", async () => {
    let fail = true;
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>(async () =>
        fail ? Response.json({ detail: "原任务暂不可读取" }, { status: 503 }) : Response.json(task),
      ),
    );
    mount(<WorkbenchMaintenanceUploads items={[upload]} refreshing={false} onRefresh={() => {}} />);
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "查看账号 101 的来源任务" }));
    const retry = await screen.findByRole("button", { name: "重新读取" });
    fail = false;
    await user.click(retry);
    const dialog = screen.getByRole("dialog", { name: "待上传来源任务" });
    expect(
      await within(dialog).findByRole("article", { name: "处理任务 source-upload" }),
    ).toHaveTextContent(task.message);
    expect(within(dialog).queryByRole("button", { name: /重试|上传/ })).not.toBeInTheDocument();
  });

  it("维护已停用时仍显示待上传账号且后台读取失败保留摘要", async () => {
    let fail = false;
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>(async (input) => {
        if (String(input).endsWith("/groups")) return Response.json([]);
        if (fail) return Response.json({ detail: "暂不可读取" }, { status: 503 });
        return Response.json({
          enabled: false,
          reauthorize_with_profiles: false,
          revision: 2,
          interval_minutes: 5,
          cooldown_minutes: 10,
          group_ids: [],
          check_after_repair: false,
          model: "test-model",
          pending_uploads: [upload],
        });
      }),
    );
    mount(<WorkbenchMaintenancePanel />);
    expect(await screen.findByText(upload.email)).toBeVisible();
    expect(screen.queryByRole("region", { name: "维护账号重新授权" })).not.toBeInTheDocument();
    fail = true;
    await userEvent.setup().click(screen.getByRole("button", { name: "刷新待上传账号" }));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "刷新待上传账号" })).toBeEnabled(),
    );
    expect(screen.getByText(upload.email)).toBeVisible();
  });
});
