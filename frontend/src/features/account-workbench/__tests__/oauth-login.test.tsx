import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { WorkbenchOAuthSession } from "@/api";
import { WorkbenchOAuth } from "../components/workbench-oauth";

const clients: QueryClient[] = [];
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
function mount(
  startResponse?: () => Promise<Response>,
): Array<{ path: string; method: string; body: unknown }> {
  const requests: Array<{ path: string; method: string; body: unknown }> = [];
  const session: WorkbenchOAuthSession = {
    id: "automatic-session",
    task_id: "automatic-task",
    host: "auth.openai.com",
    status: "waiting",
    message: "等待完成授权",
    expires_at: new Date(Date.now() + 900000).toISOString(),
    width: 1100,
    height: 760,
  };
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
      if (method === "DELETE") return Response.json({ cancelled: true });
      if (path.endsWith("/oauth") && method === "POST" && startResponse) return startResponse();
      return Response.json(session);
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <WorkbenchOAuth />
    </QueryClientProvider>,
  );
  return requests;
}
async function openLogin(user: ReturnType<typeof userEvent.setup>): Promise<HTMLElement> {
  await user.click(screen.getByRole("checkbox", { name: "自动填写登录信息" }));
  await user.click(screen.getByRole("button", { name: "开始授权登录" }));
  return screen.getByRole("dialog", { name: "自动填写登录信息" });
}

describe("自动填写授权登录信息", () => {
  it("人工授权只提交代理并在发出请求后清空，不保留mutation凭据变量", async () => {
    const requests = mount();
    const user = userEvent.setup();
    await user.type(
      screen.getByLabelText("登录代理"),
      "https://user:proxy-password@proxy.example.test:443",
    );
    await user.click(screen.getByRole("button", { name: "开始授权登录" }));
    await waitFor(() =>
      expect(requests.find((item) => item.path.endsWith("/oauth"))?.body).toEqual({
        proxy_url: "https://user:proxy-password@proxy.example.test:443",
      }),
    );
    expect(
      JSON.stringify(
        clients.flatMap((client) =>
          client
            .getMutationCache()
            .getAll()
            .map((mutation) => mutation.state.variables),
        ),
      ),
    ).not.toContain("proxy-password");
    await user.click(screen.getByRole("button", { name: "结束授权" }));
    expect(await screen.findByLabelText("登录代理")).toHaveValue("");
  });

  it("人工授权代理无效时阻止启动且显示字段错误", async () => {
    const requests = mount();
    const user = userEvent.setup();
    await user.type(screen.getByLabelText("登录代理"), "ftp://proxy.example.test");
    await user.click(screen.getByRole("button", { name: "开始授权登录" }));
    expect(screen.getByLabelText("登录代理")).toHaveAttribute("aria-invalid", "true");
    expect(requests.some((item) => item.method === "POST")).toBe(false);
  });

  it("默认人工授权不发送登录凭据", async () => {
    const requests = mount();
    await userEvent.setup().click(screen.getByRole("button", { name: "开始授权登录" }));
    await waitFor(() =>
      expect(requests.find((item) => item.path.endsWith("/oauth"))?.body).toEqual({}),
    );
  });

  it("提交自动授权后立即移除表单且结束等待后重新打开所有凭据为空", async () => {
    let resolveStart: (response: Response) => void = () => undefined;
    const pending = new Promise<Response>((resolve) => {
      resolveStart = resolve;
    });
    const requests = mount(() => pending);
    const user = userEvent.setup();
    const dialog = await openLogin(user);
    await user.type(
      within(dialog).getByRole("textbox", { name: "登录邮箱" }),
      "operator@example.test",
    );
    await user.type(within(dialog).getByLabelText("登录密码"), "fixture-password");
    await user.type(within(dialog).getByLabelText("TOTP 密钥"), "JBSWY3DPEHPK3PXP");
    await user.type(within(dialog).getByRole("textbox", { name: "工作区 ID" }), "workspace-42");
    await user.click(within(dialog).getByRole("button", { name: "开始授权登录" }));
    await waitFor(() =>
      expect(requests.find((item) => item.path.endsWith("/oauth"))?.body).toEqual({
        login: {
          email: "operator@example.test",
          password: "fixture-password",
          totp_secret: "JBSWY3DPEHPK3PXP",
          workspace_id: "workspace-42",
        },
      }),
    );
    expect(screen.queryByRole("dialog", { name: "自动填写登录信息" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "结束授权" }));
    const reopened = await openLogin(user);
    expect(within(reopened).getByLabelText("登录密码")).toHaveValue("");
    expect(within(reopened).getByLabelText("TOTP 密钥")).toHaveValue("");
    expect(within(reopened).getByRole("textbox", { name: "登录邮箱" })).toHaveValue("");
    resolveStart(Response.json({ id: "late-session" }));
    await waitFor(() =>
      expect(requests).toContainEqual({
        path: "/api/account-workbench/oauth/late-session",
        method: "DELETE",
        body: null,
      }),
    );
  });

  it("取消登录信息弹窗后重新打开不保留密码", async () => {
    mount();
    const user = userEvent.setup();
    const dialog = await openLogin(user);
    await user.type(within(dialog).getByLabelText("登录密码"), "fixture-password");
    await user.click(within(dialog).getByRole("button", { name: "取消" }));
    await user.click(screen.getByRole("button", { name: "开始授权登录" }));
    expect(
      within(screen.getByRole("dialog", { name: "自动填写登录信息" })).getByLabelText("登录密码"),
    ).toHaveValue("");
  });

  it("无效邮箱或 TOTP 密钥阻止启动并标记对应字段", async () => {
    const requests = mount();
    const user = userEvent.setup();
    const dialog = await openLogin(user);
    await user.type(within(dialog).getByRole("textbox", { name: "登录邮箱" }), "invalid-email");
    await user.type(within(dialog).getByLabelText("TOTP 密钥"), "invalid-01");
    await user.click(within(dialog).getByRole("button", { name: "开始授权登录" }));
    expect(within(dialog).getByRole("textbox", { name: "登录邮箱" })).toHaveAttribute(
      "aria-invalid",
      "true",
    );
    expect(within(dialog).getByLabelText("TOTP 密钥")).toHaveAttribute("aria-invalid", "true");
    expect(requests.some((item) => item.method === "POST")).toBe(false);
  });

  it("Microsoft 邮箱缺少令牌时保留配置弹窗并阻止启动", async () => {
    const requests = mount();
    const user = userEvent.setup();
    const dialog = await openLogin(user);
    await user.type(
      within(dialog).getByRole("textbox", { name: "登录邮箱" }),
      "operator@example.test",
    );
    await user.click(within(dialog).getByRole("combobox", { name: "邮箱验证码" }));
    await user.click(await screen.findByRole("option", { name: "Microsoft 邮箱" }));
    await user.type(
      within(dialog).getByRole("textbox", { name: "Microsoft 客户端 ID" }),
      "fixture-client",
    );
    await user.click(within(dialog).getByRole("button", { name: "开始授权登录" }));
    expect(within(dialog).getByLabelText("Microsoft Refresh Token")).toHaveAttribute(
      "aria-invalid",
      "true",
    );
    expect(requests.some((item) => item.method === "POST")).toBe(false);
  });
});
