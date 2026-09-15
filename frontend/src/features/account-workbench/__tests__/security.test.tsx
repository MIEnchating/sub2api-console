import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { WorkbenchSecuritySession } from "@/api";
import { WorkbenchSecurity } from "../components/workbench-security";

const clients: QueryClient[] = [];
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function session(status: WorkbenchSecuritySession["status"] = "waiting"): WorkbenchSecuritySession {
  return {
    id: "security-1",
    task_id: "security-1",
    account_id: "42",
    email: "owner@example.com",
    operation: "totp",
    status,
    message: "请在官方页面完成登录",
    expires_at: new Date(Date.now() + 900000).toISOString(),
    width: 1100,
    height: 760,
    image: "data:image/jpeg;base64,ZnJhbWU=",
  };
}
function mount(
  options: { start?: () => Promise<Response>; empty?: boolean; get?: () => Response } = {},
) {
  const requests: Array<{ path: string; method: string; body: unknown }> = [];
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
      if (path === "/api/accounts")
        return Response.json(
          options.empty
            ? []
            : [
                { id: "42", name: "团队账号", platform: "openai", account_type: "oauth" },
                { id: "43", name: "密钥账号", platform: "openai", account_type: "apikey" },
              ],
        );
      if (method === "DELETE") return Response.json({ cancelled: true });
      if (method === "POST" && path.endsWith("/security"))
        return options.start?.() ?? Response.json(session());
      if (method === "POST") return Response.json({ accepted: true });
      return options.get?.() ?? Response.json(session());
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  const result = render(
    <QueryClientProvider client={client}>
      <WorkbenchSecurity />
    </QueryClientProvider>,
  );
  return { requests, client, unmount: result.unmount };
}
async function choose(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  const account = await screen.findByRole("combobox", { name: "安全设置账号" });
  account.focus();
  await user.keyboard("{ArrowDown}");
  await user.click(await screen.findByRole("option", { name: "团队账号（ID 42）" }));
}
async function confirm(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await choose(user);
  await user.click(screen.getByRole("button", { name: "查看操作范围" }));
  await user.click(screen.getByRole("button", { name: "确认并开始安全设置" }));
}

describe("账号安全设置", () => {
  it("选择稳定账号并展示影响范围后才允许创建任务", async () => {
    const view = mount();
    const user = userEvent.setup();
    await choose(user);
    await user.click(screen.getByRole("button", { name: "查看操作范围" }));
    expect(
      within(screen.getByRole("region", { name: "确认安全操作" })).getByText(/团队账号（ID 42）/),
    ).toBeInTheDocument();
    expect(view.requests.some((v) => v.method === "POST")).toBe(false);
    await user.click(screen.getByRole("button", { name: "确认并开始安全设置" }));
    await screen.findByRole("button", { name: "上游登录页面" });
    expect(view.requests.find((v) => v.method === "POST")?.body).toEqual({
      account_id: "42",
      operation: "totp",
      confirmed: true,
    });
  });
  it("确认密码后清空表单并使密码不进入 React Query 变更缓存", async () => {
    const view = mount();
    const user = userEvent.setup();
    await choose(user);
    await user.click(screen.getByRole("combobox", { name: "安全操作" }));
    await user.click(await screen.findByRole("option", { name: "设置密码" }));
    await user.type(screen.getByLabelText("新密码"), "StrongPassword-2026!");
    await user.click(screen.getByRole("button", { name: "查看操作范围" }));
    await user.click(screen.getByRole("button", { name: "确认并开始安全设置" }));
    await screen.findByRole("button", { name: "上游登录页面" });
    expect(screen.queryByLabelText("新密码")).not.toBeInTheDocument();
    expect(
      JSON.stringify(
        view.client
          .getMutationCache()
          .getAll()
          .map((v) => v.state.variables),
      ),
    ).not.toContain("StrongPassword-2026!");
    await user.click(screen.getByRole("button", { name: "关闭安全任务" }));
    await user.click(screen.getByRole("combobox", { name: "安全操作" }));
    await user.click(screen.getByRole("option", { name: "设置密码" }));
    expect(screen.getByLabelText("新密码")).toHaveValue("");
  });
  it("启动期间关闭会话会撤销迟到的后端结果", async () => {
    let resolve: (value: Response) => void = () => undefined;
    const response = new Promise<Response>((done) => {
      resolve = done;
    });
    const view = mount({ start: () => response });
    const user = userEvent.setup();
    await confirm(user);
    expect(screen.getByRole("status", { name: "正在启动账号安全任务" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "关闭安全任务" }));
    resolve(Response.json(session()));
    await waitFor(() =>
      expect(
        view.requests.some((v) => v.path.endsWith("security-1") && v.method === "DELETE"),
      ).toBe(true),
    );
    expect(screen.queryByRole("button", { name: "上游登录页面" })).not.toBeInTheDocument();
  });
  it("读取失败时禁用页面输入和继续操作并保留重试入口", async () => {
    mount({ get: () => Response.json({ detail: "读取失败" }, { status: 503 }) });
    const user = userEvent.setup();
    await confirm(user);
    expect(await screen.findByRole("button", { name: "重新读取" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "验证完成，继续" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "上游登录页面" })).toHaveAttribute(
      "aria-disabled",
      "true",
    );
  });
  it("离开账号安全页面时取消会话并清除截图缓存", async () => {
    const view = mount();
    await confirm(userEvent.setup());
    await screen.findByRole("button", { name: "上游登录页面" });
    view.unmount();
    await waitFor(() => expect(view.requests.some((v) => v.method === "DELETE")).toBe(true));
    expect(
      view.client.getQueryData(["account-workbench", "security", "security-1"]),
    ).toBeUndefined();
  });
  it("没有可用账号时禁用查看范围", async () => {
    mount({ empty: true });
    expect(await screen.findByText("暂无可设置安全信息的 OpenAI OAuth 账号")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "查看操作范围" })).toBeDisabled();
  });
});
