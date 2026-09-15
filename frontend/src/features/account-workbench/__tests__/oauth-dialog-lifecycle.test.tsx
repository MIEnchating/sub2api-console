import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
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

describe("授权弹窗会话失效", () => {
  it.each([
    ["保存本地登录资料", "新增登录资料"],
    ["设置账号安全", "授权账号安全设置"],
  ])("打开%s后授权过期会关闭弹窗并允许重新登录", async (button, title) => {
    let status: WorkbenchOAuthSession["status"] = "authorized";
    const expires = new Date(Date.now() + 600000).toISOString();
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>(async (input, init) => {
        const path = String(input);
        if (init?.method === "DELETE") return Response.json({ cancelled: true });
        if (path.endsWith("/templates")) return Response.json([]);
        if (path.endsWith("/source-profiles/source") || path.endsWith("/security-source"))
          return Response.json({
            scope: "local-export",
            source: { source_oauth_id: "local-oauth" },
            source_oauth_id: "local-oauth",
            source_revision: "source-version",
            email: "owner@example.test",
            user_id: "official-user",
            workspace_id: "official-workspace",
            expires_at: expires,
          });
        if (path === "/api/account-workbench/oauth" || path.endsWith("/oauth/local-oauth"))
          return Response.json({
            id: "local-oauth",
            task_id: "local-task",
            scope: "local-export",
            host: "auth.openai.com",
            status,
            message: "",
            expires_at: expires,
            width: 1100,
            height: 760,
          } satisfies WorkbenchOAuthSession);
        throw new Error(`未预期请求 ${path}`);
      }),
    );
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    clients.push(client);
    render(
      <QueryClientProvider client={client}>
        <WorkbenchOAuth scope="local-export" />
      </QueryClientProvider>,
    );
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "开始授权登录" }));
    await user.click(await screen.findByRole("button", { name: button }));
    await screen.findByRole("dialog", { name: title });

    await act(async () => {
      status = "expired";
      await client.invalidateQueries({ queryKey: workbenchKeys.oauth("local-oauth") });
    });

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(screen.getByRole("button", { name: "重新授权登录" })).toBeEnabled();
    status = "authorized";
    await user.click(screen.getByRole("button", { name: "重新授权登录" }));
    expect(await screen.findByRole("button", { name: button })).toBeEnabled();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
});
