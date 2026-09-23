import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import type { PricingSnapshot } from "@/api";
import { PricingConfigPage, PricingPreviewTable, pricingPreviewDecisions } from "../pricing-page";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function fixture(): PricingSnapshot {
  return {
    config: {
      enabled: true,
      profit_margin: 0.2,
      exchange_group_sets: [["32", "35"]],
      exchange_group_set_names: ["国产价格"],
      interval_seconds: 120,
      write_concurrency: 4,
    },
    groups: [
      { id: "32", name: "国产-平价", rate_multiplier: "0.5" },
      { id: "35", name: "国产-特价", rate_multiplier: "0.2" },
      { id: "36", name: "国产-旗舰", rate_multiplier: "0.7" },
    ].map((group) => ({
      ...group,
      platform: "composite",
      status: "active",
      available: true,
      managed: true,
      reason: null,
    })),
    decisions: [
      {
        account_id: "41",
        account_name: "国产账号",
        platform: "openai",
        cost_multiplier: "0.15",
        current_group_ids: ["32"],
        desired_group_ids: ["35"],
        eligible_groups: ["国产-特价"],
        changed: true,
        skipped: false,
        reason: null,
      },
    ],
    accounts: 1,
    changes: 1,
    skipped: 0,
    generated_at: "2026-09-22T00:00:00Z",
  };
}

it("普通平台账号已在复合互换组时预览迁入最低盈利分组", () => {
  const data = fixture();
  const decisions = pricingPreviewDecisions(data.decisions, data.groups, data.config);
  expect(decisions[0].desired_group_ids).toEqual(["35"]);
  expect(decisions[0].eligible_groups).toEqual(["国产-特价"]);
});

it("展开复合分组计算明细时展示成本计算而非平台不匹配", async () => {
  const data = fixture();
  render(
    <PricingPreviewTable decisions={data.decisions} groups={data.groups} config={data.config} />,
  );
  await userEvent.setup().click(screen.getByRole("button", { name: "计算明细" }));
  expect(screen.getByText(/国产-特价（#35）：账号成本 0.15/)).toBeVisible();
  expect(screen.queryByText(/与账号平台.*不一致/)).not.toBeInTheDocument();
});

it("后端返回可用复合分组时允许键盘选入互换组并保存配置", async () => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const client = new QueryClient({ defaultOptions: { queries: { enabled: false } } });
  client.setQueryData(["pricing"], fixture());
  try {
    render(
      <QueryClientProvider client={client}>
        <PricingConfigPage />
      </QueryClientProvider>,
    );
    const checkbox = screen.getByRole("checkbox", { name: /^互换组 1 分组 国产-旗舰/ });
    expect(checkbox).toBeEnabled();
    checkbox.focus();
    await userEvent.setup().keyboard(" ");
    expect(checkbox).toBeChecked();
    expect(screen.getByRole("button", { name: "保存配置" })).toBeEnabled();
  } finally {
    client.clear();
  }
});
