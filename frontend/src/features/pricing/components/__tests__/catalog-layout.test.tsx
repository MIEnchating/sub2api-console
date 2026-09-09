import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";

import type { PricingSnapshot } from "@/api";
import { PricingPage, PricingPreviewTable } from "../pricing-page";

const snapshot: PricingSnapshot = {
  config: {
    enabled: false,
    profit_margin: 0.2,
    interval_seconds: 120,
    write_concurrency: 4,
    exchange_group_sets: [],
    exchange_group_set_names: [],
  },
  groups: [
    {
      id: "6",
      name: "标准",
      platform: "openai",
      status: "active",
      managed: true,
      available: true,
      rate_multiplier: "1",
      reason: null,
    },
  ],
  decisions: [],
  accounts: 0,
  changes: 0,
  skipped: 0,
  generated_at: "2026-09-09T00:00:00Z",
};

it("价格分组已加载时搜索栏位于页面滚动区之外，筛选后可恢复列表", async () => {
  const client = new QueryClient({ defaultOptions: { queries: { enabled: false } } });
  client.setQueryData(["pricing"], snapshot);
  render(
    <QueryClientProvider client={client}>
      <PricingPage />
    </QueryClientProvider>,
  );
  const search = screen.getByRole("textbox", { name: "搜索分组、ID 或平台" });
  expect(search.closest('[data-slot="page-content"]')).toBeNull();
  const user = userEvent.setup();
  await user.type(search, "找不到");
  expect(screen.getByText("没有匹配的分组")).toBeVisible();
  await user.clear(search);
  expect(screen.getByRole("cell", { name: "标准 #6" })).toBeVisible();
});

it("调整预览为空时空提示跟随可视表格宽度并完整换行", () => {
  render(<PricingPreviewTable decisions={[]} groups={[]} config={snapshot.config} />);
  expect(screen.getByText("当前筛选条件下没有账号")).toHaveClass("w-[100cqi]", "left-0");
  expect(screen.getByRole("cell", { name: "当前筛选条件下没有账号" })).toHaveAttribute(
    "colspan",
    "6",
  );
});
