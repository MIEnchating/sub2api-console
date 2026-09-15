import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { WorkbenchMaintenance } from "@/api";
import { WorkbenchMaintenanceAuthorization } from "../components/workbench-maintenance-authorization";

const clients: QueryClient[] = [];
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
const config: WorkbenchMaintenance = {
  enabled: true,
  reauthorize_with_profiles: true,
  revision: 4,
  interval_minutes: 5,
  cooldown_minutes: 10,
  group_ids: ["7"],
  check_after_repair: false,
  model: "test-model",
};

function mount(options: { fail?: boolean; active?: boolean } = {}): {
  writes: unknown[];
  unmount: () => void;
} {
  const writes: unknown[] = [];
  let attached = options.active ?? false;
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input, init) => {
      const path = String(input);
      if (init?.method === "POST") {
        writes.push(JSON.parse(String(init.body)) as unknown);
        if (options.fail) return Response.json({ detail: "维护配置已变化" }, { status: 409 });
        attached = true;
      }
      if (init?.method === "DELETE") {
        writes.push("delete");
        return Response.json({ cancelled: true });
      }
      if (path.endsWith("/authorization"))
        return Response.json({
          attached,
          current_reauthorization_id: options.active ? "auto-batch" : undefined,
        });
      if (path.endsWith("/oauth-batches/auto-batch"))
        return Response.json({
          id: "auto-batch",
          task_id: "auto-batch",
          status: "running",
          message: "等待人工验证码",
          available: 0,
          items: [],
          current_oauth_id: "auto-child",
          expires_at: new Date(Date.now() + 600000).toISOString(),
        });
      if (path.endsWith("/oauth/auto-child"))
        return Response.json({
          id: "auto-child",
          task_id: "auto-child",
          status: "waiting",
          message: "等待人工验证码",
          width: 1100,
          height: 760,
          host: "auth.openai.com",
          expires_at: new Date(Date.now() + 600000).toISOString(),
        });
      return Response.json({ detail: "未配置此隔离请求" }, { status: 503 });
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  const view = render(
    <QueryClientProvider client={client}>
      <WorkbenchMaintenanceAuthorization config={config} />
    </QueryClientProvider>,
  );
  return { writes, unmount: view.unmount };
}

describe("自动维护授权会话", () => {
  it("未连接时先展示范围确认再按配置版本连接", async () => {
    const state = mount();
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "连接本次登录会话" }));
    const dialog = screen.getByRole("dialog", { name: "确认连接自动重新授权" });
    expect(dialog).toHaveTextContent("分组 ID 7");
    expect(state.writes).toHaveLength(0);
    await user.click(within(dialog).getByRole("button", { name: "确认连接" }));
    await screen.findByText("当前登录会话已连接自动重新授权");
    expect(state.writes).toEqual([{ revision: 4, confirmed: true }]);
  });
  it("连接失败时保留确认并允许重试", async () => {
    const state = mount({ fail: true });
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "连接本次登录会话" }));
    const dialog = screen.getByRole("dialog", { name: "确认连接自动重新授权" });
    await user.click(within(dialog).getByRole("button", { name: "确认连接" }));
    await waitFor(() => expect(state.writes).toHaveLength(1));
    await waitFor(() =>
      expect(within(dialog).getByRole("button", { name: "确认连接" })).toBeEnabled(),
    );
  });
  it("离开维护页面保留后端拥有的授权队列", async () => {
    const state = mount({ active: true });
    await screen.findByRole("button", { name: "取消本轮重新授权" });
    state.unmount();
    expect(state.writes).toHaveLength(0);
  });
});
