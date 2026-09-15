import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type {
  Task,
  WorkbenchPreview,
  WorkbenchRunPreview,
  WorkbenchRunRow,
  WorkbenchRunView,
} from "@/api";
import { WorkbenchMixed } from "../components/workbench-mixed";
import { WorkbenchMixedPreview } from "../components/workbench-mixed-preview";
import { workbenchKeys } from "../constants";

let client: QueryClient;
type Request = { path: string; method: string; body: unknown };
const mixedContent =
  '{"access_token":"private-token"}\nrt_private-refresh\noperator@example.test----private-password';
const rows: WorkbenchRunRow[] = [
  {
    index: 0,
    kind: "codex_json",
    name: "JSON 账号",
    has_password: false,
    has_totp: false,
    has_proxy: false,
    status: "queued",
    message: "等待处理",
  },
  {
    index: 1,
    kind: "refresh_token",
    name: "RT 账号",
    has_password: false,
    has_totp: false,
    has_proxy: false,
    status: "queued",
    message: "等待处理",
  },
  {
    index: 2,
    kind: "oauth_login",
    name: "登录账号",
    email: "operator@example.test",
    has_password: true,
    has_totp: false,
    has_proxy: true,
    status: "queued",
    message: "等待处理",
  },
];
function preview(exportOnly = false): WorkbenchRunPreview {
  return {
    id: "mixed-preview",
    target: "https://sub2api.example.test",
    expires_at: new Date(Date.now() + 600000).toISOString(),
    export_only: exportOnly,
    items: rows,
    errors: [],
  };
}
function run(exportOnly = false): WorkbenchRunView {
  return {
    id: "mixed-run",
    task_id: "mixed-task",
    status: "ready",
    message: "账号准备完成",
    expires_at: new Date(Date.now() + 600000).toISOString(),
    export_only: exportOnly,
    available: 3,
    items: rows.map((row) => ({ ...row, status: "succeeded" })),
    errors: [],
  };
}
function resultPreview(exportOnly = false): WorkbenchPreview {
  return {
    id: "mixed-result",
    target: "https://sub2api.example.test",
    expires_at: new Date(Date.now() + 600000).toISOString(),
    export_only: exportOnly,
    check_after_import: false,
    model: "",
    errors: [],
    items: [
      {
        id: "2",
        index: 2,
        name: "登录账号",
        email: "operator@example.test",
        plan_type: "plus",
        template_id: "",
        template_name: "默认",
        template_revision: 0,
        group_ids: [],
        duplicate: false,
      },
    ],
  };
}
const task: Task = {
  id: "result-task",
  skill: "account-workbench",
  operation: "account-workbench-import",
  status: "queued",
  progress: 0,
  message: "等待处理混合账号",
  result: {},
  created_at: "2026-09-14T00:00:00Z",
  updated_at: "2026-09-14T00:00:00Z",
};
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  client?.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
function mount(handler?: (request: Request) => Promise<Response | undefined>): {
  requests: Request[];
  unmount: () => void;
} {
  const requests: Request[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input, init) => {
      const request = {
        path: String(input),
        method: init?.method ?? "GET",
        body: init?.body ? (JSON.parse(String(init.body)) as unknown) : null,
      };
      requests.push(request);
      const response = await handler?.(request);
      if (response) return response;
      if (request.method === "DELETE") return Response.json({ deleted: true, cancelled: true });
      if (request.path.endsWith("/templates")) return Response.json([]);
      if (request.path.endsWith("/runs/preview")) return Response.json(preview());
      if (request.path.endsWith("/runs/mixed-run/preview")) return Response.json(resultPreview());
      if (
        request.path.endsWith("/import") ||
        request.path.endsWith("/exports/from-input") ||
        request.path.endsWith("/tasks/result-task")
      )
        return Response.json(task);
      return Response.json(run());
    }),
  );
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const view = render(
    <QueryClientProvider client={client}>
      <WorkbenchMixed />
    </QueryClientProvider>,
  );
  return { requests, unmount: view.unmount };
}
async function parse(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await user.click(await screen.findByRole("textbox", { name: "混合账号内容" }));
  await user.paste(mixedContent);
  await user.click(screen.getByRole("button", { name: "解析混合运行范围" }));
}
async function start(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await user.click(await screen.findByRole("button", { name: "确认处理 3 项" }));
  await user.click(screen.getByRole("button", { name: "开始混合运行" }));
  await screen.findByRole("region", { name: "混合运行进度" });
}

describe("混合运行", () => {
  it("JSON、RT和登录行原文只提交一次，范围确认后仅用预览ID开始", async () => {
    const view = mount();
    const user = userEvent.setup();
    await parse(user);
    expect(await screen.findByRole("region", { name: "混合运行预览" })).toHaveTextContent(
      "共 3 项",
    );
    expect(screen.queryByRole("textbox", { name: "混合账号内容" })).not.toBeInTheDocument();
    expect(
      view.requests.find((request) => request.path.endsWith("/runs/preview"))?.body,
    ).toMatchObject({
      content: mixedContent,
      export_only: false,
      check_after_import: false,
      model: "",
    });
    expect(
      JSON.stringify(
        client
          .getMutationCache()
          .getAll()
          .map((mutation) => mutation.state.variables),
      ),
    ).not.toMatch(/private-token|private-password|private-refresh/);
    expect(
      view.requests.some((request) => request.path.endsWith("/runs") && request.method === "POST"),
    ).toBe(false);
    await start(user);
    expect(
      view.requests.find((request) => request.path.endsWith("/runs") && request.method === "POST")
        ?.body,
    ).toEqual({ preview_id: "mixed-preview", confirmed: true });
  });

  it("准备完成后读取固定选项结果，再经导入确认创建任务", async () => {
    const view = mount();
    const user = userEvent.setup();
    await parse(user);
    await start(user);
    await user.click(screen.getByRole("button", { name: "预览可导入账号" }));
    await user.click(await screen.findByRole("button", { name: "确认导入 1 个账号" }));
    expect(view.requests.some((request) => request.path.endsWith("/import"))).toBe(false);
    await user.click(
      within(screen.getByRole("dialog", { name: "确认批量导入账号" })).getByRole("button", {
        name: "创建导入任务",
      }),
    );
    await screen.findByRole("status", { name: "等待处理混合账号" });
    expect(
      view.requests.find((request) => request.path.endsWith("/runs/mixed-run/preview"))?.body ??
        null,
    ).toEqual(null);
    expect(view.requests.find((request) => request.path.endsWith("/import"))?.body).toEqual({
      preview_id: "mixed-result",
      confirmed: true,
    });
  });

  it("私有转换模式隐藏检测和导入，只创建私有转换任务", async () => {
    const view = mount(async (request) => {
      if (request.path.endsWith("/runs/preview")) return Response.json(preview(true));
      if (request.path.endsWith("/runs/mixed-run/preview"))
        return Response.json(resultPreview(true));
      if (request.path.endsWith("/runs") || request.path.endsWith("/runs/mixed-run"))
        return Response.json(run(true));
    });
    const user = userEvent.setup();
    await user.click(await screen.findByRole("combobox", { name: "处理方式" }));
    await user.click(screen.getByRole("option", { name: "生成私有 JSON" }));
    expect(screen.queryByRole("textbox", { name: "检测模型" })).not.toBeInTheDocument();
    await parse(user);
    await start(user);
    await user.click(screen.getByRole("button", { name: "预览私有转换结果" }));
    await user.click(await screen.findByRole("button", { name: "生成私有 JSON 文件" }));
    expect(screen.queryByRole("button", { name: "确认导入 1 个账号" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "创建私有转换任务" }));
    await waitFor(() =>
      expect(view.requests.some((request) => request.path.endsWith("/exports/from-input"))).toBe(
        true,
      ),
    );
    expect(view.requests.some((request) => request.path.endsWith("/import"))).toBe(false);
  });

  it("跨来源身份冲突展示原始条目问题并阻止最终预览", async () => {
    mount(async (request) =>
      request.path.endsWith("/runs") || request.path.endsWith("/runs/mixed-run")
        ? Response.json({ ...run(), errors: [{ index: 2, message: "身份与第 1 项重复" }] })
        : undefined,
    );
    const user = userEvent.setup();
    await parse(user);
    await start(user);
    expect(screen.getByRole("list", { name: "混合运行问题" })).toHaveTextContent(
      "第 3 项：身份与第 1 项重复",
    );
    expect(screen.queryByRole("button", { name: "预览可导入账号" })).not.toBeInTheDocument();
  });

  it("部分登录失败时仍可预览成功项且保留各项状态", async () => {
    mount(async (request) =>
      request.path.endsWith("/runs") || request.path.endsWith("/runs/mixed-run")
        ? Response.json({
            ...run(),
            available: 1,
            items: [
              { ...rows[0], status: "succeeded" },
              { ...rows[2], status: "failed", message: "授权未完成" },
            ],
          })
        : undefined,
    );
    const user = userEvent.setup();
    await parse(user);
    await start(user);
    expect(screen.getByRole("table", { name: "混合运行账号" })).toHaveTextContent("授权未完成");
    expect(screen.getByRole("button", { name: "预览可导入账号" })).toBeEnabled();
  });

  it("取消尚未完成的范围解析会撤销迟到预览且不恢复原凭据", async () => {
    let resolve: (response: Response) => void = () => undefined;
    const pending = new Promise<Response>((done) => {
      resolve = done;
    });
    const view = mount(async (request) =>
      request.path.endsWith("/runs/preview") ? pending : undefined,
    );
    const user = userEvent.setup();
    await parse(user);
    await user.click(await screen.findByRole("button", { name: "取消解析" }));
    await act(async () => {
      resolve(Response.json(preview()));
    });
    await waitFor(() =>
      expect(
        view.requests.some(
          (request) =>
            request.method === "DELETE" && request.path.endsWith("/runs/preview/mixed-preview"),
        ),
      ).toBe(true),
    );
    expect(await screen.findByRole("textbox", { name: "混合账号内容" })).toHaveValue("");
  });

  it("启动响应迟到且界面卸载时清理新运行", async () => {
    let resolve: (response: Response) => void = () => undefined;
    const pending = new Promise<Response>((done) => {
      resolve = done;
    });
    const view = mount(async (request) =>
      request.path.endsWith("/runs") && request.method === "POST" ? pending : undefined,
    );
    const user = userEvent.setup();
    await parse(user);
    await user.click(await screen.findByRole("button", { name: "确认处理 3 项" }));
    await user.click(screen.getByRole("button", { name: "开始混合运行" }));
    view.unmount();
    await act(async () => {
      resolve(Response.json(run()));
    });
    await waitFor(() =>
      expect(
        view.requests.some(
          (request) => request.method === "DELETE" && request.path.endsWith("/runs/mixed-run"),
        ),
      ).toBe(true),
    );
  });

  it("启用恢复时启动响应迟到且仅离开页面会保留服务器批次", async () => {
    let resolve: (response: Response) => void = () => undefined;
    const pending = new Promise<Response>((done) => {
      resolve = done;
    });
    const view = mount(async (request) =>
      request.path.endsWith("/runs") && request.method === "POST" ? pending : undefined,
    );
    const user = userEvent.setup();
    await parse(user);
    await user.click(await screen.findByRole("button", { name: "确认处理 3 项" }));
    await user.click(screen.getByRole("button", { name: "开始混合运行" }));
    view.unmount();
    await act(async () => {
      resolve(Response.json({ ...run(), recovery_enabled: true }));
    });
    expect(
      view.requests.some(
        (request) => request.method === "DELETE" && request.path.endsWith("/runs/mixed-run"),
      ),
    ).toBe(false);
  });

  it("启动请求待返回时禁用确认弹窗取消，离开页面仍按恢复授权保留批次", async () => {
    let resolve: (response: Response) => void = () => undefined;
    const pending = new Promise<Response>((done) => {
      resolve = done;
    });
    const view = mount(async (request) =>
      request.path.endsWith("/runs") && request.method === "POST" ? pending : undefined,
    );
    const user = userEvent.setup();
    await parse(user);
    await user.click(await screen.findByRole("button", { name: "确认处理 3 项" }));
    await user.click(screen.getByRole("button", { name: "开始混合运行" }));
    expect(
      within(screen.getByRole("dialog", { name: "确认开始混合运行" })).getByRole("button", {
        name: "取消",
      }),
    ).toBeDisabled();
    view.unmount();
    await act(async () => {
      resolve(Response.json({ ...run(), recovery_enabled: true }));
    });
    expect(
      view.requests.some(
        (request) => request.method === "DELETE" && request.path.endsWith("/runs/mixed-run"),
      ),
    ).toBe(false);
  });

  it("运行查询失败保留缓存与结束入口，重读恢复后重新开放最终预览", async () => {
    let fail = true;
    mount(async (request) =>
      request.path.endsWith("/runs/mixed-run") && request.method === "GET" && fail
        ? Response.json({ detail: "暂时不可读" }, { status: 503 })
        : undefined,
    );
    const user = userEvent.setup();
    await parse(user);
    await start(user);
    const retry = await screen.findByRole("button", { name: "重新读取" });
    expect(screen.getByRole("button", { name: "结束混合运行" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "预览可导入账号" })).toBeDisabled();
    fail = false;
    await user.click(retry);
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "预览可导入账号" })).toBeEnabled(),
    );
  });

  it("结束运行必须确认且完成后清除运行查询", async () => {
    const view = mount();
    const user = userEvent.setup();
    await parse(user);
    await start(user);
    await user.click(screen.getByRole("button", { name: "结束混合运行" }));
    expect(
      view.requests.some(
        (request) => request.method === "DELETE" && request.path.endsWith("/runs/mixed-run"),
      ),
    ).toBe(false);
    await user.click(screen.getByRole("button", { name: "结束并清除混合结果" }));
    await waitFor(() =>
      expect(screen.queryByRole("region", { name: "混合运行进度" })).not.toBeInTheDocument(),
    );
    expect(client.getQueryData(workbenchKeys.run("mixed-run"))).toBeUndefined();
  });

  it("最终预览读取等待时仍可结束运行并清理迟到结果", async () => {
    let resolve: (response: Response) => void = () => undefined;
    const pending = new Promise<Response>((done) => {
      resolve = done;
    });
    const view = mount(async (request) =>
      request.path.endsWith("/runs/mixed-run/preview") ? pending : undefined,
    );
    const user = userEvent.setup();
    await parse(user);
    await start(user);
    await user.click(screen.getByRole("button", { name: "预览可导入账号" }));
    await screen.findByText("正在生成混合运行结果预览");
    expect(screen.getByRole("button", { name: "结束混合运行" })).toBeEnabled();
    await user.click(screen.getByRole("button", { name: "结束混合运行" }));
    await user.click(screen.getByRole("button", { name: "结束并清除混合结果" }));
    await waitFor(() =>
      expect(screen.queryByRole("region", { name: "混合运行进度" })).not.toBeInTheDocument(),
    );
    await act(async () => {
      resolve(Response.json(resultPreview()));
    });
    await waitFor(() =>
      expect(
        view.requests.some(
          (request) =>
            request.method === "DELETE" && request.path.endsWith("/preview/mixed-result"),
        ),
      ).toBe(true),
    );
  });

  it("无效和过期预览不能启动但可关闭", () => {
    const onStart = vi.fn();
    const onClose = vi.fn();
    const value = {
      ...preview(),
      expires_at: "2000-01-01T00:00:00Z",
      errors: [{ index: 1, message: "无效 RT" }],
    };
    render(
      <WorkbenchMixedPreview preview={value} pending={false} onStart={onStart} onClose={onClose} />,
    );
    expect(screen.getByRole("button", { name: "确认处理 3 项" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "关闭混合运行预览" })).toBeEnabled();
    expect(screen.getByRole("list", { name: "混合输入问题" })).toHaveTextContent(
      "第 2 项：无效 RT",
    );
  });
});
