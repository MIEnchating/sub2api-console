import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import type { UpstreamConfiguration, UpstreamConfigurationUpdate, VaultEntryIndex } from "@/api";
import { UpstreamEditDialog } from "../upstream-edit-dialog";

const clients: QueryClient[] = [];
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => clients.splice(0).forEach((client) => client.clear()));

function renderVaultEditor(options?: {
  indexResponse?: Promise<Response>;
  entry?: string | null;
}): {
  client: QueryClient;
  configuration: UpstreamConfiguration;
  entries: VaultEntryIndex[];
  writes: UpstreamConfigurationUpdate[];
} {
  const configuration: UpstreamConfiguration = {
    entry: options?.entry === undefined ? "测试登录资料" : options.entry,
    upstream_id: "up_example",
    host: "api.example.test",
    name: "测试上游",
    base_url: "https://api.example.test",
    account_base_url: "https://api.example.test/v1",
    upstream_type: "sub2api",
    auth_mode: "sub2api_user_login",
    recharge_rate: "1",
    raw_balance: null,
    balance: null,
    has_access_token: true,
    has_refresh_token: false,
    has_admin_key: false,
    has_user_id: false,
    headers: {},
    header_names: [],
    cookie_names: [],
    groups: [],
  };
  const entries: VaultEntryIndex[] = [
    {
      entry: "测试登录资料",
      hosts: [],
      has_username: true,
      has_password: true,
      username_is_email: true,
      header_names: [],
    },
    {
      entry: "Host 默认资料",
      hosts: [configuration.host],
      has_username: true,
      has_password: true,
      username_is_email: true,
      header_names: [],
    },
  ];
  const writes: UpstreamConfigurationUpdate[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method !== "PUT") {
        if (options?.indexResponse) return options.indexResponse;
        throw new Error("未配置的测试请求");
      }
      const payload = JSON.parse(String(init.body)) as UpstreamConfigurationUpdate;
      writes.push(payload);
      return Response.json({ ...configuration, entry: payload.entry });
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(client);
  client.setQueryData(["dictionaries", "upstream_type"], { items: [] });
  client.setQueryData(["upstream-configuration", configuration.host], configuration);
  if (!options?.indexResponse) {
    client.setQueryData(["auth-recovery-config"], { vault_entries: entries });
  }
  render(
    <QueryClientProvider client={client}>
      <UpstreamEditDialog
        host={configuration.host}
        onOpenChange={() => undefined}
        onSaved={() => undefined}
      />
    </QueryClientProvider>,
  );
  return { client, configuration, entries, writes };
}

it("打开上游编辑时优先回显已保存但未关联 Host 的密码项", async () => {
  renderVaultEditor();

  await waitFor(() =>
    expect(screen.getByRole("combobox", { name: "密码箱密码项" })).toHaveTextContent(
      "测试登录资料",
    ),
  );
});

it("未记录密码项时仍回显明确关联当前 Host 的默认项", async () => {
  renderVaultEditor({ entry: null });

  await waitFor(() =>
    expect(screen.getByRole("combobox", { name: "密码箱密码项" })).toHaveTextContent(
      "Host 默认资料",
    ),
  );
});

it("密码项索引稍后返回时保留已保存的选择", async () => {
  let resolveIndex: ((response: Response) => void) | undefined;
  const indexResponse = new Promise<Response>((resolve) => {
    resolveIndex = resolve;
  });
  const state = renderVaultEditor({ indexResponse });
  await waitFor(() =>
    expect(screen.getByRole("combobox", { name: "密码箱密码项" })).toHaveTextContent(
      "测试登录资料",
    ),
  );
  await act(async () => {
    resolveIndex?.(Response.json({ vault_entries: state.entries }));
    await indexResponse;
  });
  await waitFor(() =>
    expect(state.client.getQueryState(["auth-recovery-config"])?.status).toBe("success"),
  );
  expect(screen.getByRole("combobox", { name: "密码箱密码项" })).toHaveTextContent("测试登录资料");
});

it("键盘更换密码项后刷新配置不会覆盖选择，保存提交并回显新项", async () => {
  const user = userEvent.setup();
  const state = renderVaultEditor();
  const select = await screen.findByRole("combobox", { name: "密码箱密码项" });
  await waitFor(() => expect(select).toHaveTextContent("测试登录资料"));
  select.focus();
  await user.keyboard("{ArrowDown}");
  expect(select).toHaveAttribute("aria-expanded", "true");
  await user.keyboard("{Home}{Enter}");
  await waitFor(() => expect(select).toHaveTextContent("Host 默认资料"));
  act(() =>
    state.client.setQueryData(["upstream-configuration", state.configuration.host], {
      ...state.configuration,
      name: "后台刷新名称",
    }),
  );
  expect(select).toHaveTextContent("Host 默认资料");
  await user.click(screen.getByRole("button", { name: "保存并重算" }));
  await waitFor(() => expect(state.writes).toHaveLength(1));
  expect(state.writes[0].entry).toBe("Host 默认资料");
  expect(select).toHaveTextContent("Host 默认资料");
});
