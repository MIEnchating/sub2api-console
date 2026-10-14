import { QueryClient, QueryClientProvider, useQuery } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, onTestFinished, vi } from "vitest";

import { api, type UpstreamConfiguration, type UpstreamConfigurationUpdate } from "@/api";
import { account } from "@/features/accounts/__tests__/fixtures";
import { UpstreamEditDialog } from "../upstream-edit-dialog";

afterEach(() => vi.unstubAllGlobals());

function CachedAccountAddress() {
  const accounts = useQuery({
    queryKey: ["accounts"],
    queryFn: ({ signal }) => api.accounts(signal),
  });
  return <output aria-label="账号当前上游地址">{accounts.data?.[0]?.upstream_base_url}</output>;
}

function renderUpstream(options?: { savedHost: string; showAccounts: boolean }): {
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
  let savedBaseURL = configuration.base_url;
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (String(input) === "/api/accounts") {
        return Response.json([{ ...account, upstream_base_url: savedBaseURL }]);
      }
      if (String(input).includes("/policy/upstream-concurrency/")) {
        return Response.json({
          revision: "v1",
          target_id: "up_example",
          upstream_id: "up_example",
          override: null,
          selected: false,
          effective: false,
          global_enabled: false,
          source: "policy",
        });
      }
      if (init?.method !== "PUT") throw new Error(`Unexpected request: ${String(input)}`);
      const payload = JSON.parse(String(init.body)) as UpstreamConfigurationUpdate;
      writes.push({ url: String(input), payload });
      savedBaseURL = payload.base_url;
      return new Response(
        JSON.stringify({
          ...configuration,
          ...payload,
          host: options?.savedHost ?? "new.example.test:8443",
        }),
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
  client.setQueryData(["accounts"], [{ ...account, upstream_base_url: configuration.base_url }]);
  const onSaved = vi.fn();
  const onOpenChange = vi.fn();
  render(
    <QueryClientProvider client={client}>
      {options?.showAccounts ? <CachedAccountAddress /> : null}
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
  it("打开编辑窗口时名称标签与跟随开关保持同一行", async () => {
    renderUpstream();
    const follow = await screen.findByRole("switch", { name: "跟随上游站点名称" });
    const row = follow.closest('[data-slot="field-label-row"]');
    expect(row).toHaveClass("flex", "items-center", "justify-between", "flex-nowrap");
    expect(row).toContainElement(screen.getByText("名称", { selector: "label" }));
    expect(row).not.toContainElement(screen.getByRole("textbox", { name: "名称" }));
  });

  it("保存地址时保留自动名称来源", async () => {
    const user = userEvent.setup();
    const state = renderUpstream();
    expect(await screen.findByRole("switch", { name: "跟随上游站点名称" })).toBeChecked();
    await user.click(screen.getByRole("button", { name: "保存并重算" }));
    await waitFor(() => expect(state.onSaved).toHaveBeenCalled());
    expect(state.writes[0].payload.name_mode).toBe("auto");
    expect(state.writes[0].payload.name).toBeUndefined();
  });

  it("手动修改名称时关闭跟随并提交自定义名称", async () => {
    const user = userEvent.setup();
    const state = renderUpstream();
    const name = await screen.findByRole("textbox", { name: "名称" });
    await user.clear(name);
    await user.type(name, "自定义上游");
    expect(screen.getByRole("switch", { name: "跟随上游站点名称" })).not.toBeChecked();
    await user.click(screen.getByRole("button", { name: "保存并重算" }));
    await waitFor(() => expect(state.onSaved).toHaveBeenCalled());
    expect(state.writes[0].payload).toMatchObject({ name: "自定义上游", name_mode: "manual" });
  });

  it("通过键盘恢复跟随时不提交旧的自定义名称", async () => {
    const user = userEvent.setup();
    const state = renderUpstream();
    const name = await screen.findByRole("textbox", { name: "名称" });
    await user.clear(name);
    await user.type(name, "自定义上游");
    const follow = screen.getByRole("switch", { name: "跟随上游站点名称" });
    follow.focus();
    await user.keyboard(" ");
    expect(follow).toBeChecked();
    await user.click(screen.getByRole("button", { name: "保存并重算" }));
    await waitFor(() => expect(state.onSaved).toHaveBeenCalled());
    expect(state.writes[0].payload.name_mode).toBe("auto");
    expect(state.writes[0].payload.name).toBeUndefined();
  });

  it("打开旧 Host 对应的配置时副标题显示当前名称和地址", async () => {
    renderUpstream();
    await screen.findByRole("textbox", { name: "上游地址" });
    expect(screen.getByRole("dialog", { name: "编辑上游" })).toHaveAccessibleDescription(
      "测试上游 · https://admin.example.test/console",
    );
    expect(screen.queryByText("old.example.test")).not.toBeInTheDocument();
  });

  it.each(["old.example.test", "new.example.test:8443"])(
    "保存后绑定 Host 为 %s 时，已缓存的账号重新读取当前上游地址",
    async (savedHost) => {
      const user = userEvent.setup();
      renderUpstream({ savedHost, showAccounts: true });
      expect(screen.getByLabelText("账号当前上游地址")).toHaveTextContent(
        "https://admin.example.test/console",
      );
      const address = await screen.findByRole("textbox", { name: "上游地址" });
      await user.clear(address);
      await user.type(address, "http://new.example.test:8443/admin");
      await user.click(screen.getByRole("button", { name: "保存并重算" }));
      await waitFor(() =>
        expect(screen.getByLabelText("账号当前上游地址")).toHaveTextContent(
          "http://new.example.test:8443/admin",
        ),
      );
    },
  );

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
