import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";

import type { PricingDecision, PricingSnapshot } from "@/api";
import { GroupAccountCostDetails, PricingPage } from "../pricing-page";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
});

function decisions(): PricingDecision[] {
  return [
    { id: "1", name: "A 高成本", cost: "0.10000000000000001" },
    { id: "2", name: "B 低成本", cost: "0.1" },
    { id: "3", name: "C 同一低成本", cost: "0.100" },
  ].map((row) => ({
    account_id: row.id,
    account_name: row.name,
    platform: "openai",
    cost_multiplier: row.cost,
    current_group_ids: ["6"],
    desired_group_ids: ["6"],
    eligible_groups: [],
    changed: false,
    skipped: false,
    reason: null,
  }));
}

it("分组含有两个仅高精度小数不同的成本时保留两档并合并等值表示", () => {
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
        name: "标准分组",
        platform: "openai",
        status: "active",
        rate_multiplier: "1",
        managed: true,
        available: true,
        reason: null,
      },
    ],
    decisions: decisions(),
    accounts: 3,
    changes: 0,
    skipped: 0,
    generated_at: "2026-09-14T00:00:00Z",
  };
  client = new QueryClient({ defaultOptions: { queries: { enabled: false } } });
  client.setQueryData(["pricing"], snapshot);
  render(
    <QueryClientProvider client={client}>
      <PricingPage />
    </QueryClientProvider>,
  );

  expect(screen.getByText("3 个账号 · 2 档")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "查看分组 6 的账号成本明细" })).toHaveTextContent(
    "0.10000000000000001",
  );
});

it("成本明细仅在高精度小数上不同时按精确成本升序排列", () => {
  render(<GroupAccountCostDetails groupID="6" decisions={decisions()} />);

  const rows = screen.getAllByRole("row").slice(1);
  expect(within(rows[0]).getByText("B 低成本")).toBeInTheDocument();
  expect(within(rows[2]).getByText("A 高成本")).toBeInTheDocument();
});
