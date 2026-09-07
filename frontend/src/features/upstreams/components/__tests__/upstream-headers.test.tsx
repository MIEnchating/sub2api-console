import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { UpstreamConfiguration, UpstreamConfigurationUpdate } from "@/api";
import { UpstreamEditDialog } from "../upstream-edit-dialog";

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => vi.unstubAllGlobals());

function renderConfiguredUpstream(): { writes: UpstreamConfigurationUpdate[] } {
  const configuration: UpstreamConfiguration = {
    upstream_id: "up_example",
    host: "api.example.test",
    name: "测试上游",
    base_url: "https://api.example.test",
    account_base_url: "https://api.example.test/v1",
    upstream_type: "sub2api",
    auth_mode: "sub2api_user_token",
    recharge_rate: "1",
    raw_balance: null,
    balance: null,
    has_access_token: true,
    has_refresh_token: false,
    has_admin_key: false,
    has_user_id: false,
    headers: {},
    header_names: ["Authorization"],
    cookie_names: [],
    groups: [],
  };
  const writes: UpstreamConfigurationUpdate[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "PUT")
        writes.push(JSON.parse(String(init.body)) as UpstreamConfigurationUpdate);
      return new Response(JSON.stringify(configuration), {
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  client.setQueryData(["upstream-configuration", configuration.host], configuration);
  client.setQueryData(["auth-recovery-config"], { vault_entries: [] });
  render(
    <QueryClientProvider client={client}>
      <UpstreamEditDialog
        host={configuration.host}
        onOpenChange={() => undefined}
        onSaved={() => undefined}
      />
    </QueryClientProvider>,
  );
  return { writes };
}

describe("上游自定义 Header 编辑", () => {
  it("已配置 Header 留空保存时省略字段并保留服务端值", async () => {
    const user = userEvent.setup();
    const state = renderConfiguredUpstream();

    await user.click(await screen.findByRole("button", { name: "保存并重算" }));

    await waitFor(() => expect(state.writes).toHaveLength(1));
    expect(state.writes[0]).not.toHaveProperty("headers");
  });

  it("关闭已配置的自定义 Header 时提交空对象清空配置", async () => {
    const user = userEvent.setup();
    const state = renderConfiguredUpstream();

    await user.click(await screen.findByRole("switch", { name: /自定义 Headers/ }));
    await user.click(screen.getByRole("button", { name: "保存并重算" }));

    await waitFor(() => expect(state.writes).toHaveLength(1));
    expect(state.writes[0].headers).toEqual({});
  });

  it("填写新的 Header JSON 时提交替换值并提供字段可访问名称", async () => {
    const user = userEvent.setup();
    const state = renderConfiguredUpstream();

    const headers = await screen.findByRole("textbox", { name: "Headers JSON" });
    await user.click(headers);
    await user.paste('{"X-Client":"replacement"}');
    await user.click(screen.getByRole("button", { name: "保存并重算" }));

    await waitFor(() => expect(state.writes).toHaveLength(1));
    expect(state.writes[0].headers).toEqual({ "X-Client": "replacement" });
  });
});
