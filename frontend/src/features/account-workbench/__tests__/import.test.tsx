import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReactElement } from "react";
import type { Task, WorkbenchPreview } from "@/api";
import { WorkbenchImport } from "../components/workbench-import";
import { WorkbenchPreviewPanel } from "../components/workbench-preview";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.restoreAllMocks();
});
function mount(component: ReactElement): void {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  render(<QueryClientProvider client={client}>{component}</QueryClientProvider>);
}
function preview(): WorkbenchPreview {
  return {
    id: "preview-1",
    expires_at: new Date(Date.now() + 600000).toISOString(),
    target: "https://sub2api.example.test",
    errors: [],
    check_after_import: false,
    model: "",
    items: [
      {
        id: "0",
        index: 0,
        name: "示例账号",
        email: "test@example.test",
        plan_type: "plus",
        template_id: "template-1",
        template_name: "Plus 配置",
        template_revision: 1,
        group_ids: ["7"],
        duplicate: false,
      },
    ],
  };
}
const task: Task = {
  id: "import-task",
  skill: "account-workbench",
  operation: "account-workbench-import",
  status: "queued",
  progress: 0,
  message: "等待导入账号",
  result: {},
  created_at: "2026-09-14T00:00:00Z",
  updated_at: "2026-09-14T00:00:00Z",
};

describe("账号导入预览与确认", () => {
  it("输入账号后先展示服务端预览，二次确认仅提交预览 ID 并清空凭据", async () => {
    const requests: Array<{ path: string; method: string; body: unknown }> = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
        const path = String(input);
        const method = init?.method ?? "GET";
        requests.push({
          path,
          method,
          body: init?.body ? (JSON.parse(String(init.body)) as unknown) : null,
        });
        if (path.endsWith("/templates")) return Response.json([]);
        if (path.endsWith("/preview") && method === "POST") return Response.json(preview());
        if (path.endsWith("/import")) return Response.json(task);
        if (path.includes("/tasks/")) return Response.json(task);
        return Response.json({ deleted: true });
      }),
    );
    const user = userEvent.setup();
    mount(<WorkbenchImport />);
    await user.click(await screen.findByRole("textbox", { name: "账号内容" }));
    await user.paste("rt_private_token");
    await user.click(screen.getByRole("button", { name: "解析并预览" }));
    expect(await screen.findByRole("table", { name: "账号预览" })).toBeInTheDocument();
    expect(requests.some((request) => request.path.endsWith("/import"))).toBe(false);
    await user.click(screen.getByRole("button", { name: "确认导入 1 个账号" }));
    const dialog = screen.getByRole("dialog", { name: "确认批量导入账号" });
    expect(dialog).toHaveTextContent("https://sub2api.example.test");
    expect(dialog).toHaveTextContent("Plus 配置");
    expect(dialog).toHaveTextContent("分组 ID：7");
    await user.click(within(dialog).getByRole("button", { name: "创建导入任务" }));
    await waitFor(() =>
      expect(screen.getByRole("textbox", { name: "账号内容" })).toHaveTextContent(
        /^账号 JSON 或 rt_ 刷新令牌$/,
      ),
    );
    expect(requests.find((request) => request.path.endsWith("/import"))?.body).toEqual({
      preview_id: "preview-1",
      confirmed: true,
    });
    expect(screen.getByRole("status", { name: "等待导入账号" })).toBeInTheDocument();
    expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  });

  it("预览后修改账号内容会关闭旧预览并撤销服务端预览", async () => {
    const revoked: string[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
        const path = String(input);
        if (path.endsWith("/templates")) return Response.json([]);
        if (init?.method === "DELETE") {
          revoked.push(path);
          return Response.json({ deleted: true });
        }
        return Response.json(preview());
      }),
    );
    const user = userEvent.setup();
    mount(<WorkbenchImport />);
    const input = await screen.findByRole("textbox", { name: "账号内容" });
    await user.click(input);
    await user.paste("rt_original");
    await user.click(screen.getByRole("button", { name: "解析并预览" }));
    await screen.findByRole("table", { name: "账号预览" });
    await user.click(input);
    await user.paste("_changed");
    expect(screen.queryByRole("table", { name: "账号预览" })).not.toBeInTheDocument();
    expect(revoked).toEqual(["/api/account-workbench/preview/preview-1"]);
  });

  it("已过期预览禁用导入并保留关闭入口", async () => {
    const value = preview();
    value.expires_at = "2000-01-01T00:00:00Z";
    const close = vi.fn();
    mount(
      <WorkbenchPreviewPanel
        preview={value}
        pending={false}
        onConfirm={vi.fn()}
        onDiscard={close}
      />,
    );
    expect(screen.getByRole("button", { name: "确认导入 1 个账号" })).toBeDisabled();
    await userEvent.setup().click(screen.getByRole("button", { name: "关闭预览" }));
    expect(close).toHaveBeenCalledOnce();
  });

  it("已有账号预览显示稳定 ID，并在二次确认列出将更新的凭据目标", async () => {
    const value = preview();
    value.items[0].duplicate = true;
    value.items[0].account_id = "42";
    mount(
      <WorkbenchPreviewPanel
        preview={value}
        pending={false}
        onConfirm={vi.fn()}
        onDiscard={vi.fn()}
      />,
    );
    expect(screen.getByRole("cell", { name: "更新凭据（ID 42）" })).toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole("button", { name: "确认导入 1 个账号" }));
    expect(screen.getByRole("dialog")).toHaveTextContent("更新已有账号 1 个（ID：42）");
  });
});
