import type { ReactElement } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Task, WorkbenchOAuthSession, WorkbenchPreview } from "@/api";
import { WorkbenchImport } from "../components/workbench-import";
import { WorkbenchPreviewPanel } from "../components/workbench-preview";
import { WorkbenchOAuth } from "../components/workbench-oauth";

let client: QueryClient;
type Request = { path: string; method: string; body: unknown };
const task: Task = {
  id: "convert-task",
  skill: "account-workbench",
  operation: "account-workbench-convert",
  status: "queued",
  progress: 0,
  message: "等待生成私有账号文件",
  result: {},
  created_at: "2026-09-14T00:00:00Z",
  updated_at: "2026-09-14T00:00:00Z",
};
function preview(id = "preview-one", exportOnly = true): WorkbenchPreview {
  return {
    id,
    export_only: exportOnly,
    target: "https://sub2api.example.test",
    expires_at: new Date(Date.now() + 600000).toISOString(),
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
        template_id: "team",
        template_name: "团队模板",
        template_revision: 2,
        group_ids: ["7"],
        duplicate: false,
      },
    ],
  };
}
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
function mount(
  component: ReactElement,
  handler?: (request: Request) => Promise<Response | undefined>,
): { requests: Request[]; rerender: (element: ReactElement) => void } {
  const requests: Request[] = [];
  let previews = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input, init) => {
      const request: Request = {
        path: String(input),
        method: init?.method ?? "GET",
        body: init?.body ? (JSON.parse(String(init.body)) as unknown) : null,
      };
      requests.push(request);
      const response = await handler?.(request);
      if (response) return response;
      if (request.method === "DELETE") return Response.json({ deleted: true, cancelled: true });
      if (request.path.endsWith("/templates")) return Response.json([]);
      if (request.path.endsWith("/preview")) {
        previews += 1;
        return Response.json(preview(`preview-${previews}`));
      }
      return Response.json(task);
    }),
  );
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const view = render(<QueryClientProvider client={client}>{component}</QueryClientProvider>);
  return {
    requests,
    rerender: (element) =>
      view.rerender(<QueryClientProvider client={client}>{element}</QueryClientProvider>),
  };
}
async function confirmConversion(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await user.click(screen.getByRole("button", { name: "生成私有 JSON 文件" }));
  await user.click(
    within(screen.getByRole("dialog", { name: "确认生成私有账号文件" })).getByRole("button", {
      name: "创建私有转换任务",
    }),
  );
}

describe("输入转换为私有JSON", () => {
  it("上一预览转换成功后替换预览ID可以确认新转换", async () => {
    const callbacks = { pending: false, onConfirm: vi.fn(), onDiscard: vi.fn() };
    const view = mount(<WorkbenchPreviewPanel preview={preview("first")} {...callbacks} />);
    const user = userEvent.setup();
    await confirmConversion(user);
    await screen.findByRole("status", { name: "等待生成私有账号文件" });
    expect(screen.getByRole("button", { name: "生成私有 JSON 文件" })).toBeDisabled();
    view.rerender(<WorkbenchPreviewPanel preview={preview("second")} {...callbacks} />);
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "生成私有 JSON 文件" })).toBeEnabled(),
    );
    await confirmConversion(user);
    await waitFor(() =>
      expect(view.requests.filter((request) => request.path.endsWith("/from-input"))).toHaveLength(
        2,
      ),
    );
    expect(
      view.requests
        .filter((request) => request.path.endsWith("/from-input"))
        .map((request) => request.body),
    ).toEqual([
      { preview_id: "first", confirmed: true },
      { preview_id: "second", confirmed: true },
    ]);
  });

  it("转换成功清空输入凭据并可开始下一次转换", async () => {
    const view = mount(<WorkbenchImport output="export" />);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("textbox", { name: "账号内容" }, { timeout: 5000 }));
    await user.paste("rt_private_first");
    expect(screen.queryByRole("textbox", { name: "检测模型" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "解析并预览" }));
    await screen.findByRole("table", { name: "账号预览" });
    expect(view.requests.find((request) => request.path.endsWith("/preview"))?.body).toMatchObject({
      export_only: true,
    });
    expect(screen.queryByRole("button", { name: "确认导入 1 个账号" })).not.toBeInTheDocument();
    await confirmConversion(user);
    await waitFor(() =>
      expect(screen.getByRole("textbox", { name: "账号内容" })).toHaveTextContent(
        /^账号 JSON 或 rt_ 刷新令牌$/,
      ),
    );
    expect(view.requests.some((request) => request.path.endsWith("/import"))).toBe(false);
    await user.click(screen.getByRole("textbox", { name: "账号内容" }));
    await user.paste("rt_private_second");
    await user.click(screen.getByRole("button", { name: "解析并预览" }));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "生成私有 JSON 文件" })).toBeEnabled(),
    );
  });

  it("生成文件前需确认范围且取消确认不会发送转换请求", async () => {
    const view = mount(
      <WorkbenchPreviewPanel
        preview={preview()}
        pending={false}
        onConfirm={vi.fn()}
        onDiscard={vi.fn()}
      />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "生成私有 JSON 文件" }));
    const dialog = screen.getByRole("dialog", { name: "确认生成私有账号文件" });
    expect(dialog).toHaveTextContent("1 项账号");
    expect(dialog).toHaveTextContent("不执行模型检测");
    await user.click(within(dialog).getByRole("button", { name: "取消" }));
    expect(view.requests.some((request) => request.path.endsWith("/from-input"))).toBe(false);
    expect(screen.getByRole("button", { name: "生成私有 JSON 文件" })).toBeEnabled();
  });

  it("转换失败后保留凭据，重新预览后可再次确认", async () => {
    let fail = true;
    mount(<WorkbenchImport output="export" />, async (request) => {
      if (request.path.endsWith("/from-input") && fail)
        return Response.json({ detail: "任务队列暂时已满" }, { status: 503 });
    });
    const user = userEvent.setup();
    await user.click(await screen.findByRole("textbox", { name: "账号内容" }, { timeout: 5000 }));
    await user.paste("rt_private_retry");
    await user.click(screen.getByRole("button", { name: "解析并预览" }));
    await screen.findByRole("table", { name: "账号预览" });
    await confirmConversion(user);
    await waitFor(() =>
      expect(
        screen.queryByRole("table", { name: "账号预览", hidden: true }),
      ).not.toBeInTheDocument(),
    );
    expect(screen.getByRole("textbox", { name: "账号内容" })).toHaveTextContent("rt_private_retry");
    fail = false;
    await user.click(screen.getByRole("button", { name: "解析并预览" }));
    await screen.findByRole("table", { name: "账号预览" });
    await confirmConversion(user);
    await screen.findByRole("status", { name: "等待生成私有账号文件" });
  });

  it("转换创建期间禁用输入和重复解析，成功后恢复", async () => {
    let resolve: (value: Response) => void = () => undefined;
    const pending = new Promise<Response>((done) => {
      resolve = done;
    });
    mount(<WorkbenchImport output="export" />, async (request) =>
      request.path.endsWith("/from-input") ? pending : undefined,
    );
    const user = userEvent.setup();
    await user.click(await screen.findByRole("textbox", { name: "账号内容" }, { timeout: 5000 }));
    await user.paste("rt_private_waiting");
    await user.click(screen.getByRole("button", { name: "解析并预览" }));
    await screen.findByRole("table", { name: "账号预览" });
    await confirmConversion(user);
    expect(screen.getByLabelText("账号内容")).toHaveAttribute("aria-disabled", "true");
    expect(screen.getByRole("button", { name: "解析并预览", hidden: true })).toBeDisabled();
    await act(async () => {
      resolve(Response.json(task));
    });
    await waitFor(() =>
      expect(screen.getByRole("textbox", { name: "账号内容" })).toHaveAttribute(
        "aria-disabled",
        "false",
      ),
    );
  });

  it("有无效输入时禁止转换但保留关闭入口", () => {
    const value = preview();
    value.errors = [{ index: 1, message: "账号内容不完整" }];
    mount(
      <WorkbenchPreviewPanel
        preview={value}
        pending={false}
        onConfirm={vi.fn()}
        onDiscard={vi.fn()}
      />,
    );
    expect(screen.getByRole("button", { name: "生成私有 JSON 文件" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "关闭预览" })).toBeEnabled();
  });

  it("旧预览的迟到转换结果不会清理新预览", async () => {
    let resolve: (value: Response) => void = () => undefined;
    const pending = new Promise<Response>((done) => {
      resolve = done;
    });
    const converted = vi.fn();
    const callbacks = {
      pending: false,
      onConfirm: vi.fn(),
      onDiscard: vi.fn(),
      onConverted: converted,
    };
    const view = mount(
      <WorkbenchPreviewPanel preview={preview("first")} {...callbacks} />,
      async (request) => (request.path.endsWith("/from-input") ? pending : undefined),
    );
    const user = userEvent.setup();
    await confirmConversion(user);
    view.rerender(<WorkbenchPreviewPanel preview={preview("second")} {...callbacks} />);
    await act(async () => {
      resolve(Response.json(task));
    });
    expect(converted).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "生成私有 JSON 文件" })).toBeEnabled();
  });

  it("过期或空范围的转换预览保留关闭入口且不能提交", () => {
    const callbacks = { pending: false, onConfirm: vi.fn(), onDiscard: vi.fn() };
    const expired = { ...preview(), expires_at: "2000-01-01T00:00:00Z" };
    const view = mount(<WorkbenchPreviewPanel preview={expired} {...callbacks} />);
    expect(screen.getByRole("button", { name: "生成私有 JSON 文件" })).toBeDisabled();
    view.rerender(
      <WorkbenchPreviewPanel preview={{ ...preview("empty"), items: [] }} {...callbacks} />,
    );
    expect(screen.getByRole("button", { name: "生成私有 JSON 文件" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "关闭预览" })).toBeEnabled();
  });

  it("授权账号转换成功后结束授权会话且不创建线上账号", async () => {
    const session: WorkbenchOAuthSession = {
      id: "oauth-convert",
      task_id: "oauth-task",
      host: "auth.openai.com",
      status: "authorized",
      message: "授权成功",
      expires_at: new Date(Date.now() + 600000).toISOString(),
      width: 1100,
      height: 760,
    };
    const view = mount(<WorkbenchOAuth />, async (request) => {
      if (
        request.path.endsWith("/oauth") ||
        (request.path.endsWith("/oauth/oauth-convert") && request.method === "GET")
      )
        return Response.json(session);
      if (request.path.endsWith("/oauth/oauth-convert/preview"))
        return Response.json(preview("oauth-preview", false));
    });
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "开始授权登录" }));
    await user.click(await screen.findByRole("button", { name: "预览授权账号" }));
    await screen.findByRole("table", { name: "账号预览" });
    await confirmConversion(user);
    await waitFor(() =>
      expect(
        view.requests.some(
          (request) => request.path.endsWith("/oauth/oauth-convert") && request.method === "DELETE",
        ),
      ).toBe(true),
    );
    expect(screen.queryByRole("button", { name: "结束授权" })).not.toBeInTheDocument();
    expect(view.requests.some((request) => request.path.endsWith("/import"))).toBe(false);
  });
});
