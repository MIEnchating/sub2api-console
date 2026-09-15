import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { UpstreamConfiguration } from "@/api";
import { UpstreamEditDialog } from "../upstream-edit-dialog";

afterEach(() => vi.unstubAllGlobals());

function fixture(): UpstreamConfiguration {
  return {
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
    header_names: [],
    cookie_names: [],
    groups: [],
  };
}

it("上游后台刷新余额时保留正在编辑的地址与Token", async () => {
  const config = fixture();
  const client = new QueryClient({ defaultOptions: { queries: { enabled: false, retry: false } } });
  client.setQueryData(["upstream-configuration", config.host], config);
  client.setQueryData(["auth-recovery-config"], { vault_entries: [] });
  const view = render(
    <QueryClientProvider client={client}>
      <UpstreamEditDialog
        host={config.host}
        onOpenChange={() => undefined}
        onSaved={() => undefined}
      />
    </QueryClientProvider>,
  );
  try {
    fireEvent.change(screen.getByLabelText("上游地址"), {
      target: { value: "https://edited.example.test" },
    });
    fireEvent.change(screen.getByPlaceholderText("已配置，留空则不修改"), {
      target: { value: "private-draft-token" },
    });
    await act(async () => {
      client.setQueryData(["upstream-configuration", config.host], { ...config, balance: "12" });
    });
    await screen.findByText("12");
    expect(screen.getByLabelText("上游地址")).toHaveValue("https://edited.example.test");
    expect(screen.getByPlaceholderText("已配置，留空则不修改")).toHaveValue("private-draft-token");
  } finally {
    view.unmount();
    client.clear();
  }
});

it.each([true, false])(
  "上游提交成功=%s时Token只进入请求且不进入共享变更缓存",
  async (succeeded) => {
    const config = fixture();
    const client = new QueryClient({
      defaultOptions: { queries: { enabled: false, retry: false }, mutations: { retry: false } },
    });
    client.setQueryData(["upstream-configuration", config.host], config);
    client.setQueryData(["auth-recovery-config"], { vault_entries: [] });
    let requestBody = "";
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
        requestBody = String(init?.body);
        return new Response(JSON.stringify(succeeded ? config : { error: "保存失败" }), {
          status: succeeded ? 200 : 503,
        });
      }),
    );
    const view = render(
      <QueryClientProvider client={client}>
        <UpstreamEditDialog
          host={config.host}
          onOpenChange={() => undefined}
          onSaved={() => undefined}
        />
      </QueryClientProvider>,
    );
    try {
      fireEvent.change(screen.getByPlaceholderText("已配置，留空则不修改"), {
        target: { value: "private-submitted-token" },
      });
      fireEvent.click(screen.getByRole("button", { name: "保存并重算" }));
      await waitFor(() => expect(requestBody).toContain("private-submitted-token"));
      await waitFor(() => expect(client.isMutating()).toBe(0));
      expect(
        JSON.stringify(
          client
            .getMutationCache()
            .getAll()
            .map((mutation) => mutation.state),
        ),
      ).not.toContain("private-submitted-token");
    } finally {
      view.unmount();
      client.clear();
    }
  },
);

it("切换上游后旧保存响应仅更新原上游且保留当前编辑器", async () => {
  const first = fixture();
  const second = {
    ...fixture(),
    host: "second.example.test",
    name: "第二上游",
    base_url: "https://second.example.test",
  };
  const client = new QueryClient({ defaultOptions: { queries: { enabled: false, retry: false } } });
  client.setQueryData(["upstream-configuration", first.host], first);
  client.setQueryData(["upstream-configuration", second.host], second);
  client.setQueryData(["auth-recovery-config"], { vault_entries: [] });
  let finishRequest: ((value: Response) => void) | undefined;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "PUT")
        return new Promise<Response>((resolve) => {
          finishRequest = resolve;
        });
      return Response.json(second);
    }),
  );
  const onOpenChange = vi.fn();
  const editor = (host: string) => (
    <QueryClientProvider client={client}>
      <UpstreamEditDialog host={host} onOpenChange={onOpenChange} onSaved={() => undefined} />
    </QueryClientProvider>
  );
  const view = render(editor(first.host));
  try {
    fireEvent.click(screen.getByRole("button", { name: "保存并重算" }));
    await waitFor(() => expect(finishRequest).toBeDefined());
    view.rerender(editor(second.host));
    await waitFor(() => expect(screen.getByLabelText("上游地址")).toHaveValue(second.base_url));
    await act(async () => {
      finishRequest?.(Response.json(first));
    });
    await waitFor(() => expect(client.isMutating()).toBe(0));
    expect(client.getQueryData(["upstream-configuration", second.host])).toEqual(second);
    expect(screen.getByLabelText("上游地址")).toHaveValue(second.base_url);
    expect(onOpenChange).not.toHaveBeenCalled();
  } finally {
    view.unmount();
    client.clear();
  }
});
