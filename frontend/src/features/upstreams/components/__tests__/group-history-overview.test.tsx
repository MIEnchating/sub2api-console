import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterContextProvider } from "@tanstack/react-router";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { UpstreamsPage } from "../../../../App";
import { router } from "../../../../router";

beforeEach(() => {
  const matches = Element.prototype.matches;
  vi.spyOn(Element.prototype, "matches").mockImplementation(function (
    this: Element,
    selector: string,
  ) {
    if ([":fullscreen", ":popover-open", ":modal"].includes(selector)) return false;
    return matches.call(this, selector);
  });
});
afterEach(() => vi.restoreAllMocks());

describe("上游分组变化汇总", () => {
  it("从顶部维护菜单打开统计变化后显示各上游的分组变化记录", async () => {
    const user = userEvent.setup();
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity } },
    });
    queryClient.setQueryData(["upstreams"], {
      hosts: [
        {
          upstream_id: "upstream-1",
          host: "api.example.test",
          hosts: ["api.example.test"],
          base_url: "https://api.example.test",
          name: "示例上游",
          upstream_type: "sub2api",
          account_count: 2,
          group_count: 1,
          auth_status: "已鉴权",
          raw_balance: "10",
          balance: "10",
          recharge_rate: "1",
          balance_status: "已读取",
          checked_at: "2026-09-04T00:00:00Z",
        },
      ],
      total_hosts: 1,
      authenticated_hosts: 1,
      recovery_required: 0,
      source: "Console 业务库",
    });
    queryClient.setQueryData(["config"], { mode: "完全模式" });
    queryClient.setQueryData(
      ["upstream-group-history-overview"],
      [
        {
          id: 1,
          upstream_id: "upstream-1",
          group_id: "7",
          group_name: "新分组",
          change_type: "added",
          changed_at: "2026-09-05T01:00:00Z",
        },
      ],
    );
    render(
      <QueryClientProvider client={queryClient}>
        <RouterContextProvider router={router}>
          <UpstreamsPage />
        </RouterContextProvider>
      </QueryClientProvider>,
    );

    screen.getByRole("button", { name: "上游维护" }).focus();
    await user.keyboard("{Enter}");
    await user.click(await screen.findByRole("menuitem", { name: "统计变化" }));

    const dialog = screen.getByRole("dialog", { name: "上游分组变化" });
    expect(dialog).toBeVisible();
    expect(within(dialog).getByText("示例上游")).toBeVisible();
    expect(within(dialog).getByText("api.example.test")).toBeVisible();
    expect(within(dialog).getByText("新分组")).toBeVisible();
  });
});
