import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Task, WorkbenchOAuthSession, WorkbenchPreview } from "@/api";
import { WorkbenchOAuth } from "../components/workbench-oauth";

const clients: QueryClient[] = [];
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
const task: Task = {
  id: "oauth-import-task",
  skill: "account-workbench",
  operation: "account-workbench-import",
  status: "queued",
  progress: 0,
  message: "等待导入账号",
  result: {},
  created_at: "2026-09-14T00:00:00Z",
  updated_at: "2026-09-14T00:00:00Z",
};
function preview(): WorkbenchPreview {
  return {
    id: "authorized-preview",
    expires_at: new Date(Date.now() + 600000).toISOString(),
    target: "https://sub2api.example.test",
    errors: [],
    check_after_import: false,
    model: "",
    items: [
      {
        id: "0",
        index: 0,
        name: "授权账号",
        email: "test@example.test",
        plan_type: "plus",
        template_id: "",
        template_name: "",
        template_revision: 0,
        group_ids: [],
        duplicate: false,
      },
    ],
  };
}
function mount(fetcher?: typeof fetch): {
  requests: Array<{ path: string; method: string; body: unknown }>;
  unmount: () => void;
} {
  const requests: Array<{ path: string; method: string; body: unknown }> = [];
  const authorized: WorkbenchOAuthSession = {
    id: "oauth-1",
    task_id: "oauth-task",
    host: "auth.openai.com",
    status: "authorized",
    message: "授权成功",
    expires_at: new Date(Date.now() + 900000).toISOString(),
    width: 1100,
    height: 760,
  };
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input, init) => {
      const path = String(input);
      const method = init?.method ?? "GET";
      requests.push({
        path,
        method,
        body: init?.body ? (JSON.parse(String(init.body)) as unknown) : null,
      });
      if (fetcher && path.endsWith("/preview")) return fetcher(input, init);
      if (method === "DELETE") return Response.json({ deleted: true, cancelled: true });
      if (path.endsWith("/templates")) return Response.json([]);
      if (path.endsWith("/preview")) return Response.json(preview());
      if (path.endsWith("/import") || path.includes("/tasks/")) return Response.json(task);
      return Response.json(authorized);
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  const result = render(
    <QueryClientProvider client={client}>
      <WorkbenchOAuth />
    </QueryClientProvider>,
  );
  return { requests, unmount: result.unmount };
}

describe("授权账号导入", () => {
  it("授权成功后预览只提交配置，二次确认后通过预览 ID 创建导入任务", async () => {
    const view = mount();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "开始授权登录" }));
    await user.click(await screen.findByRole("button", { name: "预览授权账号" }));
    await screen.findByRole("table", { name: "账号预览" });
    expect(
      view.requests.find((request) => request.path.endsWith("/oauth/oauth-1/preview"))?.body,
    ).toEqual({ check_after_import: false, model: "" });
    expect(view.requests.some((request) => request.path.endsWith("/import"))).toBe(false);
    await user.click(screen.getByRole("button", { name: "确认导入 1 个账号" }));
    const dialog = screen.getByRole("dialog", { name: "确认批量导入账号" });
    expect(dialog).toHaveTextContent("https://sub2api.example.test");
    await user.click(within(dialog).getByRole("button", { name: "创建导入任务" }));
    await waitFor(() =>
      expect(view.requests.find((request) => request.path.endsWith("/import"))?.body).toEqual({
        preview_id: "authorized-preview",
        confirmed: true,
      }),
    );
    await waitFor(() =>
      expect(screen.queryByRole("table", { name: "账号预览" })).not.toBeInTheDocument(),
    );
    expect(screen.getByRole("status", { name: "等待导入账号" })).toBeInTheDocument();
    expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  });

  it("开启检测但模型为空时显示字段错误且不生成预览", async () => {
    const view = mount();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "开始授权登录" }));
    await user.click(await screen.findByRole("checkbox", { name: "导入后执行模型检测" }));
    await user.click(screen.getByRole("button", { name: "预览授权账号" }));
    expect(await screen.findByRole("textbox", { name: "检测模型" })).toHaveAttribute(
      "aria-invalid",
      "true",
    );
    expect(view.requests.some((request) => request.path.endsWith("/preview"))).toBe(false);
  });

  it("预览后变更检测配置会撤销旧预览", async () => {
    const view = mount();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "开始授权登录" }));
    await user.click(await screen.findByRole("button", { name: "预览授权账号" }));
    await screen.findByRole("table", { name: "账号预览" });
    await user.click(screen.getByRole("checkbox", { name: "导入后执行模型检测" }));
    expect(screen.queryByRole("table", { name: "账号预览" })).not.toBeInTheDocument();
    expect(view.requests).toContainEqual({
      path: "/api/account-workbench/preview/authorized-preview",
      method: "DELETE",
      body: null,
    });
  });

  it("预览请求未返回时结束授权会撤销迟到的预览", async () => {
    let resolvePreview: (value: Response) => void = () => undefined;
    const response = new Promise<Response>((resolve) => {
      resolvePreview = resolve;
    });
    const view = mount(async () => response);
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "开始授权登录" }));
    await user.click(await screen.findByRole("button", { name: "预览授权账号" }));
    expect(screen.getByRole("status", { name: "正在生成授权账号预览" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "结束授权" }));
    resolvePreview(Response.json(preview()));
    await waitFor(() =>
      expect(view.requests).toContainEqual({
        path: "/api/account-workbench/preview/authorized-preview",
        method: "DELETE",
        body: null,
      }),
    );
    expect(screen.queryByRole("table", { name: "账号预览" })).not.toBeInTheDocument();
  });

  it("生成预览失败时允许重试并保持结束授权入口", async () => {
    mount(async () =>
      Response.json(
        { detail: "目标配置已改变，请重新授权", code: "workbench_target_changed" },
        { status: 409 },
      ),
    );
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "开始授权登录" }));
    await user.click(await screen.findByRole("button", { name: "预览授权账号" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "预览授权账号" })).toBeEnabled());
    expect(screen.getByRole("button", { name: "结束授权" })).toBeEnabled();
    expect(screen.queryByRole("table", { name: "账号预览" })).not.toBeInTheDocument();
  });
});
