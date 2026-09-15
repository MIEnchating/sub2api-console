import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Task, WorkbenchPreview } from "@/api";
import { WorkbenchHistory } from "../components/workbench-history";

const clients: QueryClient[] = [];
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
const source: Task = {
  id: "original-import",
  skill: "account-workbench",
  operation: "account-workbench-import",
  status: "partial",
  progress: 100,
  message: "存在待核对账号",
  result: {
    items: [
      {
        index: 7,
        name: "隔离账号",
        status: "review",
        account_id: "42",
        report: { status: "failed", matched: 0, total: 24 },
      },
      { index: 2, name: "完成账号", status: "succeeded", account_id: "43" },
      { index: 3, name: "跳过账号", status: "skipped" },
      { name: "索引缺失项", status: "failed" },
    ],
  },
  created_at: "2026-09-14T00:00:00Z",
  updated_at: "2026-09-14T00:00:00Z",
};
const retryTask: Task = {
  ...source,
  id: "retry-task",
  operation: "account-workbench-retry",
  status: "queued",
  progress: 0,
  message: "等待重新处理账号",
  result: {},
};
function preview(): WorkbenchPreview {
  return {
    id: "retry-preview",
    target: "https://sub2api.example.test",
    expires_at: new Date(Date.now() + 600000).toISOString(),
    check_after_import: true,
    model: "test-model",
    errors: [],
    items: [
      {
        id: "7",
        index: 7,
        name: "隔离账号",
        email: "",
        plan_type: "plus",
        template_id: "",
        template_name: "",
        template_revision: 0,
        group_ids: [],
        duplicate: true,
        action: "check",
        account_id: "42",
      },
    ],
  };
}
type Request = { path: string; method: string; body: unknown };
function mount(previewResponse?: () => Promise<Response>): Request[] {
  const requests: Request[] = [];
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
      if (method === "DELETE") return Response.json({ deleted: true });
      if (path.endsWith("/history")) return Response.json([source]);
      if (path.endsWith("/templates")) return Response.json([]);
      if (path.endsWith("/retry-preview")) return previewResponse?.() ?? Response.json(preview());
      if (path.endsWith("/import") || path.endsWith("/tasks/retry-task"))
        return Response.json(retryTask);
      if (path.endsWith("/tasks/original-import")) return Response.json(source);
      return Response.json({ detail: "隔离测试未配置此接口" }, { status: 503 });
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <WorkbenchHistory />
    </QueryClientProvider>,
  );
  return requests;
}
async function openRetry(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await user.click(await screen.findByRole("button", { name: "查看任务 original-import" }));
  await user.click(await screen.findByRole("checkbox", { name: "重试第 8 项 隔离账号" }));
  await user.click(screen.getByRole("button", { name: "重新处理 1 项" }));
  await screen.findByRole("textbox", { name: "重试检测模型" });
}

describe("账号处理记录重试", () => {
  it("终态任务仅选择可重试的原索引并二次确认后创建重试任务", async () => {
    const requests = mount();
    const user = userEvent.setup();
    await openRetry(user);
    expect(screen.getAllByRole("checkbox", { name: /^重试第/ })).toHaveLength(1);
    await user.type(screen.getByRole("textbox", { name: "重试检测模型" }), "test-model");
    await user.click(screen.getByRole("button", { name: "预览重新处理范围" }));
    await screen.findByRole("table", { name: "账号预览" });
    expect(requests.find((item) => item.path.endsWith("/retry-preview"))?.body).toEqual({
      task_id: "original-import",
      indexes: [7],
      model: "test-model",
    });
    expect(requests.some((item) => item.path.endsWith("/import"))).toBe(false);
    await user.click(screen.getByRole("button", { name: "确认重新处理 1 个账号" }));
    const dialog = screen.getByRole("dialog", { name: "确认重新处理账号" });
    expect(dialog).toHaveTextContent("ID：42");
    await user.click(within(dialog).getByRole("button", { name: "创建重试任务" }));
    await waitFor(() =>
      expect(requests.find((item) => item.path.endsWith("/import"))?.body).toEqual({
        preview_id: "retry-preview",
        confirmed: true,
      }),
    );
    await screen.findByRole("status", { name: "等待重新处理账号" });
    expect(screen.queryByRole("region", { name: "重新处理账号" })).not.toBeInTheDocument();
  });

  it("检测模型为空时展示字段校验且不读取重试预览", async () => {
    const requests = mount();
    const user = userEvent.setup();
    await openRetry(user);
    await user.click(screen.getByRole("button", { name: "预览重新处理范围" }));
    expect(screen.getByRole("textbox", { name: "重试检测模型" })).toHaveAttribute(
      "aria-invalid",
      "true",
    );
    expect(requests.some((item) => item.path.endsWith("/retry-preview"))).toBe(false);
  });

  it("结束等待中的重试会撤销迟到预览", async () => {
    let resolvePreview: (response: Response) => void = () => undefined;
    const response = new Promise<Response>((resolve) => {
      resolvePreview = resolve;
    });
    const requests = mount(() => response);
    const user = userEvent.setup();
    await openRetry(user);
    await user.type(screen.getByRole("textbox", { name: "重试检测模型" }), "test-model");
    await user.click(screen.getByRole("button", { name: "预览重新处理范围" }));
    expect(screen.getByRole("status", { name: "正在核对原任务和账号范围" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "结束重试" }));
    resolvePreview(Response.json(preview()));
    await waitFor(() =>
      expect(requests).toContainEqual({
        path: "/api/account-workbench/preview/retry-preview",
        method: "DELETE",
        body: null,
      }),
    );
    expect(screen.queryByRole("table", { name: "账号预览" })).not.toBeInTheDocument();
  });

  it("重试预览失败时保留模型且提供重新预览入口", async () => {
    mount(async () =>
      Response.json(
        { code: "workbench_changed", detail: "隔离账号配置已变化，请核对后重试" },
        { status: 409 },
      ),
    );
    const user = userEvent.setup();
    await openRetry(user);
    await user.type(screen.getByRole("textbox", { name: "重试检测模型" }), "test-model");
    await user.click(screen.getByRole("button", { name: "预览重新处理范围" }));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "预览重新处理范围" })).toBeEnabled(),
    );
    expect(screen.getByRole("textbox", { name: "重试检测模型" })).toHaveValue("test-model");
    expect(screen.queryByRole("table", { name: "账号预览" })).not.toBeInTheDocument();
  });

  it("打开报告时通过共享只读 JSON 编辑器展示后端结构化结果", async () => {
    mount();
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "查看任务 original-import" }));
    await user.click(await screen.findByRole("button", { name: "查看 隔离账号 的报告" }));
    const dialog = screen.getByRole("dialog", { name: "隔离账号处理报告" });
    const editor = await within(dialog).findByRole("textbox", { name: "账号处理报告 JSON" });
    expect(editor).toHaveAttribute("aria-readonly", "true");
    expect(editor).toHaveTextContent('"total": 24');
  });
});
