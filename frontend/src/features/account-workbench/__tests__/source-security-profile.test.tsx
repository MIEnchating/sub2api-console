import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { WorkbenchSourceSecurityProfile } from "../components/workbench-source-security-profile";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
function mount(options: { wrongWorkspace?: boolean; conflict?: boolean } = {}) {
  const requests: Array<{ path: string; body: unknown }> = [];
  let revision = 4;
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input, init) => {
      const path = String(input);
      if (init?.method === "POST") {
        requests.push({ path, body: JSON.parse(String(init.body)) as unknown });
        revision = 5;
        return options.conflict
          ? Response.json({ detail: "登录资料版本已变化" }, { status: 409 })
          : Response.json({ saved: true });
      }
      expect(path).toBe("/api/account-workbench/source-profiles?scope=local-export");
      return Response.json([
        {
          id: "local-profile",
          scope: "local-export",
          user_id: "user-1",
          workspace_id: options.wrongWorkspace ? "workspace-2" : "workspace-1",
          email: "owner@example.test",
          revision,
          updated_at: "2026-09-14T00:00:00Z",
          has_password: false,
          has_totp: false,
        },
      ]);
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <WorkbenchSourceSecurityProfile
        securityId="security-1"
        userId="user-1"
        workspaceId="workspace-1"
      />
    </QueryClientProvider>,
  );
  return requests;
}
describe("本地安全结果更新资料", () => {
  it("官方用户和工作区同时匹配时展示版本确认并只提交安全任务ID", async () => {
    const requests = mount();
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "更新本地登录资料" }));
    expect(screen.getByRole("dialog", { name: "更新本地登录资料" })).toHaveTextContent("版本 4");
    expect(requests).toEqual([]);
    await user.click(screen.getByRole("button", { name: "确认写入本地资料" }));
    await screen.findByRole("button", { name: "已更新本地登录资料" });
    expect(requests).toEqual([
      {
        path: "/api/account-workbench/source-profiles/local-profile/security",
        body: { scope: "local-export", revision: 4, security_id: "security-1", confirmed: true },
      },
    ]);
  });
  it("只有邮箱用户相同但工作区不同的资料不能接收安全结果", async () => {
    mount({ wrongWorkspace: true });
    await screen.findByText("此官方账号尚未保存本地登录资料");
    expect(screen.queryByRole("button", { name: "更新本地登录资料" })).not.toBeInTheDocument();
  });
  it("版本冲突后关闭旧确认并重新读取，再次确认使用新版本", async () => {
    const requests = mount({ conflict: true });
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "更新本地登录资料" }));
    await user.click(screen.getByRole("button", { name: "确认写入本地资料" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    await user.click(screen.getByRole("button", { name: "更新本地登录资料" }));
    expect(screen.getByRole("dialog", { name: "更新本地登录资料" })).toHaveTextContent("版本 5");
    expect(requests).toHaveLength(1);
  });
});
