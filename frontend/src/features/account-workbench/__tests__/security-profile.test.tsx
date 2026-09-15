import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { WorkbenchSecurityProfile } from "../components/workbench-security-profile";

const clients: QueryClient[] = [];
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function mount(options: { batch?: boolean; fail?: boolean; missing?: boolean } = {}): unknown[] {
  const writes: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (_input, init) => {
      if (init?.method === "POST") {
        writes.push(JSON.parse(String(init.body)) as unknown);
        if (options.fail) return Response.json({ detail: "资料版本已变化" }, { status: 409 });
        return Response.json({ id: "profile", revision: 4 });
      }
      return Response.json(
        options.missing
          ? []
          : [{ id: "profile", account_id: "42", email: "owner@example.com", revision: 3 }],
      );
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <WorkbenchSecurityProfile
        accountId="42"
        securityId={options.batch ? undefined : "security"}
        batchId={options.batch ? "batch" : undefined}
      />
    </QueryClientProvider>,
  );
  return writes;
}

describe("安全结果更新登录资料", () => {
  it.each([false, true])(
    "单项或批次成功结果（批次=%s）确认后仅提交稳定ID和资料版本",
    async (batch) => {
      const writes = mount({ batch });
      const user = userEvent.setup();
      await user.click(await screen.findByRole("button", { name: "更新已保存的登录资料" }));
      const dialog = screen.getByRole("dialog", { name: "确认更新登录资料" });
      expect(dialog).toHaveTextContent("账号 ID：42");
      expect(dialog).toHaveTextContent("资料版本：3");
      expect(writes).toHaveLength(0);
      await user.click(within(dialog).getByRole("button", { name: "确认更新资料" }));
      await waitFor(() =>
        expect(screen.getByRole("button", { name: "已更新登录资料" })).toBeDisabled(),
      );
      const body = { profile_id: "profile", revision: 3, account_id: "42", confirmed: true };
      expect(writes).toEqual([
        batch ? { ...body, batch_id: "batch" } : { ...body, security_id: "security" },
      ]);
    },
  );

  it("资料版本冲突时保留确认并允许重新处理", async () => {
    mount({ fail: true });
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "更新已保存的登录资料" }));
    const dialog = screen.getByRole("dialog", { name: "确认更新登录资料" });
    await user.click(within(dialog).getByRole("button", { name: "确认更新资料" }));
    await waitFor(() =>
      expect(within(dialog).getByRole("button", { name: "确认更新资料" })).toBeEnabled(),
    );
    expect(screen.queryByRole("button", { name: "已更新登录资料" })).not.toBeInTheDocument();
  });

  it("账号没有保存资料时不提供结果写入入口", async () => {
    mount({ missing: true });
    await screen.findByText("此账号尚未保存登录资料。");
    expect(screen.queryByRole("button", { name: "更新已保存的登录资料" })).not.toBeInTheDocument();
  });
});
