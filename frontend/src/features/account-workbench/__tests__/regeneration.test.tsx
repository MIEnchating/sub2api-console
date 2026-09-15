import type { ReactElement } from "react";
import { StrictMode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { WorkbenchRegeneration } from "../components/workbench-regeneration";
import { WorkbenchExports } from "../components/workbench-exports";

const clients: QueryClient[] = [];
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
function mount(view: ReactElement): ReturnType<typeof render> {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  return render(<QueryClientProvider client={client}>{view}</QueryClientProvider>);
}
const sourceTask = {
  id: "original-convert",
  skill: "account-workbench",
  operation: "account-workbench-convert",
  status: "succeeded",
  progress: 100,
  message: "私有文件已生成",
  result: {
    items: [
      { index: 0, status: "succeeded", report: { artifact_id: "private-file", kind: "accounts" } },
    ],
  },
  created_at: "2026-09-14T00:00:00Z",
  updated_at: "2026-09-14T00:00:00Z",
};
function preview() {
  return {
    id: "rt-preview",
    target: "https://target.example.test",
    expires_at: new Date(Date.now() + 600000).toISOString(),
    items: [
      {
        index: 7,
        account_id: "42",
        name: "重新生成账号",
        email: "owner@example.com",
        user_id: "user-42",
        workspace_id: "workspace-42",
        revision: "source-version",
      },
    ],
  };
}
function network(): { path: string; body: unknown }[] {
  const requests: { path: string; body: unknown }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (url, init) => {
      const path = String(url);
      if (init?.method === "POST") requests.push({ path, body: JSON.parse(String(init.body)) });
      if (path.endsWith("/regenerate/preview")) return Response.json(preview());
      if (path.endsWith("/regenerate"))
        return Response.json({
          ...sourceTask,
          id: "regenerated",
          operation: "account-workbench-regenerate",
          status: "queued",
          progress: 0,
          message: "等待刷新 RT",
          result: {},
        });
      if (path.endsWith("/history")) return Response.json([sourceTask]);
      if (path.includes("/tasks/")) return Response.json(sourceTask);
      if (path.endsWith("/accounts"))
        return Response.json([
          { id: "42", name: "账号甲", platform: "openai", account_type: "oauth" },
        ]);
      if (path.endsWith("/exports")) return Response.json([]);
      return Response.json({ deleted: true });
    }),
  );
  return requests;
}

it("选择线上账号后先读取范围且确认才刷新现有RT", async () => {
  const requests = network();
  mount(<WorkbenchExports />);
  const user = userEvent.setup();
  await user.click(await screen.findByRole("checkbox", { name: "账号甲（ID 42）" }));
  await user.click(screen.getByRole("button", { name: "重新生成授权文件" }));
  const dialog = screen.getByRole("dialog", { name: "确认重新生成授权文件" });
  await within(dialog).findByText(/第 8 项：重新生成账号/);
  expect(dialog).toHaveTextContent("工作区：workspace-42");
  expect(dialog).toHaveTextContent("线上账号凭据不会自动更新");
  expect(requests).toEqual([
    { path: "/api/account-workbench/exports/regenerate/preview", body: { account_ids: ["42"] } },
  ]);
  await user.click(within(dialog).getByRole("button", { name: "确认刷新并生成文件" }));
  await waitFor(() => expect(requests).toHaveLength(2));
  expect(requests[1]).toEqual({
    path: "/api/account-workbench/exports/regenerate",
    body: { preview_id: "rt-preview", confirmed: true },
  });
});

it("开发模式重挂载效果不会重复创建或撤销仍在使用的再生预览", async () => {
  const requests: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input) => {
      requests.push(String(input));
      return Response.json(preview());
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  render(
    <StrictMode>
      <QueryClientProvider client={client}>
        <WorkbenchRegeneration
          source={{ account_ids: ["42"] }}
          onClose={() => undefined}
          onCreated={() => undefined}
        />
      </QueryClientProvider>
    </StrictMode>,
  );
  await screen.findByText(/第 8 项：重新生成账号/);
  expect(requests).toEqual(["/api/account-workbench/exports/regenerate/preview"]);
  expect(screen.getByRole("button", { name: "确认刷新并生成文件" })).toBeEnabled();
});

it("已结束转换记录用来源任务ID读取再生范围", async () => {
  const requests = network();
  mount(
    <WorkbenchRegeneration
      source={{ source_task_id: "original-convert" }}
      onClose={() => undefined}
      onCreated={() => undefined}
    />,
  );
  await screen.findByText(/第 8 项：重新生成账号/);
  expect(requests).toEqual([
    {
      path: "/api/account-workbench/exports/regenerate/preview",
      body: { source_task_id: "original-convert" },
    },
  ]);
});

it("再生预览读取失败保留重试且禁止提交", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async () =>
      Response.json({ detail: "来源已过期，请重新导出" }, { status: 409 }),
    ),
  );
  mount(
    <WorkbenchRegeneration
      source={{ source_task_id: "expired" }}
      onClose={() => undefined}
      onCreated={() => undefined}
    />,
  );
  await screen.findByRole("button", { name: "重新读取" });
  expect(screen.getByRole("button", { name: "确认刷新并生成文件" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "返回" })).toBeEnabled();
});

it("关闭仍在读取的再生预览后删除迟到预览", async () => {
  let resolve: (response: Response) => void = () => undefined;
  const discarded: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>((url, init) => {
      if (init?.method === "DELETE") {
        discarded.push(String(url));
        return Promise.resolve(Response.json({ deleted: true }));
      }
      return new Promise((done) => {
        resolve = done;
      });
    }),
  );
  const view = mount(
    <WorkbenchRegeneration
      source={{ account_ids: ["42"] }}
      onClose={() => undefined}
      onCreated={() => undefined}
    />,
  );
  expect(await screen.findByRole("status", { name: "正在核对重新生成范围" })).toHaveAttribute(
    "aria-busy",
    "true",
  );
  view.unmount();
  resolve(Response.json(preview()));
  await waitFor(() =>
    expect(discarded).toEqual(["/api/account-workbench/exports/preview/rt-preview"]),
  );
});

it("已经过期的再生预览不能刷新RT", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async () =>
      Response.json({ ...preview(), expires_at: "2020-01-01T00:00:00Z" }),
    ),
  );
  mount(
    <WorkbenchRegeneration
      source={{ account_ids: ["42"] }}
      onClose={() => undefined}
      onCreated={() => undefined}
    />,
  );
  await screen.findByRole("status", { name: "" });
  await screen.findByText("预览已过期，请关闭后重新选择来源。");
  expect(screen.getByRole("button", { name: "确认刷新并生成文件" })).toBeDisabled();
});
