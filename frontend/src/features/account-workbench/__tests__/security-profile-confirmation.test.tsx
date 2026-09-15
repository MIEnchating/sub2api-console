import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { WorkbenchSecurityProfile } from "../components/workbench-security-profile";
import { WorkbenchSourceSecurityProfile } from "../components/workbench-source-security-profile";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("安全结果回写确认版本", () => {
  it.each([
    ["托管", "profiles", "更新已保存的登录资料", "确认更新资料"],
    ["本地", "source-profiles", "更新本地登录资料", "确认写入本地资料"],
  ])("%s资料确认期间后台更新版本时仍提交原确认版本", async (scope, key, open, confirm) => {
    let revision = 4;
    const writes: Array<{ revision: number }> = [];
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>(async (_input, init) => {
        if (init?.method === "POST") {
          writes.push(JSON.parse(String(init.body)) as { revision: number });
          return Response.json({ detail: "资料版本已变化" }, { status: 409 });
        }
        return Response.json([
          {
            id: "profile-a",
            scope: "local-export",
            account_id: "account-a",
            user_id: "official-user-a",
            workspace_id: "workspace-a",
            email: "owner@example.test",
            revision,
            has_password: true,
            has_totp: false,
            updated_at: "2026-09-14T00:00:00Z",
          },
        ]);
      }),
    );
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    clients.push(client);
    render(
      <QueryClientProvider client={client}>
        {scope === "本地" ? (
          <WorkbenchSourceSecurityProfile
            securityId="security-a"
            userId="official-user-a"
            workspaceId="workspace-a"
          />
        ) : (
          <WorkbenchSecurityProfile accountId="account-a" securityId="security-a" />
        )}
      </QueryClientProvider>,
    );
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: open }));
    await act(async () => {
      revision = 5;
      await client.invalidateQueries({ queryKey: ["account-workbench", key] });
    });
    await waitFor(() => expect(screen.getByRole("button", { name: confirm })).toBeEnabled());
    await user.click(screen.getByRole("button", { name: confirm }));
    await waitFor(() => expect(writes).toHaveLength(1));
    expect(writes[0]?.revision).toBe(4);
  });
});
