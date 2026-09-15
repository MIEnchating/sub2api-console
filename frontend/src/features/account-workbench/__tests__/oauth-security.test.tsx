import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { WorkbenchSecuritySession, WorkbenchSecuritySource } from "@/api";
import { WorkbenchOAuthSecurity } from "../components/workbench-oauth-security";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function source(): WorkbenchSecuritySource {
  return {
    source_oauth_id: "local-oauth",
    scope: "local-export",
    email: "owner@example.test",
    user_id: "official-user",
    workspace_id: "official-workspace",
    expires_at: new Date(Date.now() + 600000).toISOString(),
  };
}

function mount(
  options: {
    read?: () => Promise<Response>;
    start?: () => Promise<Response>;
    succeeded?: boolean;
  } = {},
) {
  const requests: Array<{ path: string; method: string; body: unknown }> = [];
  const result: WorkbenchSecuritySession = {
    id: "security-local",
    task_id: "security-local",
    source_oauth_id: "local-oauth",
    scope: "local-export",
    operation: "totp",
    email: "owner@example.test",
    status: options.succeeded ? "succeeded" : "waiting",
    message: "等待官方登录",
    expires_at: source().expires_at,
    width: 1100,
    height: 760,
    artifact_id: options.succeeded ? "private-security" : undefined,
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
      if (path.endsWith("/security-source")) return options.read?.() ?? Response.json(source());
      if (method === "DELETE") return Response.json({ cancelled: true });
      if (path === "/api/account-workbench/security" && method === "POST")
        return options.start?.() ?? Response.json(result);
      if (path === "/api/account-workbench/security/security-local") return Response.json(result);
      throw new Error(`未预期请求 ${method} ${path}`);
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  const close = vi.fn();
  const view = render(
    <QueryClientProvider client={client}>
      <WorkbenchOAuthSecurity sourceId="local-oauth" onClose={close} />
    </QueryClientProvider>,
  );
  return { ...view, requests, client, close, result };
}

describe("授权结果安全设置", () => {
  it("本地授权展示后端身份并二次确认后只提交不透明来源", async () => {
    const view = mount({ succeeded: true });
    const user = userEvent.setup();
    expect(await screen.findByText("owner@example.test")).toBeInTheDocument();
    expect(screen.getByText("official-workspace")).toBeInTheDocument();
    expect(screen.queryByRole("combobox", { name: "安全设置账号" })).not.toBeInTheDocument();
    expect(screen.queryByLabelText("登录代理")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "查看操作范围" }));
    const confirmation = screen.getByRole("region", { name: "确认安全操作" });
    expect(within(confirmation).getByText(/official-user/)).toBeInTheDocument();
    expect(view.requests.some((request) => request.method === "POST")).toBe(false);
    await user.click(screen.getByRole("button", { name: "确认并开始安全设置" }));
    await screen.findByText("私有结果 ID：private-security");
    expect(view.requests.find((request) => request.method === "POST")?.body).toEqual({
      source_oauth_id: "local-oauth",
      scope: "local-export",
      operation: "totp",
      confirmed: true,
    });
    expect(view.requests.some((request) => request.path === "/api/accounts")).toBe(false);
    expect(screen.queryByText(/保存为登录资料/)).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "返回授权结果" }));
    expect(view.close).toHaveBeenCalledOnce();
    view.unmount();
    await waitFor(() =>
      expect(view.requests.some((request) => request.method === "DELETE")).toBe(true),
    );
    expect(
      view.requests.some(
        (request) => request.method === "DELETE" && request.path.includes("/oauth/"),
      ),
    ).toBe(false);
    expect(
      view.client.getQueryData(["account-workbench", "security-source", "local-oauth"]),
    ).toBeUndefined();
  });

  it("读取身份期间保留关闭入口且不能提交，失败后可以重读", async () => {
    let resolve: (response: Response) => void = () => undefined;
    let failed = false;
    const view = mount({
      read: () => {
        if (failed) return Promise.resolve(Response.json(source()));
        return new Promise<Response>((done) => {
          resolve = done;
        });
      },
    });
    expect(screen.getByRole("status", { name: "正在核对授权账号身份" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "返回授权结果" })).toBeEnabled();
    expect(screen.queryByRole("button", { name: "查看操作范围" })).not.toBeInTheDocument();
    await act(async () => {
      failed = true;
      resolve(Response.json({ detail: "授权身份暂不可读取" }, { status: 503 }));
    });
    await userEvent.setup().click(await screen.findByRole("button", { name: "重新读取" }));
    expect(await screen.findByRole("button", { name: "查看操作范围" })).toBeEnabled();
    expect(view.requests.some((request) => request.method === "POST")).toBe(false);
  });

  it("来源已过期时展示状态且禁止创建安全任务", async () => {
    mount({ read: async () => Response.json({ ...source(), expires_at: "2000-01-01T00:00:00Z" }) });
    await screen.findByText("授权已到期，请重新授权");
    expect(screen.getByRole("button", { name: "查看操作范围" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "返回授权结果" })).toBeEnabled();
  });

  it("启动后离开会撤销迟到安全会话且保留原授权", async () => {
    let resolve: (response: Response) => void = () => undefined;
    const view = mount({
      start: () =>
        new Promise<Response>((done) => {
          resolve = done;
        }),
    });
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "查看操作范围" }));
    await user.click(screen.getByRole("button", { name: "确认并开始安全设置" }));
    await screen.findByRole("status", { name: "正在启动账号安全任务" });
    view.unmount();
    await act(async () => resolve(Response.json(view.result)));
    await waitFor(() =>
      expect(
        view.requests.some(
          (request) => request.path.endsWith("/security-local") && request.method === "DELETE",
        ),
      ).toBe(true),
    );
    expect(
      view.requests.some(
        (request) => request.path.includes("/oauth/") && request.method === "DELETE",
      ),
    ).toBe(false);
  });
});
