import { StrictMode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { WorkbenchCleanupPreview, WorkbenchLoginProfile } from "@/api";
import { WorkbenchCleanup } from "../components/workbench-cleanup";

const profile: WorkbenchLoginProfile = {
  id: "profile-42",
  account_id: "42",
  user_id: "user-42",
  workspace_id: "workspace-42",
  email: "owner@example.com",
  revision: 7,
  updated_at: "2026-09-14T00:00:00Z",
  has_password: true,
  has_totp: false,
  has_proxy: false,
};
const clients: QueryClient[] = [];
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
function preview(): WorkbenchCleanupPreview {
  return {
    id: "cleanup-preview",
    target: "https://target.example.test",
    expires_at: new Date(Date.now() + 600000).toISOString(),
    profiles: [profile],
    blocked: false,
    blockers: [],
    items: [
      {
        id: "profile-42",
        kind: "login_profile",
        account_ids: ["42"],
        count: 1,
        action: "delete",
        reason: "所选账号登录资料",
      },
      {
        id: "mixed-file",
        kind: "account_export",
        account_ids: ["42", "43"],
        count: 2,
        action: "retain",
        reason: "文件包含其他账号",
      },
    ],
  };
}
function mount(onCreated = vi.fn(), onClose = vi.fn()): ReturnType<typeof render> {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  return render(
    <StrictMode>
      <QueryClientProvider client={client}>
        <WorkbenchCleanup profiles={[profile]} onClose={onClose} onCreated={onCreated} />
      </QueryClientProvider>
    </StrictMode>,
  );
}

it("清理预览显示删除及保留范围，确认仅提交一次性预览ID", async () => {
  const requests: { path: string; body: unknown }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input, init) => {
      requests.push({
        path: String(input),
        body: init?.body ? (JSON.parse(String(init.body)) as unknown) : null,
      });
      return Response.json(
        String(input).endsWith("/preview") ? preview() : { id: "cleanup-task", status: "queued" },
      );
    }),
  );
  const created = vi.fn();
  mount(created);
  await screen.findByText("文件包含其他账号");
  const scope = screen.getByRole("list", { name: "关联资料清理明细" });
  expect(within(scope).getByText("保留")).toBeInTheDocument();
  expect(within(scope).getByText("删除")).toBeInTheDocument();
  expect(screen.getByText("删除 1 项，保留 1 项")).toBeInTheDocument();
  expect(requests).toEqual([
    {
      path: "/api/account-workbench/cleanup/preview",
      body: { items: [{ id: "profile-42", revision: 7 }] },
    },
  ]);
  await userEvent.setup().click(screen.getByRole("button", { name: "确认永久删除所列资料" }));
  await waitFor(() => expect(created).toHaveBeenCalledOnce());
  expect(requests[1]).toEqual({
    path: "/api/account-workbench/cleanup",
    body: { preview_id: "cleanup-preview", confirmed: true },
  });
});

it("活动任务阻止清理时展示稳定任务范围并禁止确认", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async () =>
      Response.json({
        ...preview(),
        blocked: true,
        blockers: [
          {
            task_id: "active-task",
            operation: "account-workbench-import",
            status: "running",
            message: "正在导入账号",
          },
        ],
      }),
    ),
  );
  mount();
  await screen.findByText("任务 ID：active-task");
  expect(screen.getByRole("button", { name: "确认永久删除所列资料" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "返回" })).toBeEnabled();
});

it("清理范围读取失败仍可重试和返回且不能删除", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async () => Response.json({ detail: "资料版本已变化" }, { status: 409 })),
  );
  mount();
  await screen.findByRole("button", { name: "重新读取" });
  expect(screen.getByRole("button", { name: "确认永久删除所列资料" })).toBeDisabled();
});

it("关闭等待中的清理预览后删除迟到结果", async () => {
  let resolve: (response: Response) => void = () => undefined;
  const discarded: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>((input, init) => {
      if (init?.method === "DELETE") {
        discarded.push(String(input));
        return Promise.resolve(Response.json({ deleted: true }));
      }
      return new Promise((done) => {
        resolve = done;
      });
    }),
  );
  const view = mount();
  expect(await screen.findByRole("status", { name: "正在核对关联资料清理范围" })).toHaveAttribute(
    "aria-busy",
    "true",
  );
  view.unmount();
  resolve(Response.json(preview()));
  await waitFor(() =>
    expect(discarded).toEqual(["/api/account-workbench/cleanup/preview/cleanup-preview"]),
  );
});

it("清理预览过期时禁止删除", async () => {
  const expired = { ...preview(), expires_at: "2020-01-01T00:00:00Z" };
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async () => Response.json(expired)),
  );
  mount();
  await screen.findByText("清理预览已过期，请关闭后重新核对。");
  expect(screen.getByRole("button", { name: "确认永久删除所列资料" })).toBeDisabled();
});
