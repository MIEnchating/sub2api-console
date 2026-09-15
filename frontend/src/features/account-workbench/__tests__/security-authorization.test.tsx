import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { WorkbenchOAuthSession } from "@/api";
import { WorkbenchSecurityAuthorization } from "../components/workbench-security-authorization";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  vi.useRealTimers();
});

function mount(options: { expired?: boolean; authorize?: () => Promise<Response> } = {}) {
  const requests: Array<{ path: string; method: string; body: unknown }> = [];
  const oauth: WorkbenchOAuthSession = {
    id: "new-oauth",
    task_id: "new-oauth",
    scope: "local-export",
    host: "auth.openai.com",
    status: "waiting",
    message: "等待官方登录",
    expires_at: new Date(Date.now() + 600000).toISOString(),
    width: 1100,
    height: 760,
  };
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input, init) => {
      const path = String(input),
        method = init?.method ?? "GET";
      requests.push({
        path,
        method,
        body: init?.body ? (JSON.parse(String(init.body)) as unknown) : null,
      });
      if (method === "DELETE") return Response.json({ cancelled: true });
      if (path === "/api/account-workbench/security/finished-security/oauth")
        return options.authorize?.() ?? Response.json(oauth);
      throw new Error(`未预期请求 ${method} ${path}`);
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  const onOAuth = vi.fn();
  const view = render(
    <QueryClientProvider client={client}>
      <WorkbenchSecurityAuthorization
        securityId="finished-security"
        expiresAt={new Date(Date.now() + (options.expired ? -1 : 600000)).toISOString()}
        onOAuth={onOAuth}
      />
    </QueryClientProvider>,
  );
  return { ...view, onOAuth, requests, oauth };
}

describe("安全结果重新授权", () => {
  it("点击明确确认后以安全任务ID创建新授权并交给授权页面", async () => {
    const view = mount();
    expect(view.requests).toEqual([]);
    await userEvent.setup().click(screen.getByRole("button", { name: "确认发起新的 OAuth 授权" }));
    await waitFor(() => expect(view.onOAuth).toHaveBeenCalledWith(view.oauth));
    expect(view.requests).toEqual([
      {
        path: "/api/account-workbench/security/finished-security/oauth",
        method: "POST",
        body: { confirmed: true },
      },
    ]);
  });

  it("来源已经到期时禁用重新授权且不发送请求", async () => {
    const view = mount({ expired: true });
    const button = screen.getByRole("button", { name: "确认发起新的 OAuth 授权" });
    expect(button).toBeDisabled();
    await userEvent.setup().click(button);
    expect(view.requests).toEqual([]);
  });

  it("来源在显示期间到期后禁用操作", () => {
    vi.useFakeTimers();
    mount();
    act(() => vi.advanceTimersByTime(600001));
    expect(screen.getByRole("button", { name: "确认发起新的 OAuth 授权" })).toBeDisabled();
  });

  it("新授权被拒绝后恢复按钮且不交给授权页面", async () => {
    const view = mount({
      authorize: async () => Response.json({ detail: "安全来源已变化" }, { status: 409 }),
    });
    await userEvent.setup().click(screen.getByRole("button", { name: "确认发起新的 OAuth 授权" }));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "确认发起新的 OAuth 授权" })).toBeEnabled(),
    );
    expect(view.onOAuth).not.toHaveBeenCalled();
  });

  it("新授权等待期间关闭时取消迟到响应且不交给已关闭页面", async () => {
    let resolve: (response: Response) => void = () => undefined;
    const view = mount({
      authorize: () =>
        new Promise<Response>((done) => {
          resolve = done;
        }),
    });
    await userEvent.setup().click(screen.getByRole("button", { name: "确认发起新的 OAuth 授权" }));
    expect(await screen.findByRole("button", { name: "正在发起新授权" })).toBeDisabled();
    view.unmount();
    await act(async () => resolve(Response.json(view.oauth)));
    await waitFor(() =>
      expect(
        view.requests.some(
          (item) => item.method === "DELETE" && item.path.endsWith("/oauth/new-oauth"),
        ),
      ).toBe(true),
    );
    expect(view.onOAuth).not.toHaveBeenCalled();
  });
});
