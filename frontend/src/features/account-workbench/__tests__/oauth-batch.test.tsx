import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type {
  Task,
  WorkbenchOAuthBatch,
  WorkbenchOAuthBatchPreview,
  WorkbenchOAuthBatchRow,
  WorkbenchPreview,
} from "@/api";
import { WorkbenchOAuthBatch as BatchPanel } from "../components/workbench-oauth-batch";
import { workbenchKeys } from "../constants";

let client: QueryClient;
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  client?.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
type Request = { path: string; method: string; body: unknown };
function row(index = 0): WorkbenchOAuthBatchRow {
  return {
    index,
    email: `user${index}@example.test`,
    has_password: true,
    has_totp: false,
    status: "queued",
    message: "等待授权",
  };
}
function preview(): WorkbenchOAuthBatchPreview {
  return {
    id: "batch-preview",
    target: "https://sub2api.example.test",
    expires_at: new Date(Date.now() + 600000).toISOString(),
    items: [row(), row(1)],
    errors: [],
  };
}
function batch(): WorkbenchOAuthBatch {
  return {
    id: "batch-1",
    task_id: "batch-task",
    status: "queued",
    expires_at: new Date(Date.now() + 600000).toISOString(),
    available: 0,
    message: "等待批量授权",
    items: [row(), row(1)],
  };
}
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
      if (request.method === "DELETE") return Response.json({ cancelled: true, deleted: true });
      if (request.path.endsWith("/oauth-batches/preview")) return Response.json(preview());
      if (request.path.endsWith("/templates")) return Response.json([]);
      return Response.json(batch());
    }),
  );
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const view = render(
    <QueryClientProvider client={client}>
      <BatchPanel />
    </QueryClientProvider>,
  );
  return { requests, unmount: view.unmount };
}
async function parse(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await user.click(screen.getByRole("button", { name: "填写批量账号" }));
  await user.type(
    screen.getByRole("textbox", { name: "批量授权内容" }),
    "user0@example.test----private-password\nuser1@example.test",
  );
  await user.click(screen.getByRole("button", { name: "解析授权账号" }));
}
async function start(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await parse(user);
  await user.click(await screen.findByRole("button", { name: "确认授权 2 个账号" }));
  await user.click(
    within(screen.getByRole("dialog", { name: "确认批量授权" })).getByRole("button", {
      name: "开始批量授权",
    }),
  );
  await screen.findByRole("region", { name: "批量授权进度" });
}

describe("批量授权流程", () => {
  it("解析后清除凭据表单且确认范围后仅用预览ID启动", async () => {
    const view = mount();
    const user = userEvent.setup();
    await parse(user);
    await screen.findByRole("region", { name: "批量授权预览" });
    expect(screen.queryByRole("dialog", { name: "批量授权账号" })).not.toBeInTheDocument();
    expect(
      JSON.stringify(
        client
          .getMutationCache()
          .getAll()
          .map((mutation) => mutation.state.variables),
      ),
    ).not.toContain("private-password");
    expect(
      view.requests.some((item) => item.path.endsWith("/oauth-batches") && item.method === "POST"),
    ).toBe(false);
    await user.click(screen.getByRole("button", { name: "确认授权 2 个账号" }));
    expect(screen.getByRole("dialog", { name: "确认批量授权" })).toHaveTextContent(
      "https://sub2api.example.test",
    );
    await user.click(screen.getByRole("button", { name: "开始批量授权" }));
    await screen.findByRole("region", { name: "批量授权进度" });
    expect(
      view.requests.find((item) => item.path.endsWith("/oauth-batches") && item.method === "POST")
        ?.body,
    ).toEqual({ preview_id: "batch-preview", confirmed: true });
    expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  });

  it("取消尚未完成的解析会撤销迟到预览", async () => {
    let resolve: (value: Response) => void = () => undefined;
    const response = new Promise<Response>((done) => {
      resolve = done;
    });
    const view = mount(async (request) =>
      request.path.endsWith("/oauth-batches/preview") ? response : undefined,
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
          (item) =>
            item.path.endsWith("/oauth-batches/preview/batch-preview") && item.method === "DELETE",
        ),
      ).toBe(true),
    );
    expect(screen.queryByRole("region", { name: "批量授权预览" })).not.toBeInTheDocument();
  });

  it("无效账号返回逐项问题且不能开始批次", async () => {
    mount(async (request) =>
      request.path.endsWith("/oauth-batches/preview")
        ? Response.json({
            id: "",
            target: "",
            expires_at: "",
            items: [],
            errors: [{ index: 1, message: "邮箱格式无效，请填写完整登录邮箱" }],
          })
        : undefined,
    );
    const user = userEvent.setup();
    await parse(user);
    expect(await screen.findByRole("list", { name: "授权输入问题" })).toHaveTextContent(
      "第 2 项：邮箱格式无效",
    );
    expect(screen.getByRole("button", { name: "确认授权 0 个账号" })).toBeDisabled();
  });

  it("结束批量授权需要确认并清除会话查询", async () => {
    const view = mount();
    const user = userEvent.setup();
    await start(user);
    await user.click(screen.getByRole("button", { name: "结束批量授权" }));
    expect(
      view.requests.some(
        (item) => item.path.endsWith("/oauth-batches/batch-1") && item.method === "DELETE",
      ),
    ).toBe(false);
    await user.click(screen.getByRole("button", { name: "结束并清除本批" }));
    await waitFor(() =>
      expect(screen.queryByRole("region", { name: "批量授权进度" })).not.toBeInTheDocument(),
    );
    expect(client.getQueryData(workbenchKeys.batch("batch-1"))).toBeUndefined();
    expect(
      view.requests.some(
        (item) => item.path.endsWith("/oauth-batches/batch-1") && item.method === "DELETE",
      ),
    ).toBe(true);
  });

  it("卸载批次页面会清除后端批次和浏览器查询", async () => {
    const view = mount();
    const user = userEvent.setup();
    await start(user);
    view.unmount();
    await waitFor(() =>
      expect(
        view.requests.some(
          (item) => item.path.endsWith("/oauth-batches/batch-1") && item.method === "DELETE",
        ),
      ).toBe(true),
    );
  });

  it("批次查询失败后保留结束入口并允许重新读取", async () => {
    let fail = true;
    mount(async (request) => {
      if (request.path.endsWith("/oauth-batches/batch-1") && request.method === "GET" && fail)
        return Response.json({ detail: "批次暂时不可读" }, { status: 503 });
    });
    const user = userEvent.setup();
    await start(user);
    const retry = await screen.findByRole("button", { name: "重新读取" });
    expect(screen.getByRole("button", { name: "结束批量授权" })).toBeEnabled();
    fail = false;
    await user.click(retry);
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: "重新读取" })).not.toBeInTheDocument(),
    );
  });

  it("批次启动未返回时离开页面会取消迟到批次", async () => {
    let resolve: (value: Response) => void = () => undefined;
    const response = new Promise<Response>((done) => {
      resolve = done;
    });
    const view = mount(async (request) =>
      request.path.endsWith("/oauth-batches") && request.method === "POST" ? response : undefined,
    );
    const user = userEvent.setup();
    await parse(user);
    await user.click(await screen.findByRole("button", { name: "确认授权 2 个账号" }));
    await user.click(screen.getByRole("button", { name: "开始批量授权" }));
    await screen.findByRole("button", { name: "处理中…" });
    view.unmount();
    await act(async () => {
      resolve(Response.json(batch()));
    });
    await waitFor(() =>
      expect(
        view.requests.some(
          (item) => item.path.endsWith("/oauth-batches/batch-1") && item.method === "DELETE",
        ),
      ).toBe(true),
    );
  });

  it.each(["import", "convert"])(
    "成功批次创建 %s 任务期间禁止结束授权并在成功后显示任务",
    async (operation) => {
      const authorized: WorkbenchOAuthBatch = {
        ...batch(),
        status: "authorized",
        available: 1,
        items: [
          { ...row(), status: "succeeded" },
          { ...row(1), status: "failed" },
        ],
      };
      const importPreview: WorkbenchPreview = {
        id: "batch-import-preview",
        target: "https://sub2api.example.test",
        expires_at: new Date(Date.now() + 600000).toISOString(),
        check_after_import: false,
        model: "",
        errors: [],
        items: [
          {
            id: "0",
            index: 0,
            name: "user0",
            email: "user0@example.test",
            plan_type: "plus",
            template_id: "",
            template_name: "",
            template_revision: 0,
            group_ids: [],
            duplicate: false,
          },
        ],
      };
      const task: Task = {
        id: "batch-import-task",
        skill: "account-workbench",
        operation: "account-workbench-import",
        status: "queued",
        progress: 0,
        message: "等待导入账号",
        result: {},
        created_at: "2026-09-14T00:00:00Z",
        updated_at: "2026-09-14T00:00:00Z",
      };
      let resolve: (value: Response) => void = () => undefined;
      const response = new Promise<Response>((done) => {
        resolve = done;
      });
      const endpoint = operation === "import" ? "/import" : "/from-input";
      const view = mount(async (request) => {
        if (request.path.endsWith("/oauth-batches/batch-1/preview"))
          return Response.json(importPreview);
        if (request.path.endsWith("/oauth-batches/batch-1") && request.method === "GET")
          return Response.json(authorized);
        if (request.path.endsWith(endpoint)) return response;
        if (request.path.includes("/tasks/")) return Response.json(task);
      });
      const user = userEvent.setup();
      await start(user);
      await user.click(await screen.findByRole("button", { name: "预览授权账号" }));
      const previewButton = operation === "import" ? "确认导入 1 个账号" : "生成私有 JSON 文件";
      const confirmButton = operation === "import" ? "创建导入任务" : "创建私有转换任务";
      await user.click(await screen.findByRole("button", { name: previewButton }));
      await user.click(screen.getByRole("button", { name: confirmButton }));
      expect(screen.getByRole("button", { name: "结束批量授权", hidden: true })).toBeDisabled();
      await act(async () => resolve(Response.json(task)));
      await screen.findByRole("status", { name: "等待导入账号" });
      expect(view.requests.find((item) => item.path.endsWith(endpoint))?.body).toEqual({
        preview_id: "batch-import-preview",
        confirmed: true,
      });
    },
  );
});
