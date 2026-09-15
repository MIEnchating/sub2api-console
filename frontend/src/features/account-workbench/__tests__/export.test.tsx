import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Task, WorkbenchExportPreview } from "@/api";
import { WorkbenchExports } from "../components/workbench-exports";

const clients: QueryClient[] = [];
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
const task: Task = {
  id: "export-task",
  skill: "account-workbench",
  operation: "account-workbench-export",
  status: "queued",
  progress: 0,
  message: "等待生成私有文件",
  result: {},
  created_at: "2026-09-14T00:00:00Z",
  updated_at: "2026-09-14T00:00:00Z",
};
function preview(): WorkbenchExportPreview {
  return {
    id: "export-preview",
    revision: "preview-version",
    expires_at: new Date(Date.now() + 600000).toISOString(),
    target: "https://sub2api.example.test",
    items: [{ account_id: "42", name: "团队账号", revision: "account-version" }],
  };
}
type Request = { path: string; method: string; body: unknown };
function mount(
  options: {
    previewResponse?: () => Promise<Response>;
    accounts?: Array<{ id: string; name: string; platform: string; account_type: string }>;
  } = {},
): { requests: Request[]; unmount: () => void } {
  const requests: Request[] = [];
  let artifacts = [
    {
      id: "private-artifact",
      kind: "accounts",
      count: 1,
      created_at: "2026-09-14T00:00:00Z",
      expires_at: "2026-09-15T00:00:00Z",
    },
  ];
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
      if (path === "/api/accounts")
        return Response.json(
          options.accounts ?? [
            { id: "42", name: "团队账号", platform: "openai", account_type: "oauth" },
            { id: "43", name: "API Key 账号", platform: "openai", account_type: "apikey" },
            { id: "44", name: "其他平台", platform: "anthropic", account_type: "oauth" },
          ],
        );
      if (path === "/api/account-workbench/exports/preview")
        return options.previewResponse?.() ?? Response.json(preview());
      if (method === "DELETE") {
        if (path.endsWith("/private-artifact")) artifacts = [];
        return Response.json({ deleted: true });
      }
      if (path === "/api/account-workbench/exports" && method === "GET")
        return Response.json(artifacts);
      if (
        (path === "/api/account-workbench/exports" && method === "POST") ||
        path.includes("/tasks/")
      )
        return Response.json(task);
      return Response.json({ detail: "隔离测试未配置此接口" }, { status: 503 });
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  const rendered = render(
    <QueryClientProvider client={client}>
      <WorkbenchExports />
    </QueryClientProvider>,
  );
  return { requests, unmount: rendered.unmount };
}

describe("账号私有导出", () => {
  it("选择 OAuth 稳定 ID 后预览范围，二次确认仅提交预览 ID 创建任务", async () => {
    const view = mount();
    const user = userEvent.setup();
    await user.click(await screen.findByRole("checkbox", { name: "团队账号（ID 42）" }));
    expect(
      screen.queryByRole("checkbox", { name: /API Key 账号|其他平台/ }),
    ).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "预览导出范围" }));
    await screen.findByRole("region", { name: "账号导出预览" });
    expect(view.requests.find((item) => item.path.endsWith("/exports/preview"))?.body).toEqual({
      account_ids: ["42"],
    });
    await user.click(screen.getByRole("button", { name: "确认导出 1 个账号" }));
    const dialog = screen.getByRole("dialog", { name: "确认生成私有账号文件" });
    expect(dialog).toHaveTextContent("https://sub2api.example.test");
    expect(dialog).toHaveTextContent("ID：42");
    expect(
      view.requests.some((item) => item.path.endsWith("/exports") && item.method === "POST"),
    ).toBe(false);
    await user.click(within(dialog).getByRole("button", { name: "创建导出任务" }));
    await waitFor(() =>
      expect(
        view.requests.find((item) => item.path.endsWith("/exports") && item.method === "POST")
          ?.body,
      ).toEqual({ preview_id: "export-preview", confirmed: true }),
    );
    await waitFor(() =>
      expect(screen.queryByRole("region", { name: "账号导出预览" })).not.toBeInTheDocument(),
    );
    expect(screen.getByRole("status", { name: "等待生成私有文件" })).toBeInTheDocument();
    expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  });

  it("变更已选账号会撤销原预览并移除确认入口", async () => {
    const view = mount();
    const user = userEvent.setup();
    const account = await screen.findByRole("checkbox", { name: "团队账号（ID 42）" });
    await user.click(account);
    await user.click(screen.getByRole("button", { name: "预览导出范围" }));
    await screen.findByRole("region", { name: "账号导出预览" });
    await user.click(account);
    expect(screen.queryByRole("region", { name: "账号导出预览" })).not.toBeInTheDocument();
    expect(view.requests).toContainEqual({
      path: "/api/account-workbench/exports/preview/export-preview",
      method: "DELETE",
      body: null,
    });
  });

  it("取消尚未返回的预览后清理迟到结果并允许重新选择", async () => {
    let resolvePreview: (response: Response) => void = () => undefined;
    const response = new Promise<Response>((resolve) => {
      resolvePreview = resolve;
    });
    const view = mount({ previewResponse: () => response });
    const user = userEvent.setup();
    await user.click(await screen.findByRole("checkbox", { name: "团队账号（ID 42）" }));
    await user.click(screen.getByRole("button", { name: "预览导出范围" }));
    expect(screen.getByRole("status", { name: "正在生成导出预览" })).toBeInTheDocument();
    expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "取消预览" }));
    resolvePreview(Response.json(preview()));
    await waitFor(() =>
      expect(view.requests).toContainEqual({
        path: "/api/account-workbench/exports/preview/export-preview",
        method: "DELETE",
        body: null,
      }),
    );
    expect(screen.queryByRole("region", { name: "账号导出预览" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "预览导出范围" })).toBeEnabled();
  });

  it("预览失败时保留选择且不出现可确认的旧范围", async () => {
    mount({
      previewResponse: async () =>
        Response.json(
          { code: "workbench_changed", detail: "账号已改变，请刷新后重试" },
          { status: 409 },
        ),
    });
    const user = userEvent.setup();
    await user.click(await screen.findByRole("checkbox", { name: "团队账号（ID 42）" }));
    await user.click(screen.getByRole("button", { name: "预览导出范围" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "预览导出范围" })).toBeEnabled());
    expect(screen.getByRole("checkbox", { name: "团队账号（ID 42）" })).toBeChecked();
    expect(screen.queryByRole("region", { name: "账号导出预览" })).not.toBeInTheDocument();
  });

  it("没有 OAuth 账号时显示空状态并禁止生成预览", async () => {
    mount({ accounts: [] });
    expect(await screen.findByText("暂无可导出的 OpenAI OAuth 账号")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "预览导出范围" })).toBeDisabled();
  });

  it("删除私有产物必须确认且成功后移除列表项", async () => {
    const view = mount();
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "删除私有文件 private-artifact" }));
    const dialog = screen.getByRole("dialog", { name: "删除私有文件" });
    expect(view.requests.some((item) => item.method === "DELETE")).toBe(false);
    await user.click(within(dialog).getByRole("button", { name: "删除文件" }));
    await screen.findByText("暂无私有导出文件");
    expect(view.requests).toContainEqual({
      path: "/api/account-workbench/exports/private-artifact",
      method: "DELETE",
      body: null,
    });
  });
});
