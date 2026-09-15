import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { WorkbenchOAuthCheckpoint, WorkbenchOAuthSession } from "@/api";
import { WorkbenchOAuth } from "../components/workbench-oauth";
import { workbenchKeys } from "../constants";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
function checkpoint(): WorkbenchOAuthCheckpoint {
  return {
    id: "checkpoint-1",
    source_task_id: "original-task",
    status: "ready",
    revision: 2,
    created_at: "2026-09-14T00:00:00Z",
    expires_at: new Date(Date.now() + 600000).toISOString(),
    can_restore: true,
  };
}
function session(): WorkbenchOAuthSession {
  return {
    id: "oauth-1",
    task_id: "oauth-task",
    host: "auth.openai.com",
    status: "waiting",
    message: "等待人工继续授权",
    expires_at: new Date(Date.now() + 600000).toISOString(),
    width: 1100,
    height: 760,
    image: "data:image/jpeg;base64,ZnJhbWU=",
  };
}
function mount(fetcher: typeof fetch): { client: QueryClient; unmount: () => void } {
  vi.stubGlobal("fetch", fetcher);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  const view = render(
    <QueryClientProvider client={client}>
      <WorkbenchOAuth />
    </QueryClientProvider>,
  );
  return { client, unmount: view.unmount };
}

it("暂停授权必须确认，成功后清除画面且离开页面不取消已保存会话", async () => {
  const fetcher = vi.fn<typeof fetch>(async (url) =>
    Response.json(String(url).endsWith("/checkpoint") ? checkpoint() : session()),
  );
  const view = mount(fetcher);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "开始授权登录" }));
  await user.click(await screen.findByRole("button", { name: "暂停并保存授权" }));
  expect(fetcher.mock.calls.some(([url]) => String(url).endsWith("/checkpoint"))).toBe(false);
  await user.click(screen.getByRole("button", { name: "确认暂停授权" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(screen.queryByRole("button", { name: "上游登录页面" })).not.toBeInTheDocument();
  expect(view.client.getQueryData(workbenchKeys.oauth("oauth-1"))).toBeUndefined();
  view.unmount();
  expect(fetcher.mock.calls.some(([, options]) => options?.method === "DELETE")).toBe(false);
});

it("恢复授权使用检查点版本并进入人工页面，未经确认不发送恢复请求", async () => {
  const fetcher = vi.fn<typeof fetch>(async (url) =>
    Response.json(String(url).endsWith("/oauth-checkpoints") ? [checkpoint()] : session()),
  );
  mount(fetcher);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "已暂停的授权" }));
  await user.click(await screen.findByRole("button", { name: "恢复授权" }));
  expect(fetcher.mock.calls.some(([url]) => String(url).endsWith("/restore"))).toBe(false);
  await user.click(screen.getByRole("button", { name: "确认人工恢复" }));
  expect(await screen.findByRole("button", { name: "上游登录页面" })).toHaveAttribute(
    "aria-disabled",
    "false",
  );
  expect(fetcher).toHaveBeenCalledWith(
    "/api/account-workbench/oauth-checkpoints/checkpoint-1/restore",
    expect.objectContaining({
      method: "POST",
      body: JSON.stringify({ revision: 2, confirmed: true }),
    }),
  );
  expect(
    fetcher.mock.calls.some(
      ([url]) => String(url).endsWith("/input") || String(url).endsWith("/finish"),
    ),
  ).toBe(false);
});

it("恢复请求等待时结束操作会撤销迟到的新会话", async () => {
  let resolve: (response: Response) => void = () => undefined;
  const response = new Promise<Response>((done) => {
    resolve = done;
  });
  const fetcher = vi.fn<typeof fetch>(async (url) =>
    String(url).endsWith("/restore") ? response : Response.json([checkpoint()]),
  );
  mount(fetcher);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "已暂停的授权" }));
  await user.click(await screen.findByRole("button", { name: "恢复授权" }));
  await user.click(screen.getByRole("button", { name: "确认人工恢复" }));
  await user.click(screen.getByRole("button", { name: "结束授权" }));
  resolve(Response.json(session()));
  await waitFor(() =>
    expect(fetcher).toHaveBeenCalledWith(
      "/api/account-workbench/oauth/oauth-1",
      expect.objectContaining({ method: "DELETE" }),
    ),
  );
  expect(screen.queryByRole("button", { name: "上游登录页面" })).not.toBeInTheDocument();
});

it("过期及已消费检查点禁止恢复，删除按当前版本确认", async () => {
  const expired = { ...checkpoint(), expires_at: "2020-01-01T00:00:00Z" };
  const used = { ...checkpoint(), id: "used", status: "restored", can_restore: false };
  const fetcher = vi.fn<typeof fetch>(async () => Response.json([expired, used]));
  mount(fetcher);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "已暂停的授权" }));
  const buttons = await screen.findAllByRole("button", { name: "恢复授权" });
  buttons.forEach((button) => expect(button).toBeDisabled());
  await user.click(screen.getAllByRole("button", { name: "删除检查点" })[0]);
  const dialog = screen.getByRole("dialog", { name: "删除授权检查点" });
  expect(fetcher.mock.calls.some(([, options]) => options?.method === "DELETE")).toBe(false);
  await user.click(within(dialog).getByRole("button", { name: "确认删除检查点" }));
  await waitFor(() =>
    expect(fetcher).toHaveBeenCalledWith(
      "/api/account-workbench/oauth-checkpoints/checkpoint-1",
      expect.objectContaining({
        method: "DELETE",
        body: JSON.stringify({ revision: 2, confirmed: true }),
      }),
    ),
  );
});

it("检查点读取失败保留返回与重试入口，重试后显示空状态", async () => {
  let failed = true;
  mount(
    vi.fn<typeof fetch>(async () =>
      failed ? Response.json({ detail: "检查点读取暂不可用" }, { status: 503 }) : Response.json([]),
    ),
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "已暂停的授权" }));
  await screen.findByRole("button", { name: "重新读取" });
  expect(screen.getByRole("button", { name: "返回" })).toBeEnabled();
  failed = false;
  await user.click(screen.getByRole("button", { name: "重新读取" }));
  expect(await screen.findByText("暂无已暂停的授权")).toBeInTheDocument();
});
