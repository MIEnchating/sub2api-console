import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type {
  WorkbenchOAuthBatch,
  WorkbenchOAuthBatchPreview,
  WorkbenchOAuthBatchRow,
} from "@/api";
import { WorkbenchOAuthBatch as Batch } from "../components/workbench-oauth-batch";

let client: QueryClient;
type Request = { path: string; method: string; body: unknown };
const row: WorkbenchOAuthBatchRow = {
  index: 0,
  account_id: "42",
  profile_id: "profile-a",
  profile_revision: 4,
  user_id: "user-a",
  email: "operator@example.test",
  workspace_id: "workspace-a",
  has_password: true,
  has_totp: false,
  status: "queued",
  message: "等待授权",
};
function preview(id = "preview-a"): WorkbenchOAuthBatchPreview {
  return {
    id,
    target: "https://sub2api.example.test",
    fresh_login: true,
    expires_at: new Date(Date.now() + 600000).toISOString(),
    items: [row],
    errors: [],
  };
}
function batch(id = "batch-a"): WorkbenchOAuthBatch {
  return {
    id,
    task_id: "task-a",
    fresh_login: true,
    status: "failed",
    available: 0,
    message: "授权失败",
    expires_at: new Date(Date.now() + 600000).toISOString(),
    items: [{ ...row, status: "failed" }],
  };
}
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  client?.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
function mount(
  handler?: (request: Request) => Promise<Response | undefined>,
  sourceTaskId?: string,
): { requests: Request[]; unmount: () => void } {
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
      if (request.path.endsWith("/accounts")) return Response.json([]);
      if (request.path.endsWith("/login-profiles"))
        return Response.json([
          {
            id: "profile-a",
            account_id: "42",
            user_id: "user-a",
            workspace_id: "workspace-a",
            email: row.email,
            revision: 4,
            updated_at: "2026-09-14T10:00:00Z",
            has_password: true,
            has_totp: false,
          },
        ]);
      if (request.path.endsWith("/reauthorization/preview")) return Response.json(preview());
      if (request.path.endsWith("/templates")) return Response.json([]);
      return Response.json(batch());
    }),
  );
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const view = render(
    <QueryClientProvider client={client}>
      <Batch reauthorization={!sourceTaskId} sourceTaskId={sourceTaskId} />
    </QueryClientProvider>,
  );
  return { requests, unmount: view.unmount };
}
async function parse(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await user.click(
    await screen.findByRole("checkbox", { name: "选择 operator@example.test（ID 42）" }),
  );
  await user.click(screen.getByRole("button", { name: "预览重新授权（1）" }));
  await screen.findByRole("region", { name: "批量授权预览" });
}
async function start(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await user.click(screen.getByRole("button", { name: "确认授权 1 个账号" }));
  await user.click(screen.getByRole("button", { name: "开始批量授权" }));
  await screen.findByRole("region", { name: "批量授权进度" });
}

describe("登录资料重新授权", () => {
  it("只用账号ID预览新登录且展示版本，确认后仅提交预览ID启动", async () => {
    const view = mount();
    const user = userEvent.setup();
    await parse(user);
    expect(
      view.requests.find((request) => request.path.endsWith("/reauthorization/preview"))?.body,
    ).toEqual({ account_ids: ["42"], fresh_login: true });
    expect(screen.getByRole("region", { name: "批量授权预览" })).toHaveTextContent("资料版本：4");
    expect(
      view.requests.some(
        (request) => request.path.endsWith("/oauth-batches") && request.method === "POST",
      ),
    ).toBe(false);
    await start(user);
    expect(
      view.requests.find(
        (request) => request.path.endsWith("/oauth-batches") && request.method === "POST",
      )?.body,
    ).toEqual({ preview_id: "preview-a", confirmed: true });
  });

  it("失败批次重新预览只发送原批ID并在确认新批后清理原批", async () => {
    let starts = 0;
    const view = mount(async (request) => {
      if (request.path.endsWith("/oauth-batches") && request.method === "POST")
        return Response.json(batch(++starts === 1 ? "batch-a" : "batch-b"));
      if (request.path.endsWith("/oauth-batches/batch-b")) return Response.json(batch("batch-b"));
    });
    const user = userEvent.setup();
    await parse(user);
    await start(user);
    await user.click(screen.getByRole("button", { name: "重新授权失败项" }));
    await screen.findByRole("region", { name: "批量授权预览" });
    expect(
      view.requests.filter((request) => request.path.endsWith("/reauthorization/preview")).at(-1)
        ?.body,
    ).toEqual({ account_ids: [], failed_batch_id: "batch-a", fresh_login: true });
    expect(
      view.requests.some(
        (request) => request.method === "DELETE" && request.path.endsWith("/batch-a"),
      ),
    ).toBe(false);
    await start(user);
    await waitFor(() =>
      expect(
        view.requests.some(
          (request) => request.method === "DELETE" && request.path.endsWith("/batch-a"),
        ),
      ).toBe(true),
    );
  });

  it("失败项重试有成功结果未导入时明确确认清除数量且取消保留原批", async () => {
    const view = mount(async (request) => {
      if (
        request.path.endsWith("/oauth-batches") ||
        request.path.endsWith("/oauth-batches/batch-a")
      )
        return Response.json({
          ...batch(),
          status: "authorized",
          available: 1,
          items: [
            { ...row, index: 0, status: "succeeded" },
            { ...row, index: 1, status: "failed", account_id: "43" },
          ],
        });
    });
    const user = userEvent.setup();
    await parse(user);
    await start(user);
    await user.click(screen.getByRole("button", { name: "重新授权失败项" }));
    await user.click(await screen.findByRole("button", { name: "确认授权 1 个账号" }));
    const dialog = screen.getByRole("dialog", { name: "确认批量授权" });
    expect(dialog).toHaveTextContent("1 个成功结果未导入");
    await user.click(within(dialog).getByRole("button", { name: "取消" }));
    expect(
      view.requests.filter(
        (request) => request.path.endsWith("/oauth-batches") && request.method === "POST",
      ),
    ).toHaveLength(1);
    expect(
      view.requests.some(
        (request) => request.method === "DELETE" && request.path.endsWith("/batch-a"),
      ),
    ).toBe(false);
  });

  it("历史重新授权需要点击并只提交历史任务ID，不复用旧会话", async () => {
    const view = mount(undefined, "history-task");
    const user = userEvent.setup();
    expect(view.requests).toHaveLength(0);
    await user.click(screen.getByRole("button", { name: "预览历史账号重新授权" }));
    await screen.findByRole("region", { name: "批量授权预览" });
    expect(view.requests[0]?.body).toEqual({
      account_ids: [],
      source_task_id: "history-task",
      fresh_login: true,
    });
  });

  it("预览请求失败后恢复资料选择且不能启动不存在的预览", async () => {
    mount(async (request) =>
      request.path.endsWith("/reauthorization/preview")
        ? Response.json({ detail: "登录资料版本变化" }, { status: 409 })
        : undefined,
    );
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("checkbox", { name: "选择 operator@example.test（ID 42）" }),
    );
    await user.click(screen.getByRole("button", { name: "预览重新授权（1）" }));
    expect(await screen.findByRole("region", { name: "登录资料" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "确认授权 1 个账号" })).not.toBeInTheDocument();
  });

  it("取消尚未完成的重新授权预览会清理迟到预览", async () => {
    let resolve: (response: Response) => void = () => undefined;
    const response = new Promise<Response>((done) => {
      resolve = done;
    });
    const view = mount(async (request) =>
      request.path.endsWith("/reauthorization/preview") ? response : undefined,
    );
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("checkbox", { name: "选择 operator@example.test（ID 42）" }),
    );
    await user.click(screen.getByRole("button", { name: "预览重新授权（1）" }));
    await user.click(await screen.findByRole("button", { name: "取消解析" }));
    await act(async () => {
      resolve(Response.json(preview()));
    });
    await waitFor(() =>
      expect(
        view.requests.some(
          (request) => request.method === "DELETE" && request.path.endsWith("/preview/preview-a"),
        ),
      ).toBe(true),
    );
    expect(screen.queryByRole("region", { name: "批量授权预览" })).not.toBeInTheDocument();
  });
});
