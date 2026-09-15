import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { WorkbenchOAuthSession } from "@/api";
import { WorkbenchOAuth } from "../components/workbench-oauth";
import { workbenchKeys } from "../constants";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
function session(status: WorkbenchOAuthSession["status"] = "waiting"): WorkbenchOAuthSession {
  return {
    id: "oauth-1",
    task_id: "authorization-task",
    host: "auth.openai.com",
    status,
    message: "等待完成授权",
    expires_at: new Date(Date.now() + 900000).toISOString(),
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
  const result = render(
    <QueryClientProvider client={client}>
      <WorkbenchOAuth />
    </QueryClientProvider>,
  );
  return { client, unmount: result.unmount };
}

describe("工作台授权会话", () => {
  it("启动请求未返回时保留结束入口，结束后撤销迟到的会话", async () => {
    let resolveStart: (value: Response) => void = () => undefined;
    const startResponse = new Promise<Response>((resolve) => {
      resolveStart = resolve;
    });
    const fetcher = vi.fn<typeof fetch>(async (_url, init) => {
      if (init?.method === "POST") return startResponse;
      return Response.json({ cancelled: true });
    });
    mount(fetcher);
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "开始授权登录" }));
    expect(screen.getByRole("status", { name: "正在启动授权浏览器" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "预览授权账号" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "结束授权" }));
    resolveStart(Response.json(session()));
    await waitFor(() =>
      expect(fetcher).toHaveBeenCalledWith(
        "/api/account-workbench/oauth/oauth-1",
        expect.objectContaining({ method: "DELETE", credentials: "include" }),
      ),
    );
    expect(screen.queryByRole("button", { name: "上游登录页面" })).not.toBeInTheDocument();
  });

  it("离开授权页面时撤销后端会话并清除画面查询缓存", async () => {
    const fetcher = vi.fn<typeof fetch>(async (_url, init) =>
      Response.json(init?.method === "DELETE" ? { cancelled: true } : session()),
    );
    const view = mount(fetcher);
    await userEvent.setup().click(screen.getByRole("button", { name: "开始授权登录" }));
    await screen.findByRole("button", { name: "上游登录页面" });
    await waitFor(() =>
      expect(view.client.getQueryData(workbenchKeys.oauth("oauth-1"))).toBeDefined(),
    );
    view.unmount();
    await waitFor(() =>
      expect(fetcher).toHaveBeenCalledWith(
        "/api/account-workbench/oauth/oauth-1",
        expect.objectContaining({ method: "DELETE" }),
      ),
    );
    expect(view.client.getQueryData(workbenchKeys.oauth("oauth-1"))).toBeUndefined();
  });

  it("等待人工授权时不展示导入入口，确认完成后才请求授权复核", async () => {
    let finished = false;
    const fetcher = vi.fn<typeof fetch>(async (url) => {
      if (String(url).endsWith("/finish")) {
        finished = true;
        return Response.json({ accepted: true });
      }
      if (String(url).endsWith("/templates")) return Response.json([]);
      return Response.json(session(finished ? "authorized" : "waiting"));
    });
    mount(fetcher);
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "开始授权登录" }));
    await screen.findByRole("button", { name: "上游登录页面" });
    expect(screen.queryByRole("button", { name: "预览授权账号" })).not.toBeInTheDocument();
    expect(fetcher.mock.calls.some(([url]) => String(url).endsWith("/finish"))).toBe(false);
    await user.click(screen.getByRole("button", { name: "登录完成，验证授权" }));
    expect(await screen.findByRole("button", { name: "预览授权账号" })).toBeEnabled();
    expect(fetcher).toHaveBeenCalledWith(
      "/api/account-workbench/oauth/oauth-1/finish",
      expect.objectContaining({ method: "POST", body: "{}" }),
    );
    expect(screen.queryByRole("button", { name: "上游登录页面" })).not.toBeInTheDocument();
  });

  it("会话读取失败时停用画面和复核并提供重试与结束入口", async () => {
    const fetcher = vi.fn<typeof fetch>(async (_url, init) => {
      if (init?.method === "POST") return Response.json(session());
      if (init?.method === "DELETE") return Response.json({ cancelled: true });
      return Response.json(
        { detail: "授权页面读取失败", code: "oauth_read_failed" },
        { status: 503 },
      );
    });
    mount(fetcher);
    await userEvent.setup().click(screen.getByRole("button", { name: "开始授权登录" }));
    await screen.findByRole("button", { name: "重新读取" });
    expect(screen.getByRole("button", { name: "上游登录页面" })).toHaveAttribute(
      "aria-disabled",
      "true",
    );
    expect(screen.getByRole("button", { name: "登录完成，验证授权" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "结束授权" })).toBeEnabled();
  });

  it("服务端返回过期会话时禁止导入并允许重新授权", async () => {
    mount(vi.fn<typeof fetch>(async () => Response.json(session("expired"))));
    await userEvent.setup().click(screen.getByRole("button", { name: "开始授权登录" }));
    expect(await screen.findByRole("status")).toHaveTextContent("授权会话已过期");
    expect(screen.queryByRole("button", { name: "预览授权账号" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "重新授权登录" })).toBeEnabled();
  });

  it("授权回调未就绪时提前复核会显示等待原因并恢复可交互状态", async () => {
    let finished = false;
    mount(
      vi.fn<typeof fetch>(async (url) => {
        if (String(url).endsWith("/finish")) {
          finished = true;
          return Response.json({ accepted: true });
        }
        const value = session();
        if (finished) value.message = "尚未收到授权回调，请完成登录后重试";
        return Response.json(value);
      }),
    );
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "开始授权登录" }));
    await screen.findByRole("button", { name: "上游登录页面" });
    await user.click(screen.getByRole("button", { name: "登录完成，验证授权" }));
    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveTextContent("尚未收到授权回调，请完成登录后重试"),
    );
    expect(screen.getByRole("button", { name: "登录完成，验证授权" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "上游登录页面" })).toHaveAttribute(
      "aria-disabled",
      "false",
    );
    expect(screen.queryByRole("button", { name: "预览授权账号" })).not.toBeInTheDocument();
  });
});
