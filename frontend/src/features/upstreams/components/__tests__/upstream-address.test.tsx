import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, onTestFinished, vi } from "vitest";

import type { UpstreamConfiguration, UpstreamConfigurationUpdate } from "@/api";
import { UpstreamEditDialog } from "../upstream-edit-dialog";

afterEach(() => vi.unstubAllGlobals());

function renderUpstream(): {
  writes: { url: string; payload: UpstreamConfigurationUpdate }[];
  onSaved: () => void;
  onOpenChange: (open: boolean) => void;
} {
  const configuration: UpstreamConfiguration = {
    upstream_id: "up_example",
    host: "old.example.test",
    name: "测试上游",
    base_url: "https://admin.example.test/console",
    account_base_url: "https://models.example.test/v1",
    upstream_type: "sub2api",
    auth_mode: "sub2api_user_token",
    recharge_rate: "1",
    raw_balance: null,
    balance: null,
    has_access_token: true,
    has_refresh_token: true,
    has_admin_key: false,
    has_user_id: false,
    headers: {},
    header_names: [],
    cookie_names: [],
    groups: [],
  };
  const writes: { url: string; payload: UpstreamConfigurationUpdate }[] = [];
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method !== "PUT") throw new Error(`Unexpected request: ${String(input)}`);
      const payload = JSON.parse(String(init.body)) as UpstreamConfigurationUpdate;
      writes.push({ url: String(input), payload });
      return new Response(
        JSON.stringify({ ...configuration, ...payload, host: "new.example.test:8443" }),
        {
          headers: { "Content-Type": "application/json" },
        },
      );
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  client.setQueryData(["upstream-configuration", configuration.host], configuration);
  client.setQueryData(["auth-recovery-config"], { vault_entries: [] });
  const onSaved = vi.fn();
  const onOpenChange = vi.fn();
  render(
    <QueryClientProvider client={client}>
      <UpstreamEditDialog host={configuration.host} onOpenChange={onOpenChange} onSaved={onSaved} />
    </QueryClientProvider>,
  );
  onTestFinished(() => {
    cleanup();
    client.clear();
  });
  return { writes, onSaved, onOpenChange };
}

describe("编辑上游地址", () => {
  it("打开已有配置时只显示完整上游地址和独立账号地址", async () => {
    renderUpstream();

    expect(await screen.findByRole("textbox", { name: "上游地址" })).toHaveValue(
      "https://admin.example.test/console",
    );
    expect(screen.getByRole("textbox", { name: "账号 Base URL" })).toHaveValue(
      "https://models.example.test/v1",
    );
    expect(screen.queryByText("上游 Host")).not.toBeInTheDocument();
    expect(screen.queryByText("请求 Base URL")).not.toBeInTheDocument();
  });

  it("键盘修改上游地址后向原上游提交单一地址并在迁移成功后关闭弹窗", async () => {
    const user = userEvent.setup();
    const state = renderUpstream();
    const address = await screen.findByRole("textbox", { name: "上游地址" });
    await user.clear(address);
    await user.type(address, "http://new.example.test:8443/admin");
    await user.tab();
    expect(screen.getByRole("textbox", { name: "账号 Base URL" })).toHaveFocus();
    await user.click(screen.getByRole("button", { name: "保存并重算" }));

    await waitFor(() => expect(state.onSaved).toHaveBeenCalled());
    expect(state.writes).toHaveLength(1);
    expect(state.writes[0].url).toBe("/api/upstreams/old.example.test/configuration");
    expect(state.writes[0].payload).toMatchObject({
      base_url: "http://new.example.test:8443/admin",
      account_base_url: "https://models.example.test/v1",
    });
    expect(state.writes[0].payload).not.toHaveProperty("host");
    expect(state.onOpenChange).toHaveBeenCalledWith(false);
  });

  it("上游地址缺少协议时显示字段错误且不发送保存请求", async () => {
    const user = userEvent.setup();
    const state = renderUpstream();
    const address = await screen.findByRole("textbox", { name: "上游地址" });
    await user.clear(address);
    await user.type(address, "admin.example.test");
    await user.click(screen.getByRole("button", { name: "保存并重算" }));

    await waitFor(() => expect(address).toHaveAttribute("aria-invalid", "true"));
    expect(address).toHaveAccessibleDescription(/完整的 HTTP\/HTTPS 地址/);
    expect(address).toHaveFocus();
    expect(state.writes).toHaveLength(0);
  });
});
