import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import { NewAPIManagementPage } from "../newapi-management-page";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

function mountPage(view: "platform" | "groups" | "channels" | "prices" | "differences"): void {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <NewAPIManagementPage view={view} />
    </QueryClientProvider>,
  );
}

it("首次平台请求失败只提供重试，成功确认没有平台后才显示未配置", async () => {
  let failed = true;
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      failed
        ? Response.json({ detail: "配置读取暂时不可用" }, { status: 503 })
        : Response.json({ platforms: [], local_groups: [], bindings: [] }),
    ),
  );
  mountPage("platform");
  const retry = await screen.findByRole("button", { name: "重新读取" });
  expect(screen.queryByText("尚未添加 New API 平台配置")).not.toBeInTheDocument();
  failed = false;
  await userEvent.setup().click(retry);
  expect(await screen.findByText("尚未添加 New API 平台配置")).toBeVisible();
});

it.each(["groups", "channels", "prices", "differences"] as const)(
  "%s 首次远端请求失败显示重试，不能操作缺少数据的表单",
  async (view) => {
    let failed = true;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes("/refresh"))
          return failed
            ? Response.json({ detail: "远端读取暂时不可用" }, { status: 503 })
            : Response.json({
                groups: [],
                models: [],
                unset_models: [],
                references: [],
                tool_prices: [],
                differences: [],
                upstream_prices: [],
              });
        if (url.includes("/auth-recovery/config"))
          return Response.json({ auth_records: [], vault_entries: [] });
        if (url.includes("/dictionaries")) return Response.json({ items: [] });
        return Response.json({
          platforms: [
            {
              id: "primary",
              name: "测试平台",
              base_url: "https://newapi.example.test",
              user_id: "1",
              admin_key_configured: true,
              updated_at: "",
            },
          ],
          local_groups: [],
          bindings: [],
          sub2api_base_url: "https://target.example.test",
        });
      }),
    );
    mountPage(view);
    const retry = await screen.findByRole("button", { name: "重新读取" });
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
    failed = false;
    await userEvent.setup().click(retry);
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: "重新读取" })).not.toBeInTheDocument(),
    );
  },
);
