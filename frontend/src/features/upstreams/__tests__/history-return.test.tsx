import type { QueryClient } from "@tanstack/react-query";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { api } from "@/api";
import { boundCandidate, renderOnboarding, upstream } from "./onboarding-fixture";

let client: QueryClient | undefined;
afterEach(() => {
  client?.clear();
  vi.restoreAllMocks();
});

it("从统计变化开户后返回时恢复原上游明细并可继续开户", async () => {
  client = renderOnboarding(boundCandidate("active"), true, undefined, {
    history: "overview",
    realUpstreamsPage: true,
  });
  vi.mocked(api.upstreams).mockResolvedValue({
    hosts: [
      {
        upstream_id: upstream.upstream_id,
        host: upstream.host,
        hosts: [upstream.host],
        base_url: upstream.base_url,
        name: upstream.name,
        upstream_type: "sub2api",
        account_count: 1,
        group_count: 1,
        auth_status: "已鉴权",
        balance: "10",
        raw_balance: "10",
        recharge_rate: "1",
        balance_status: "已读取",
        checked_at: null,
      },
    ],
    total_hosts: 1,
    authenticated_hosts: 1,
    recovery_required: 0,
    source: "Console",
  });
  vi.spyOn(api, "allUpstreamGroupHistory").mockResolvedValue([
    {
      id: 1,
      upstream_id: upstream.upstream_id,
      group_id: "7",
      group_name: "已有绑定分组",
      change_type: "added",
      changed_at: "2026-09-28T00:00:00Z",
    },
  ]);
  vi.spyOn(api, "upstreamGroupBindingAudit").mockResolvedValue({
    items: [],
    total_bindings: 0,
    present: 0,
    missing: 0,
    unknown: 0,
  });
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "返回统计变化" }));
  const dialog = await screen.findByRole("dialog", { name: "上游分组变化" });
  expect(
    await within(dialog).findByRole("button", { name: "收起 测试上游 的变化明细" }),
  ).toHaveAttribute("aria-expanded", "true");
  await user.click(within(dialog).getByRole("button", { name: "向已有绑定分组添加账号" }));
  expect(await screen.findByRole("button", { name: "返回统计变化" })).toBeVisible();
  await user.click(screen.getByRole("button", { name: "返回统计变化" }));
  const returned = await screen.findByRole("dialog", { name: "上游分组变化" });
  expect(
    await within(returned).findByRole("button", { name: "向已有绑定分组添加账号" }),
  ).toBeVisible();
});
